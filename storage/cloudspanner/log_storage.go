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
	"math"
	"math/rand"
	"sort"
	"sync"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/glog"
	"github.com/google/trillian"
	rfc6962 "github.com/google/trillian/merkle/rfc6962/hasher"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/cache"
	"github.com/google/trillian/storage/cloudspanner/spannerpb"
	"github.com/google/trillian/types"
	"go.opencensus.io/trace"
	"golang.org/x/sync/semaphore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	leafDataTbl            = "LeafData"
	seqDataByMerkleHashIdx = "SequenceByMerkleHash"
	seqDataTbl             = "SequencedLeafData"
	unseqTable             = "Unsequenced"

	// t.TreeType: 1 = Log, 3 = PreorderedLog.
	// t.TreeState: 1 = Active, 5 = Draining.
	getActiveLogIDsSQL = `SELECT t.TreeID FROM TreeRoots t
WHERE (t.TreeType = 1 OR t.TreeType = 3)
AND (t.TreeState = 1 OR t.TreeState = 5)
AND t.Deleted=false`
)

// LogStorageOptions are tuning, experiments and workarounds that can be used.
type LogStorageOptions struct {
	TreeStorageOptions

	// DequeueAcrossMerkleBuckets controls whether DequeueLeaves will only dequeue
	// from within the chosen Time+Merkle bucket, or whether it will attempt to
	// continue reading from contiguous Merkle buckets until a sufficient number
	// of leaves have been dequeued, or the entire Time bucket has been read.
	DequeueAcrossMerkleBuckets bool
	// DequeueAcrossMerkleBucketsRangeFraction specifies the fraction of Merkle
	// keyspace to dequeue from when using multi-bucket-dequeue.
	DequeueAcrossMerkleBucketsRangeFraction float64
}

var (
	// Spanner DB columns:
	colLeafIdentityHash    = "LeafIdentityHash"
	colLeafValue           = "LeafValue"
	colExtraData           = "ExtraData"
	colMerkleLeafHash      = "MerkleLeafHash"
	colSequenceNumber      = "SequenceNumber"
	colQueueTimestampNanos = "QueueTimestampNanos"
)

type leafDataCols struct {
	TreeID              int64
	LeafIdentityHash    []byte
	LeafValue           []byte
	ExtraData           []byte
	QueueTimestampNanos int64
}

type sequencedLeafDataCols struct {
	TreeID                  int64
	SequenceNumber          int64
	LeafIdentityHash        []byte
	MerkleLeafHash          []byte
	IntegrateTimestampNanos int64
}

type unsequencedCols struct {
	TreeID              int64
	Bucket              int64
	QueueTimestampNanos int64
	MerkleLeafHash      []byte
	LeafIdentityHash    []byte
}

// NewLogStorage initialises and returns a new LogStorage.
func NewLogStorage(client *spanner.Client) storage.LogStorage {
	return NewLogStorageWithOpts(client, LogStorageOptions{})
}

// NewLogStorageWithOpts initialises and returns a new LogStorage.
// The opts parameter can be used to enable custom workarounds.
func NewLogStorageWithOpts(client *spanner.Client, opts LogStorageOptions) storage.LogStorage {
	if got := opts.DequeueAcrossMerkleBucketsRangeFraction; got <= 0 || got > 1.0 {
		opts.DequeueAcrossMerkleBucketsRangeFraction = 1.0
	}
	return &logStorage{
		ts: newTreeStorageWithOpts(client, opts.TreeStorageOptions),
		// This number is taken from the maximum number of in-flight
		// transaction in the mutation pool. Add a field to opts if we decide to
		// adopt this strategy.
		writeSem: semaphore.NewWeighted(128),
		opts:     opts,
	}
}

// logStorage provides a Cloud Spanner backed trillian.LogStorage implementation.
// See third_party/golang/trillian/storage/log_storage.go for more details.
type logStorage struct {
	// ts provides the merkle-tree level primitives which are built upon by this
	// logStorage.
	ts *treeStorage

	// writeSem controls how many concurrent writes QueueLeaves/AddSequencedLeaves will do.
	writeSem *semaphore.Weighted

	// Additional options applied to this logStorage
	opts LogStorageOptions
}

func (ls *logStorage) CheckDatabaseAccessible(ctx context.Context) error {
	return checkDatabaseAccessible(ctx, ls.ts.client)
}

func (ls *logStorage) Snapshot(ctx context.Context) (storage.ReadOnlyLogTX, error) {
	var staleness spanner.TimestampBound
	if ls.opts.ReadOnlyStaleness > 0 {
		staleness = spanner.ExactStaleness(ls.opts.ReadOnlyStaleness)
	} else {
		staleness = spanner.StrongRead()
	}

	snapshotTX := &snapshotTX{
		client: ls.ts.client,
		stx:    ls.ts.client.ReadOnlyTransaction().WithTimestampBound(staleness),
		ls:     ls,
	}
	return &readOnlyLogTX{snapshotTX}, nil
}

