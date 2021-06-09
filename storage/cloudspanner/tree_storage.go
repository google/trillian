// Copyright 2018 Google LLC. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cloudspanner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/merkle/compact"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/cache"
	"github.com/google/trillian/storage/cloudspanner/spannerpb"
	"github.com/google/trillian/storage/storagepb"
	"github.com/google/trillian/storage/tree"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

var (
	// ErrNotFound is returned when a read/lookup fails because there was no such
	// item.
	ErrNotFound = status.Errorf(codes.NotFound, "not found")

	// ErrNotImplemented is returned by any interface methods which have not been
	// implemented yet.
	ErrNotImplemented = errors.New("not implemented")

	// ErrTransactionClosed is returned by interface methods when an operation is
	// attempted on a transaction whose Commit or Rollback methods have
	// previously been called.
	ErrTransactionClosed = errors.New("transaction is closed")

	// ErrWrongTXType is returned when, somehow, a write operation is attempted
	// with a read-only transaction.  This should not even be possible.
	ErrWrongTXType = errors.New("mutating method called on read-only transaction")
)

const (
	subtreeTbl   = "SubtreeData"
	colSubtree   = "Subtree"
	colSubtreeID = "SubtreeID"
	colTreeID    = "TreeID"
	colRevision  = "Revision"
)

// treeStorage provides a shared base for the concrete CloudSpanner-backed
// implementation of the Trillian storage.LogStorage and storage.MapStorage
// interfaces.
type treeStorage struct {
	admin  storage.AdminStorage
	opts   TreeStorageOptions
	client *spanner.Client
}

// TreeStorageOptions holds various levers for configuring the tree storage instance.
type TreeStorageOptions struct {
	// ReadOnlyStaleness controls how far in the past a read-only snapshot
	// transaction will read.
	// This is intended to allow Spanner to use local replicas for read requests
	// to help with performance.
	// See https://cloud.google.com/spanner/docs/timestamp-bounds for more details.
	ReadOnlyStaleness time.Duration
}

func newTreeStorageWithOpts(client *spanner.Client, opts TreeStorageOptions) *treeStorage {
	return &treeStorage{client: client, admin: nil, opts: opts}
}

type spanRead interface {
	Query(context.Context, spanner.Statement) *spanner.RowIterator
	Read(ctx context.Context, table string, keys spanner.KeySet, columns []string) *spanner.RowIterator
	ReadUsingIndex(ctx context.Context, table, index string, keys spanner.KeySet, columns []string) *spanner.RowIterator
	ReadRow(ctx context.Context, table string, key spanner.Key, columns []string) (*spanner.Row, error)
	ReadWithOptions(ctx context.Context, table string, keys spanner.KeySet, columns []string, opts *spanner.ReadOptions) (ri *spanner.RowIterator)
}

// latestSTH reads and returns the newest STH.
func (t *treeStorage) latestSTH(ctx context.Context, stx spanRead, treeID int64) (*spannerpb.TreeHead, error) {
	query := spanner.NewStatement(
		"SELECT TreeID, TimestampNanos, TreeSize, RootHash, RootSignature, TreeRevision, TreeMetadata FROM TreeHeads" +
			"   WHERE TreeID = @tree_id" +
			"   ORDER BY TreeRevision DESC " +
			"   LIMIT 1")
	query.Params["tree_id"] = treeID

	var th *spannerpb.TreeHead
	rows := stx.Query(ctx, query)
	defer rows.Stop()
	err := rows.Do(func(r *spanner.Row) error {
		tth := &spannerpb.TreeHead{}
		if err := r.Columns(&tth.TreeId, &tth.TsNanos, &tth.TreeSize, &tth.RootHash, &tth.Signature, &tth.TreeRevision, &tth.Metadata); err != nil {
			return err
		}

		th = tth
		return nil
	})
	if err != nil {
		return nil, err
	}
	if th == nil {
		glog.Warningf("no head found for treeID %v", treeID)
		return nil, storage.ErrTreeNeedsInit
	}
	return th, nil
}

type newCacheFn func(*trillian.Tree) (*cache.SubtreeCache, error)

func (t *treeStorage) getTreeAndConfig(ctx context.Context, tree *trillian.Tree) (*trillian.Tree, proto.Message, error) {
	config, err := unmarshalSettings(tree)
	if err != nil {
		return nil, nil, err
	}
	return tree, config, nil
}

// begin returns a newly started tree transaction for the specified tree.
func (t *treeStorage) begin(ctx context.Context, tree *trillian.Tree, newCache newCacheFn, stx spanRead) (*treeTX, error) {
	tree, config, err := t.getTreeAndConfig(ctx, tree)
	if err != nil {
		return nil, err
	}
	subtreeCache, err := newCache(tree)
	if err != nil {
		return nil, err
	}
	treeTX := &treeTX{
		treeID:    tree.TreeId,
		treeType:  tree.TreeType,
		ts:        t,
		stx:       stx,
		cache:     subtreeCache,
		config:    config,
		_writeRev: -1,
	}

	return treeTX, nil
}