func newLogCache(tree *trillian.Tree) (*cache.SubtreeCache, error) {
	if s := tree.HashStrategy; s != trillian.HashStrategy_RFC6962_SHA256 {
		return nil, fmt.Errorf("unknown hash strategy: %s", s)
	}
	return cache.NewLogSubtreeCache(defLogStrata, rfc6962.DefaultHasher), nil
}

func (ls *logStorage) begin(ctx context.Context, tree *trillian.Tree, readonly bool, stx spanRead) (*logTX, error) {
	tx, err := ls.ts.begin(ctx, tree, newLogCache, stx)
	if err != nil {
		return nil, err
	}

	ltx := &logTX{
		ls:       ls,
		dequeued: make(map[string]*QueuedEntry),
		treeTX:   tx,
	}

	// Needed to generate ErrTreeNeedsInit in SnapshotForTree and other methods.
	if err := ltx.getLatestRoot(ctx); err == storage.ErrTreeNeedsInit {
		return ltx, err
	} else if err != nil {
		tx.Rollback()
		return nil, err
	}
	return ltx, nil
}

func (ls *logStorage) BeginForTree(ctx context.Context, treeID int64) (storage.LogTreeTX, error) {
	return nil, ErrNotImplemented
}

func (ls *logStorage) ReadWriteTransaction(ctx context.Context, tree *trillian.Tree, f storage.LogTXFunc) error {
	_, err := ls.ts.client.ReadWriteTransaction(ctx, func(ctx context.Context, stx *spanner.ReadWriteTransaction) error {
		tx, err := ls.begin(ctx, tree, false /* readonly */, stx)
		if err != nil && err != storage.ErrTreeNeedsInit {
			return err
		}
		if err := f(ctx, tx); err != nil {
			return err
		}
		return tx.flushSubtrees(ctx)
	})
	return err
}

func (ls *logStorage) SnapshotForTree(ctx context.Context, tree *trillian.Tree) (storage.ReadOnlyLogTreeTX, error) {
	return ls.begin(ctx, tree, true /* readonly */, ls.ts.client.ReadOnlyTransaction())
}

func (ls *logStorage) QueueLeaves(ctx context.Context, tree *trillian.Tree, leaves []*trillian.LogLeaf, qTimestamp time.Time) ([]*trillian.QueuedLogLeaf, error) {
	_, treeConfig, err := ls.ts.getTreeAndConfig(ctx, tree)
	if err != nil {
		return nil, err
	}
	config, ok := treeConfig.(*spannerpb.LogStorageConfig)
	if !ok {
		return nil, status.Errorf(codes.Internal, "got unexpected config type for Log operation: %T", treeConfig)
	}

	now := time.Now().UTC().Unix()
	bucketPrefix := (now % config.NumUnseqBuckets) << 8

	results := make([]*trillian.QueuedLogLeaf, len(leaves))
	writeDupes := make(map[string][]int)

	qTS := qTimestamp.UnixNano()
	var wg sync.WaitGroup
	for i, l := range leaves {
		wg.Add(1)
		// Capture values of i and l for later reference in the MutationResultFunc below.
		i := i
		l := l
		go func() {
			defer wg.Done()

			// The insert of the leafdata and the unsequenced work item must happen atomically.
			m1, err := spanner.InsertStruct(leafDataTbl, leafDataCols{
				TreeID:              tree.TreeId,
				LeafIdentityHash:    l.LeafIdentityHash,
				LeafValue:           l.LeafValue,
				ExtraData:           l.ExtraData,
				QueueTimestampNanos: qTS,
			})
			if err != nil {
				results[i] = &trillian.QueuedLogLeaf{Status: status.Convert(err).Proto()}
				return
			}
			b := bucketPrefix | int64(l.MerkleLeafHash[0])
			m2, err := spanner.InsertStruct(unseqTable, unsequencedCols{
				TreeID:              tree.TreeId,
				Bucket:              b,
				QueueTimestampNanos: qTS,
				MerkleLeafHash:      l.MerkleLeafHash,
				LeafIdentityHash:    l.LeafIdentityHash,
			})
			if err != nil {
				results[i] = &trillian.QueuedLogLeaf{Status: status.Convert(err).Proto()}
				return
			}

			_, err = ls.ts.client.Apply(ctx, []*spanner.Mutation{m1, m2})
			if spanner.ErrCode(err) == codes.AlreadyExists {
				k := string(l.LeafIdentityHash)
				writeDupes[k] = append(writeDupes[k], i)
			} else if err != nil {
				results[i] = &trillian.QueuedLogLeaf{Status: status.Convert(err).Proto()}
			} else {
				results[i] = &trillian.QueuedLogLeaf{Leaf: l} // implicit OK status
			}
		}()
	}

	// Wait for all of our mutations to apply (or fail):
	wg.Wait()

	// Finally, read back any leaves which failed with an already exists error
	// when we tried to insert them:
	err = ls.readDupeLeaves(ctx, tree.TreeId, writeDupes, results)
	if err != nil {
		return nil, err
	}
	return results, nil
}

func (ls *logStorage) AddSequencedLeaves(ctx context.Context, tree *trillian.Tree, leaves []*trillian.LogLeaf, ts time.Time) ([]*trillian.QueuedLogLeaf, error) {
	ctx, span := trace.StartSpan(ctx, "AddSequencedLeaves")
	defer span.End()

	okProto := status.New(codes.OK, "OK").Proto()

	_, span = trace.StartSpan(ctx, "insert")
	defer span.End()
	res := make([]*trillian.QueuedLogLeaf, len(leaves))
	errs := make(chan error, 1)
	var wg sync.WaitGroup
	for i, l := range leaves {
		l.QueueTimestamp = timestamppb.New(ts)
		if err := l.QueueTimestamp.CheckValid(); err != nil {
			return nil, fmt.Errorf("got invalid queue timestamp: %w", err)
		}

		// Capture the values for later reference in the MutationResultFunc below.
		i, l := i, l
		res[i] = &trillian.QueuedLogLeaf{Status: okProto}

		wg.Add(1)
		var err error
		// The insert of the LeafData and SequencedLeafData must happen atomically.
		m1, err := spanner.InsertStruct(leafDataTbl, leafDataCols{
			TreeID:              tree.TreeId,
			LeafIdentityHash:    l.LeafIdentityHash,
			LeafValue:           l.LeafValue,
			ExtraData:           l.ExtraData,
			QueueTimestampNanos: ts.UnixNano(),
		})
		if err != nil {
			return nil, err
		}
		m2, err := spanner.InsertStruct(seqDataTbl, sequencedLeafDataCols{
			TreeID:                  tree.TreeId,
			SequenceNumber:          l.LeafIndex,
			LeafIdentityHash:        l.LeafIdentityHash,
			MerkleLeafHash:          l.MerkleLeafHash,
			IntegrateTimestampNanos: 0,
		})
		if err != nil {
			return nil, err
		}
		m := []*spanner.Mutation{m1, m2}

		doneFunc := func(err error) {
			defer wg.Done()
			if err != nil {
				// If failed because of a duplicate insert, set the status correspondingly.
				if status.Code(err) == codes.AlreadyExists {
					glog.Infof("Found already exists: index=%v, id=%v", l.LeafIndex, l.LeafIdentityHash)
					res[i].Status = status.New(codes.FailedPrecondition, "conflicting LeafIndex or LeafIdentityHash").Proto()
					return
				}
				select {
				case errs <- err:
				default: // Skip this error, we only need one.
				}
			}
		}
		if err := ls.writeSem.Acquire(ctx, 1); err != nil {
			doneFunc(err)
		} else {
			go func() {
				defer ls.writeSem.Release(1)
				doneFunc(func() error {
					_, err := ls.ts.client.Apply(ctx, m)
					return err
				}())
			}()
		}
	}
	span.End()

	// Wait for all of our mutations to apply (or fail).
	_, span = trace.StartSpan(ctx, "wait")
	wg.Wait()
	span.End()

	// Check if any failed, and return the first error if so.
	select {
	case err := <-errs:
		return nil, err
	default: // No error.
	}

	return res, nil
}

// readDupeLeaves reads the leaves whose ids are passed as keys in the dupes map,
// and stores them in results.
func (ls *logStorage) readDupeLeaves(ctx context.Context, logID int64, dupes map[string][]int, results []*trillian.QueuedLogLeaf) error {
	numDupes := len(dupes)
	if numDupes == 0 {
		return nil
	}
	glog.V(2).Infof("dupe rowsToRead: %v", numDupes)

	ids := make([][]byte, 0, numDupes)
	for k := range dupes {
		ids = append(ids, []byte(k))
	}
	dupesRead := 0
	tx := ls.ts.client.Single()
	err := readLeaves(ctx, tx, logID, ids, func(l *trillian.LogLeaf) {
		glog.V(2).Infof("Found already exists dupe: %v", l)
		dupesRead++

		indices := dupes[string(l.LeafIdentityHash)]
		glog.V(2).Infof("Indices %v", indices)
		if len(indices) == 0 {
			glog.Warningf("Logic error: Spanner returned a leaf %x, but it matched no requested index", l.LeafIdentityHash)
			return
		}
		for _, i := range indices {
			leaf := l
			results[i] = &trillian.QueuedLogLeaf{
				Leaf:   leaf,
				Status: status.Newf(codes.AlreadyExists, "leaf already exists: %v", l.LeafIdentityHash).Proto(),
			}
		}
	})
	tx.Close()
	if err != nil {
		return err
	}
	if got, want := dupesRead, numDupes; got != want {
		return fmt.Errorf("read unexpected number of dupe rows %d, want %d", got, want)
	}
	return nil
}