// getLatestRoot populates this TX with the newest tree root visible (when
// taking read-staleness into account) by this transaction.
func (t *treeTX) getLatestRoot(ctx context.Context) error {
	t.getLatestRootOnce.Do(func() {
		t._currentSTH, t._currentSTHErr = t.ts.latestSTH(ctx, t.stx, t.treeID)
		if t._currentSTH != nil {
			t._writeRev = t._currentSTH.TreeRevision + 1
		}
	})

	return t._currentSTHErr
}

// treeTX is a concrete implementation of the Trillian
// storage.TreeTX interface.
type treeTX struct {
	treeID   int64
	treeType trillian.TreeType

	ts *treeStorage

	// mu guards the nil setting/checking of stx as part of the open checking.
	mu sync.RWMutex
	// stx is the underlying Spanner transaction in which all operations will be
	// performed.
	stx spanRead

	// config holds the StorageSettings proto acquired from the trillian.Tree.
	// Varies according to tree_type (LogStorageConfig vs MapStorageConfig).
	config proto.Message

	// currentSTH holds a copy of the latest known STH at the time the
	// transaction was started, or nil if there was no STH.
	_currentSTH    *spannerpb.TreeHead
	_currentSTHErr error

	// writeRev is the tree revision at which any writes will be made.
	_writeRev int64

	cache *cache.SubtreeCache

	getLatestRootOnce sync.Once
}

func (t *treeTX) currentSTH(ctx context.Context) (*spannerpb.TreeHead, error) {
	if err := t.getLatestRoot(ctx); err != nil {
		return nil, err
	}
	return t._currentSTH, nil
}

func (t *treeTX) writeRev(ctx context.Context) (int64, error) {
	if err := t.getLatestRoot(ctx); err == storage.ErrTreeNeedsInit {
		return 0, nil
	} else if err != nil {
		return -1, fmt.Errorf("writeRev(): %v", err)
	}
	return t._writeRev, nil
}

// storeSubtrees adds buffered writes to the in-flight transaction to store the
// passed in subtrees.
func (t *treeTX) storeSubtrees(ctx context.Context, sts []*storagepb.SubtreeProto) error {
	stx, ok := t.stx.(*spanner.ReadWriteTransaction)
	if !ok {
		return ErrWrongTXType
	}
	for _, st := range sts {
		if st == nil {
			continue
		}
		stBytes, err := proto.Marshal(st)
		if err != nil {
			return err
		}
		m := spanner.Insert(
			subtreeTbl,
			[]string{colTreeID, colSubtreeID, colRevision, colSubtree},
			[]interface{}{t.treeID, st.Prefix, t._writeRev, stBytes},
		)
		if err := stx.BufferWrite([]*spanner.Mutation{m}); err != nil {
			return err
		}
	}
	return nil
}

func (t *treeTX) flushSubtrees(ctx context.Context) error {
	return t.cache.Flush(ctx, t.storeSubtrees)
}

// Commit attempts to apply all actions perfomed to the underlying Spanner
// transaction.  If this call returns an error, any values READ via this
// transaction MUST NOT be used.
// On return from the call, this transaction will be in a closed state.
func (t *treeTX) Commit(ctx context.Context) error {
	t.mu.Lock()
	defer func() {
		t.stx = nil
		t.mu.Unlock()
	}()

	if t.stx == nil {
		return ErrTransactionClosed
	}
	switch stx := t.stx.(type) {
	case *spanner.ReadOnlyTransaction:
		glog.V(1).Infof("Closed readonly tx %p", stx)
		stx.Close()
		return nil
	case *spanner.ReadWriteTransaction:
		return t.flushSubtrees(ctx)
	default:
		return fmt.Errorf("internal error: unknown transaction type %T", stx)
	}
}

// Rollback aborts any operations perfomed on the underlying Spanner
// transaction.
// On return from the call, this transaction will be in a closed state.
func (t *treeTX) Rollback() error {
	t.mu.Lock()
	defer func() {
		t.stx = nil
		t.mu.Unlock()
	}()

	if t.stx == nil {
		return ErrTransactionClosed
	}

	if stx, ok := t.stx.(*spanner.ReadOnlyTransaction); ok {
		glog.V(1).Infof("Closed snapshot %p", stx)
		stx.Close()
	}

	return nil
}

func (t *treeTX) Close() error {
	if t.IsOpen() {
		if err := t.Rollback(); err != nil && err != ErrTransactionClosed {
			glog.Warningf("Rollback error on Close(): %v", err)
			return err
		}
	}
	return nil
}

// IsOpen returns true iff neither Commit nor Rollback have been called.
// If this function returns false, further operations may not be attempted on
// this transaction object.
func (t *treeTX) IsOpen() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.stx != nil
}

// readRevision returns the tree revision at which the currently visible (taking
// into account read-staleness) STH was stored.
func (t *treeTX) readRevision(ctx context.Context) (int64, error) {
	sth, err := t.currentSTH(ctx)
	if err != nil {
		return -1, err
	}
	return sth.TreeRevision, nil
}