// logTX is a concrete implementation of the Trillian storage.LogStorage
// interface.
type logTX struct {
	// treeTX embeds the merkle-tree level transactional actions.
	*treeTX

	// logStorage is the logStorage which begat this logTX.
	ls *logStorage

	// numSequenced holds the number of leaves sequenced by this transaction.
	numSequenced int64

	// dequeued is a map of LeafIdentityHash to QueuedEntry containing entries for
	// everything dequeued by this transaction.
	// This is required to recover the primary key for the unsequenced entry in
	// UpdateSequencedLeaves.
	dequeued map[string]*QueuedEntry
}

func (tx *logTX) getLogStorageConfig() *spannerpb.LogStorageConfig {
	return tx.config.(*spannerpb.LogStorageConfig)
}

// LatestSignedLogRoot returns the freshest SignedLogRoot for this log at the
// time the transaction was started.
func (tx *logTX) LatestSignedLogRoot(ctx context.Context) (*trillian.SignedLogRoot, error) {
	currentSTH, err := tx.currentSTH(ctx)
	if err != nil {
		return nil, err
	}
	writeRev, err := tx.writeRev(ctx)
	if err != nil {
		return nil, err
	}

	if got, want := currentSTH.TreeRevision+1, writeRev; got != want {
		return nil, fmt.Errorf("inconsistency: currentSTH.TreeRevision+1 (%d) != writeRev (%d)", got, want)
	}

	// Put logRoot back together. Fortunately LogRoot has a deterministic serialization.
	logRoot, err := (&types.LogRootV1{
		TimestampNanos: uint64(currentSTH.TsNanos),
		RootHash:       currentSTH.RootHash,
		TreeSize:       uint64(currentSTH.TreeSize),
		Revision:       uint64(currentSTH.TreeRevision),
		Metadata:       currentSTH.Metadata,
	}).MarshalBinary()
	if err != nil {
		return nil, err
	}

	// We already read the latest root as part of starting the transaction (in
	// order to calculate the writeRevision), so we just return that data here:
	return &trillian.SignedLogRoot{LogRoot: logRoot}, nil
}

// StoreSignedLogRoot stores the provided root.
// This method will return an error if the caller attempts to store more than
// one root per log for a given tree size.
func (tx *logTX) StoreSignedLogRoot(ctx context.Context, root *trillian.SignedLogRoot) error {
	writeRev, err := tx.writeRev(ctx)
	if err == storage.ErrTreeNeedsInit {
		writeRev = 0
	} else if err != nil {
		return err
	}

	var logRoot types.LogRootV1
	if err := logRoot.UnmarshalBinary(root.LogRoot); err != nil {
		glog.Warningf("Failed to parse log root: %x %v", root.LogRoot, err)
		return err
	}

	if got, want := int64(logRoot.Revision), writeRev; got != want {
		return status.Errorf(codes.Internal, "root.Revision: %v, want %v", got, want)
	}

	m := spanner.Insert(
		"TreeHeads",
		[]string{
			"TreeID",
			"TimestampNanos",
			"TreeSize",
			"RootHash",
			"RootSignature",
			"TreeRevision",
			"TreeMetadata",
		},
		[]interface{}{
			int64(tx.treeID),
			int64(logRoot.TimestampNanos),
			int64(logRoot.TreeSize),
			logRoot.RootHash,
			[]byte{},
			writeRev,
			logRoot.Metadata,
		})

	stx, ok := tx.stx.(*spanner.ReadWriteTransaction)
	if !ok {
		return ErrWrongTXType
	}
	return stx.BufferWrite([]*spanner.Mutation{m})
}

func readLeaves(ctx context.Context, stx *spanner.ReadOnlyTransaction, logID int64, ids [][]byte, f func(*trillian.LogLeaf)) error {
	leafTable := leafDataTbl
	cols := []string{colLeafIdentityHash, colLeafValue, colExtraData, colQueueTimestampNanos}
	keys := make([]spanner.KeySet, 0)
	for _, l := range ids {
		keys = append(keys, spanner.Key{logID, l})
	}

	rows := stx.Read(ctx, leafTable, spanner.KeySets(keys...), cols)
	return rows.Do(func(r *spanner.Row) error {
		var l trillian.LogLeaf
		var qTimestamp int64
		if err := r.Columns(&l.LeafIdentityHash, &l.LeafValue, &l.ExtraData, &qTimestamp); err != nil {
			return err
		}
		l.QueueTimestamp = timestamppb.New(time.Unix(0, qTimestamp))
		if err := l.QueueTimestamp.CheckValid(); err != nil {
			return fmt.Errorf("got invalid queue timestamp: %w", err)
		}
		f(&l)
		return nil
	})
}

// DequeueLeaves removes [0, limit) leaves from the to-be-sequenced queue.
// The leaves returned are not guaranteed to be in any particular order.
// The caller should assign sequence numbers and pass the updated leaves as
// arguments to the UpdateSequencedLeaves method.
//
// The LogLeaf structs returned by this method will not be fully populated;
// only the LeafIdentityHash and MerkleLeafHash fields will contain data, this
// should be sufficient for assigning sequence numbers with this storage impl.
//
// TODO(al): cutoff is currently ignored.
func (tx *logTX) DequeueLeaves(ctx context.Context, limit int, cutoff time.Time) ([]*trillian.LogLeaf, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit should be > 0, got %d", limit)
	}
	// Special case pre-ordered logs.
	if tx.treeType == trillian.TreeType_PREORDERED_LOG {
		sth, err := tx.currentSTH(ctx)
		if err != nil {
			return nil, err
		}
		return tx.GetLeavesByRange(ctx, sth.TreeSize, int64(limit))
	}

	// Decide which bucket(s) to dequeue from.
	// The high 8 bits of the bucket key is a time based ring - at any given
	// moment, FEs queueing entries will be adding them to different buckets
	// than we're dequeuing from here - the low 8 bits are the first byte of the
	// merkle hash of the entry.
	now := time.Now().UTC()
	cfg := tx.getLogStorageConfig()
	// Select a prefix that is likley to be on a different span server to spread load.
	prefix := int64((((now.Unix() + cfg.NumUnseqBuckets/2) % cfg.NumUnseqBuckets) << 8))

	// Choose a starting point in the merkle prefix range, and calculate the
	// start/limit of the merkle range we'll dequeue from.
	// It seems to be much better to tune for keeping this range small, and allow
	// the signer to run multiple times per second than try to dequeue a large batch
	// which spans a large number of merkle prefixes.
	const suffixBuckets = 0x100
	suffixStart := rand.Int63n(suffixBuckets)
	suffixFraction := float64(cfg.NumMerkleBuckets) / float64(suffixBuckets)
	if tx.ls.opts.DequeueAcrossMerkleBuckets {
		suffixFraction = tx.ls.opts.DequeueAcrossMerkleBucketsRangeFraction
	}
	suffixEnd := suffixStart + int64(math.Ceil(suffixBuckets*suffixFraction))

	keysets := []spanner.KeySet{}
	if suffixEnd < suffixBuckets {
		keysets = append(keysets,
			spanner.KeyRange{
				Start: spanner.Key{tx.treeID, prefix | suffixStart},
				End:   spanner.Key{tx.treeID, prefix | suffixEnd},
				Kind:  spanner.ClosedClosed,
			})
	} else {
		// The range is too big and wraps around, overflowing a byte value, so we'll
		// start the second range at 0 and end at the upper limit modulo suffixBuckets:
		suffixEnd %= suffixBuckets
		keysets = append(keysets,
			spanner.KeyRange{
				Start: spanner.Key{tx.treeID, prefix | suffixStart},
				End:   spanner.Key{tx.treeID, prefix | suffixBuckets - 1},
				Kind:  spanner.ClosedClosed,
			},
			spanner.KeyRange{
				Start: spanner.Key{tx.treeID, prefix},
				// XXX: When suffixFraction = 1, this produces an overlapping range at suffixStart
				End:  spanner.Key{tx.treeID, prefix | suffixEnd},
				Kind: spanner.ClosedClosed,
			})
	}

	errBreak := errors.New("break")
	ret := make([]*trillian.LogLeaf, 0, limit)
	if err := tx.stx.Read(ctx, unseqTable, spanner.KeySets(keysets...),
		[]string{"Bucket", colQueueTimestampNanos, colMerkleLeafHash, colLeafIdentityHash},
	).Do(func(r *spanner.Row) error {
		var l trillian.LogLeaf
		var qe QueuedEntry
		if err := r.Columns(&qe.bucket, &qe.timestamp, &l.MerkleLeafHash, &l.LeafIdentityHash); err != nil {
			return err
		}

		l.QueueTimestamp = timestamppb.New(time.Unix(0, qe.timestamp))
		if err := l.QueueTimestamp.CheckValid(); err != nil {
			return fmt.Errorf("got invalid queue timestamp: %w", err)
		}
		k := string(l.LeafIdentityHash)
		if tx.dequeued[k] != nil {
			// dupe, user probably called DequeueLeaves more than once.
			return nil
		}

		ret = append(ret, &l)
		qe.leaf = &l
		tx.dequeued[k] = &qe

		// If we've already got enough leaves, don't wrap around for any further reads.
		if len(ret) >= limit {
			return errBreak
		}
		return nil
	}); err != nil && err != errBreak {
		return nil, err
	}
	return ret, nil
}