// WriteRevision returns the tree revision at which any tree-modifying
// operations will write.
func (t *treeTX) WriteRevision(ctx context.Context) (int64, error) {
	rev, err := t.writeRev(ctx)
	if err != nil {
		return -1, err
	}
	return rev, nil
}

// getSubtree retrieves the most recent subtree specified by id at (or below)
// the requested revision.
// If no such subtree exists it returns nil.
func (t *treeTX) getSubtree(ctx context.Context, rev int64, id []byte) (p *storagepb.SubtreeProto, e error) {
	var ret *storagepb.SubtreeProto
	stmt := spanner.NewStatement(
		"SELECT Revision, Subtree FROM SubtreeData" +
			"  WHERE TreeID = @tree_id" +
			"  AND   SubtreeID = @subtree_id" +
			"  AND   Revision <= @revision" +
			"  ORDER BY Revision DESC" +
			"  LIMIT 1")
	stmt.Params["tree_id"] = t.treeID
	stmt.Params["subtree_id"] = id
	stmt.Params["revision"] = rev

	rows := t.stx.Query(ctx, stmt)
	err := rows.Do(func(r *spanner.Row) error {
		if ret != nil {
			return nil
		}

		var rRev int64
		var st storagepb.SubtreeProto
		stBytes := make([]byte, 1<<20)
		if err := r.Columns(&rRev, &stBytes); err != nil {
			return err
		}
		if err := proto.Unmarshal(stBytes, &st); err != nil {
			return err
		}

		if rRev > rev {
			return fmt.Errorf("got subtree with too new a revision %d, want %d", rRev, rev)
		}
		if got, want := id, st.Prefix; !bytes.Equal(got, want) {
			return fmt.Errorf("got subtree with prefix %v, wanted %v", got, want)
		}
		if got, want := rRev, rev; got > rev {
			return fmt.Errorf("got subtree rev %d, wanted <= %d", got, want)
		}
		ret = &st

		// If this is a subtree with a zero-length prefix, we'll need to create an
		// empty Prefix field:
		if st.Prefix == nil && len(id) == 0 {
			st.Prefix = []byte{}
		}
		return nil
	})
	return ret, err
}

// GetMerkleNodes returns the requested set of nodes at, or before, the
// transaction read revision.
func (t *treeTX) GetMerkleNodes(ctx context.Context, ids []compact.NodeID) ([]tree.Node, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.stx == nil {
		return nil, ErrTransactionClosed
	}
	rev, err := t.readRevision(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get read revision: %v", err)
	}

	return t.cache.GetNodes(ids,
		func(ids [][]byte) ([]*storagepb.SubtreeProto, error) {
			// Request the various subtrees in parallel.
			// c will carry any retrieved subtrees
			c := make(chan *storagepb.SubtreeProto, len(ids))

			// Spawn goroutines for each request
			g, gctx := errgroup.WithContext(ctx)
			for _, id := range ids {
				id := id
				g.Go(func() error {
					st, err := t.getSubtree(gctx, rev, id)
					if err != nil {
						return err
					}
					c <- st
					return nil
				})
			}
			if err := g.Wait(); err != nil {
				return nil, err
			}
			close(c)

			// Now wait for the goroutines to signal their completion, and collect
			// the results.
			ret := make([]*storagepb.SubtreeProto, 0, len(ids))
			for st := range c {
				if st != nil {
					ret = append(ret, st)
				}
			}
			return ret, nil
		})
}

// SetMerkleNodes stores the provided merkle nodes at the writeRevision of the
// transaction.
func (t *treeTX) SetMerkleNodes(ctx context.Context, nodes []tree.Node) error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.stx == nil {
		return ErrTransactionClosed
	}

	writeRev, err := t.writeRev(ctx)
	if err != nil {
		return err
	}

	for _, n := range nodes {
		err := t.cache.SetNodeHash(
			n.ID,
			n.Hash,
			func(id []byte) (*storagepb.SubtreeProto, error) {
				return t.getSubtree(ctx, writeRev-1, id)
			})
		if err != nil {
			return err
		}
	}
	return nil
}

func checkDatabaseAccessible(ctx context.Context, client *spanner.Client) error {
	stmt := spanner.NewStatement("SELECT 1")
	// We don't care about freshness here, being able to read *something* is enough
	rows := client.Single().Query(ctx, stmt)
	defer rows.Stop()
	return rows.Do(func(row *spanner.Row) error { return nil })
}

// snapshotTX provides the standard methods for snapshot-based TXs.
type snapshotTX struct {
	client *spanner.Client

	// mu guards stx, which is set to nil when the TX is closed.
	mu  sync.RWMutex
	stx spanRead
	ls  *logStorage
}

func (t *snapshotTX) Commit(ctx context.Context) error {
	// No work required to commit snapshot transactions
	return t.Close()
}

func (t *snapshotTX) Rollback() error {
	return t.Close()
}

func (t *snapshotTX) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stx == nil {
		return ErrTransactionClosed
	}
	if stx, ok := t.stx.(*spanner.ReadOnlyTransaction); ok {
		glog.V(1).Infof("Closed log snapshot %p", stx)
		stx.Close()
	}
	t.stx = nil

	return nil
}