// UpdateSequencedLeaves stores the sequence numbers assigned to the leaves,
// and integrates them into the tree.
func (tx *logTX) UpdateSequencedLeaves(ctx context.Context, leaves []*trillian.LogLeaf) error {
	stx, ok := tx.stx.(*spanner.ReadWriteTransaction)
	if !ok {
		return ErrWrongTXType
	}
	// We need the latest root to know what the next sequence number to use below is.
	currentSTH, err := tx.currentSTH(ctx)
	if err != nil {
		return err
	}

	for _, l := range leaves {
		if got, want := l.LeafIndex, currentSTH.TreeSize+tx.numSequenced; got != want {
			return fmt.Errorf("attempting to assign non-sequential leaf with sequence %d, want %d", got, want)
		}

		qe, ok := tx.dequeued[string(l.LeafIdentityHash)]
		if !ok {
			return fmt.Errorf("attempting to assign unknown merkleleafhash %v", l.MerkleLeafHash)
		}

		if err := l.IntegrateTimestamp.CheckValid(); err != nil {
			return fmt.Errorf("got invalid integrate timestamp: %w", err)
		}
		iTimestamp := l.IntegrateTimestamp.AsTime()

		// Add the sequence mapping...
		m1, err := spanner.InsertStruct(seqDataTbl, sequencedLeafDataCols{
			TreeID:                  tx.treeID,
			SequenceNumber:          l.LeafIndex,
			LeafIdentityHash:        l.LeafIdentityHash,
			MerkleLeafHash:          l.MerkleLeafHash,
			IntegrateTimestampNanos: iTimestamp.UnixNano(),
		})
		if err != nil {
			return err
		}

		m2 := spanner.Delete(unseqTable, spanner.Key{tx.treeID, qe.bucket, qe.timestamp, l.MerkleLeafHash})

		tx.numSequenced++
		if err := stx.BufferWrite([]*spanner.Mutation{m1, m2}); err != nil {
			return fmt.Errorf("bufferwrite(): %v", err)
		}
	}

	return nil
}

// leafmap is a map of LogLeaf by sequence number which knows how to populate
// itself directly from Spanner Rows.
type leafmap map[int64]*trillian.LogLeaf

// addFullRow appends the leaf data in row to the array
func (l leafmap) addFullRow(seqLeaves map[string]sequencedLeafDataCols) func(r *spanner.Row) error {
	return func(r *spanner.Row) error {
		var leafData leafDataCols
		if err := r.ToStruct(&leafData); err != nil {
			return err
		}
		seqLeaf, ok := seqLeaves[string(leafData.LeafIdentityHash)]
		if !ok {
			return fmt.Errorf("LeafIdentityHash %x not found in SequencedLeafData",
				leafData.LeafIdentityHash)
		}
		leaf := &trillian.LogLeaf{
			MerkleLeafHash:   seqLeaf.MerkleLeafHash,
			LeafValue:        leafData.LeafValue,
			ExtraData:        leafData.ExtraData,
			LeafIndex:        seqLeaf.SequenceNumber,
			LeafIdentityHash: leafData.LeafIdentityHash,
		}
		leaf.QueueTimestamp = timestamppb.New(time.Unix(0, leafData.QueueTimestampNanos))
		if err := leaf.QueueTimestamp.CheckValid(); err != nil {
			return fmt.Errorf("got invalid queue timestamp: %w", err)
		}
		leaf.IntegrateTimestamp = timestamppb.New(time.Unix(0, seqLeaf.IntegrateTimestampNanos))
		if err := leaf.IntegrateTimestamp.CheckValid(); err != nil {
			return fmt.Errorf("got invalid integrate timestamp: %w", err)
		}

		l[seqLeaf.SequenceNumber] = leaf
		return nil
	}
}

// leavesByHash is a map of []LogLeaf (keyed by value hash) which knows how to
// populate itself from Spanner Rows.
type leavesByHash map[string][]*trillian.LogLeaf

// addRow adds the contents of the Spanner Row to this map.
func (b leavesByHash) addRow(r *spanner.Row) error {
	var h []byte
	var v []byte
	var ed []byte
	var qTimestamp int64
	if err := r.Columns(&h, &v, &ed, &qTimestamp); err != nil {
		return err
	}
	queueTimestamp := timestamppb.New(time.Unix(0, qTimestamp))
	if err := queueTimestamp.CheckValid(); err != nil {
		return fmt.Errorf("got invalid queue timestamp: %w", err)
	}

	leaves, ok := b[string(h)]
	if !ok {
		return fmt.Errorf("inconsistency: unexpected leafValueHash %v", h)
	}
	for i := range leaves {
		if got, want := leaves[i].LeafIdentityHash, h; !bytes.Equal(got, want) {
			return fmt.Errorf("inconsistency: unexpected leafvaluehash %v, want %v", got, want)
		}
		leaves[i].LeafValue = v
		leaves[i].ExtraData = ed
		leaves[i].QueueTimestamp = queueTimestamp
	}
	return nil
}

// populateLeafData populates the partial LogLeaf structs held in the passed in
// map of LeafIdentityHash to []LogLeaf by reading the remaining LogLeaf data from
// Spanner.
// The value of byHash is an []LogLeaf because the underlying leaf data could
// be sequenced into multiple tree leaves if the log allows duplication.
func (tx *logTX) populateLeafData(ctx context.Context, byHash leavesByHash) error {
	keySet := make([]spanner.KeySet, 0, len(byHash))
	for k := range byHash {
		keySet = append(keySet, spanner.Key{tx.treeID, []byte(k)})
	}
	cols := []string{colLeafIdentityHash, colLeafValue, colExtraData, colQueueTimestampNanos}
	rows := tx.stx.Read(ctx, leafDataTbl, spanner.KeySets(keySet...), cols)
	return rows.Do(byHash.addRow)
}

func validateRange(start, count, treeSize int64) error {
	if count <= 0 {
		return status.Errorf(codes.InvalidArgument, "invalid count %d", count)
	}
	if start < 0 {
		return status.Errorf(codes.InvalidArgument, "invalid start %d", start)
	}
	if treeSize >= 0 && start >= treeSize {
		return status.Errorf(codes.OutOfRange, "start index %d beyond tree size %d", start, treeSize)
	}
	return nil
}

// GetLeavesByRange returns the leaves corresponding to the given index range.
func (tx *logTX) GetLeavesByRange(ctx context.Context, start, count int64) ([]*trillian.LogLeaf, error) {
	// We need the latest root to validate the indices are within range.
	currentSTH, err := tx.currentSTH(ctx)
	if err != nil {
		return nil, err
	}

	xsize := currentSTH.TreeSize
	if tx.treeType == trillian.TreeType_PREORDERED_LOG {
		xsize = -1 // Allow requesting entries beyond the tree size.
	}
	if err := validateRange(start, count, xsize); err != nil {
		return nil, err
	}
	xend := start + count
	if tx.treeType != trillian.TreeType_PREORDERED_LOG && xend > xsize {
		xend = xsize
		count = xend - start
	}

	// TODO: replace with INNER JOIN when spannertest supports JOINs
	// https://github.com/googleapis/google-cloud-go/tree/master/spanner/spannertest
	stmt := spanner.NewStatement(
		`SELECT 
		   TreeID,
		   SequenceNumber,
		   LeafIdentityHash,
		   MerkleLeafHash, 
		   IntegrateTimestampNanos
		 FROM 
		   SequencedLeafData
		 WHERE 
		   TreeID = @tree_id AND 
		   SequenceNumber >= @start AND 
		   SequenceNumber < @xend`)
	stmt.Params["tree_id"] = tx.treeID
	stmt.Params["start"] = start
	stmt.Params["xend"] = xend
	seqLeaves := make(map[string]sequencedLeafDataCols)
	if err := tx.stx.Query(ctx, stmt).Do(func(r *spanner.Row) error {
		var seqLeaf sequencedLeafDataCols
		if err := r.ToStruct(&seqLeaf); err != nil {
			return err
		}
		seqLeaves[string(seqLeaf.LeafIdentityHash)] = seqLeaf
		return nil
	}); err != nil {
		return nil, err
	}

	idHashes := make([][]byte, 0, len(seqLeaves))
	for _, l := range seqLeaves {
		idHashes = append(idHashes, l.LeafIdentityHash)
	}

	stmt = spanner.NewStatement(
		`SELECT 
		   TreeID,
		   LeafIdentityHash, 
		   LeafValue, 
		   ExtraData, 
		   QueueTimestampNanos
		 FROM 
		   LeafData
		 WHERE 
		   TreeID = @tree_id AND 
		   LeafIdentityHash IN UNNEST(@id_hashes)`)
	stmt.Params["tree_id"] = tx.treeID
	stmt.Params["id_hashes"] = idHashes

	// Results need to be returned in order [start, end), all of which
	// should be available (as we restricted xend/count to TreeSize).
	leaves := make(leafmap)
	if err := tx.stx.Query(ctx, stmt).
		Do(leaves.addFullRow(seqLeaves)); err != nil {
		return nil, err
	}

	if got := int64(len(leaves)); got > count {
		return nil, fmt.Errorf("unexpected number of leaves %d, want <= %d", got, count)
	}

	ret := make([]*trillian.LogLeaf, 0, count)
	for i := start; i < (start + count); i++ {
		l, ok := leaves[i]
		if !ok {
			if i < int64(currentSTH.TreeSize) {
				return nil, fmt.Errorf("missing expected index %d", i)
			}
			break
		}
		ret = append(ret, l)
	}
	return ret, nil
}

// leafSlice is a slice of LogLeaf which knows how to populate itself from
// Spanner Rows.
type leafSlice []*trillian.LogLeaf

// addRow appends the leaf data in Row to the array.
func (l *leafSlice) addRow(r *spanner.Row) error {
	var (
		s      int64
		mh, lh []byte
	)

	if err := r.Columns(&s, &mh, &lh); err != nil {
		return err
	}
	leaf := trillian.LogLeaf{
		LeafIndex:        s,
		MerkleLeafHash:   mh,
		LeafIdentityHash: lh,
	}
	*l = append(*l, &leaf)
	return nil
}

// getUsingIndex returns a slice containing the LogLeaf structs corresponding
// to the requested keys.
// The entries in key are used in constructing a primary key (treeID, keyElem)
// for the specified Spanner index.
// If bySeq is true, the returned slice will be order by LogLeaf.LeafIndex.
func (tx *logTX) getUsingIndex(ctx context.Context, idx string, keys [][]byte, bySeq bool) ([]*trillian.LogLeaf, error) {
	keySet := make([]spanner.KeySet, 0, len(keys))
	for _, k := range keys {
		keySet = append(keySet, spanner.Key{tx.treeID, k})
	}

	leaves := make(leafSlice, 0, len(keys))
	cols := []string{colSequenceNumber, colMerkleLeafHash, colLeafIdentityHash}
	rows := tx.stx.ReadUsingIndex(ctx, seqDataTbl, idx, spanner.KeySets(keySet...), cols)
	if err := rows.Do(leaves.addRow); err != nil {
		return nil, err
	}

	byHash := make(leavesByHash)
	for i := range leaves {
		k := string(leaves[i].LeafIdentityHash)
		byHash[k] = append(byHash[k], leaves[i])
	}

	// Now we can fetch & combine the actual leaf data:
	if err := tx.populateLeafData(ctx, byHash); err != nil {
		return nil, err
	}

	if bySeq {
		sort.Sort(byIndex(leaves))
	}

	return leaves, nil
}

// GetLeavesByHash returns the leaves corresponding to the given merkle hashes.
// Any unknown hashes will simply be ignored, and the caller should inspect the
// returned leaves to determine whether this has occurred.
// TODO(al): Currently, this method does not populate the IntegrateTimestamp
//   member of the returned leaves. We should convert this method to use SQL
//   rather than denormalising IntegrateTimestampNanos into the index too.
func (tx *logTX) GetLeavesByHash(ctx context.Context, hashes [][]byte, bySeq bool) ([]*trillian.LogLeaf, error) {
	return tx.getUsingIndex(ctx, seqDataByMerkleHashIdx, hashes, bySeq)
}

// QueuedEntry represents a leaf which was dequeued.
// It's used to store some extra info which is necessary for rebuilding the
// leaf's primary key when it's passed back in to UpdateSequencedLeaves.
type QueuedEntry struct {
	// leaf is partially populated with the Merkle and LeafValue hashes only.
	leaf      *trillian.LogLeaf
	bucket    int64
	timestamp int64
}

// readOnlyLogTX implements storage.ReadOnlyLogTX.
type readOnlyLogTX struct {
	*snapshotTX
}

func (tx *readOnlyLogTX) GetActiveLogIDs(ctx context.Context) ([]int64, error) {
	tx.mu.RLock()
	defer tx.mu.RUnlock()
	if tx.stx == nil {
		return nil, ErrTransactionClosed
	}

	ids := []int64{}
	// We have to use SQL as Read() doesn't work against an index.
	stmt := spanner.NewStatement(getActiveLogIDsSQL)
	rows := tx.stx.Query(ctx, stmt)
	if err := rows.Do(func(r *spanner.Row) error {
		var id int64
		if err := r.Columns(&id); err != nil {
			return err
		}
		ids = append(ids, id)
		return nil
	}); err != nil {
		glog.Warningf("GetActiveLogIDs: %v", err)
		return nil, fmt.Errorf("problem executing getActiveLogIDsSQL: %v", err)
	}
	return ids, nil
}

// LogLeaf sorting boilerplate below.

type byIndex []*trillian.LogLeaf

func (b byIndex) Len() int { return len(b) }

func (b byIndex) Swap(i, j int) { b[i], b[j] = b[j], b[i] }

func (b byIndex) Less(i, j int) bool { return b[i].LeafIndex < b[j].LeafIndex }
