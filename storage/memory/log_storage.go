// Copyright 2017 Google LLC. All Rights Reserved.
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

package memory

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/google/btree"
	"github.com/google/trillian"
	"github.com/google/trillian/merkle/compact"
	rfc6962 "github.com/google/trillian/merkle/rfc6962/hasher"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/cache"
	stree "github.com/google/trillian/storage/tree"
	"github.com/google/trillian/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const logIDLabel = "logid"

var (
	once            sync.Once
	queuedCounter   monitoring.Counter
	dequeuedCounter monitoring.Counter
)

func createMetrics(mf monitoring.MetricFactory) {
	queuedCounter = mf.NewCounter("mem_queued_leaves", "Number of leaves queued", logIDLabel)
	dequeuedCounter = mf.NewCounter("mem_dequeued_leaves", "Number of leaves dequeued", logIDLabel)
}

func labelForTX(t *logTreeTX) string {
	return strconv.FormatInt(t.treeID, 10)
}

// unseqKey formats a key for use in a tree's BTree store.
// The associated Item value will be a list of unsequenced entries.
func unseqKey(treeID int64) btree.Item {
	return &kv{k: fmt.Sprintf("/%d/unseq", treeID)}
}

// seqLeafKey formats a key for use in a tree's BTree store.
// The associated Item value will be the leaf at the given sequence number.
func seqLeafKey(treeID, seq int64) btree.Item {
	return &kv{k: fmt.Sprintf("/%d/seq/%020d", treeID, seq)}
}

// hashToSeqKey formats a key for use in a tree's BTree store.
// The associated Item value will be the sequence number for the leaf with
// the given hash.
func hashToSeqKey(treeID int64) btree.Item {
	return &kv{k: fmt.Sprintf("/%d/h2s", treeID)}
}

// sthKey formats a key for use in a tree's BTree store.
// The associated Item value will be the STH with the given timestamp.
func sthKey(treeID int64, timestamp uint64) btree.Item {
	return &kv{k: fmt.Sprintf("/%d/sth/%020d", treeID, timestamp)}
}

// getActiveLogIDs returns the IDs of all logs that are currently in a state
// that requires sequencing (e.g. ACTIVE, DRAINING).
func getActiveLogIDs(trees map[int64]*tree) []int64 {
	var ret []int64
	for id, tree := range trees {
		if tree.meta.GetDeleted() {
			continue
		}

		switch tree.meta.GetTreeType() {
		case trillian.TreeType_LOG, trillian.TreeType_PREORDERED_LOG:
			switch tree.meta.GetTreeState() {
			case trillian.TreeState_ACTIVE, trillian.TreeState_DRAINING:
				ret = append(ret, id)
			}
		}
	}
	return ret
}

type memoryLogStorage struct {
	*TreeStorage
	metricFactory monitoring.MetricFactory
}

// NewLogStorage creates an in-memory LogStorage instance.
func NewLogStorage(ts *TreeStorage, mf monitoring.MetricFactory) storage.LogStorage {
	if mf == nil {
		mf = monitoring.InertMetricFactory{}
	}
	ret := &memoryLogStorage{
		TreeStorage:   ts,
		metricFactory: mf,
	}
	return ret
}

func (m *memoryLogStorage) CheckDatabaseAccessible(ctx context.Context) error {
	return nil
}

type readOnlyLogTX struct {
	ms *TreeStorage
}

func (m *memoryLogStorage) Snapshot(ctx context.Context) (storage.ReadOnlyLogTX, error) {
	return &readOnlyLogTX{m.TreeStorage}, nil
}

func (t *readOnlyLogTX) Commit(context.Context) error {
	return nil
}

func (t *readOnlyLogTX) Rollback() error {
	return nil
}

func (t *readOnlyLogTX) Close() error {
	return nil
}

func (t *readOnlyLogTX) GetActiveLogIDs(ctx context.Context) ([]int64, error) {
	t.ms.mu.RLock()
	defer t.ms.mu.RUnlock()

	return getActiveLogIDs(t.ms.trees), nil
}

func (m *memoryLogStorage) beginInternal(ctx context.Context, tree *trillian.Tree, readonly bool) (*logTreeTX, error) {
	once.Do(func() {
		createMetrics(m.metricFactory)
	})

	stCache := cache.NewLogSubtreeCache(rfc6962.DefaultHasher)
	ttx, err := m.TreeStorage.beginTreeTX(ctx, tree.TreeId, rfc6962.DefaultHasher.Size(), stCache, readonly)
	if err != nil {
		return nil, err
	}

	ltx := &logTreeTX{
		treeTX: ttx,
		ls:     m,
	}

	ltx.slr, err = ltx.fetchLatestRoot(ctx)
	if err == storage.ErrTreeNeedsInit {
		return ltx, err
	} else if err != nil {
		ttx.Rollback()
		return nil, err
	}

	if err := ltx.root.UnmarshalBinary(ltx.slr.LogRoot); err != nil {
		ttx.Rollback()
		return nil, err
	}

	ltx.treeTX.writeRevision = int64(ltx.root.Revision) + 1

	return ltx, nil
}

func (m *memoryLogStorage) ReadWriteTransaction(ctx context.Context, tree *trillian.Tree, f storage.LogTXFunc) error {
	tx, err := m.beginInternal(ctx, tree, false /* readonly */)
	if err != nil && err != storage.ErrTreeNeedsInit {
		return err
	}
	defer tx.Close()
	if err := f(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (m *memoryLogStorage) AddSequencedLeaves(ctx context.Context, tree *trillian.Tree, leaves []*trillian.LogLeaf, timestamp time.Time) ([]*trillian.QueuedLogLeaf, error) {
	return nil, status.Errorf(codes.Unimplemented, "AddSequencedLeaves is not implemented")
}

func (m *memoryLogStorage) SnapshotForTree(ctx context.Context, tree *trillian.Tree) (storage.ReadOnlyLogTreeTX, error) {
	tx, err := m.beginInternal(ctx, tree, true /* readonly */)
	if err != nil {
		return nil, err
	}
	return tx, err
}

func (m *memoryLogStorage) QueueLeaves(ctx context.Context, tree *trillian.Tree, leaves []*trillian.LogLeaf, queueTimestamp time.Time) ([]*trillian.QueuedLogLeaf, error) {
	tx, err := m.beginInternal(ctx, tree, false /* readonly */)
	if tx != nil {
		// Ensure we don't leak the transaction. For example if we get an
		// ErrTreeNeedsInit from beginInternal() or if QueueLeaves fails
		// below.
		defer tx.Close()
	}
	if err != nil {
		return nil, err
	}
	existing, err := tx.QueueLeaves(ctx, leaves, queueTimestamp)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	ret := make([]*trillian.QueuedLogLeaf, len(leaves))
	for i, e := range existing {
		if e != nil {
			ret[i] = &trillian.QueuedLogLeaf{
				Leaf:   e,
				Status: status.Newf(codes.AlreadyExists, "leaf already exists: %v", e.LeafIdentityHash).Proto(),
			}
			continue
		}
		ret[i] = &trillian.QueuedLogLeaf{Leaf: leaves[i]}
	}
	return ret, nil
}

type logTreeTX struct {
	treeTX
	ls   *memoryLogStorage
	root types.LogRootV1
	slr  *trillian.SignedLogRoot
}

func (t *logTreeTX) WriteRevision(ctx context.Context) (int64, error) {
	if t.treeTX.writeRevision < 0 {
		return t.treeTX.writeRevision, errors.New("logTreeTX write revision not populated")
	}
	return t.treeTX.writeRevision, nil
}

// GetMerkleNodes returns the requested nodes at (or below) the read revision.
func (t *logTreeTX) GetMerkleNodes(ctx context.Context, ids []compact.NodeID) ([]stree.Node, error) {
	rev := int64(t.root.Revision)
	return t.treeTX.subtreeCache.GetNodes(ids, t.treeTX.getSubtreesAtRev(ctx, rev))
}

func (t *logTreeTX) DequeueLeaves(ctx context.Context, limit int, cutoffTime time.Time) ([]*trillian.LogLeaf, error) {
	leaves := make([]*trillian.LogLeaf, 0, limit)

	q := t.tx.Get(unseqKey(t.treeID)).(*kv).v.(*list.List)
	e := q.Front()
	for i := 0; i < limit && e != nil; i++ {
		// TODO(al): consider cutoffTime
		leaves = append(leaves, e.Value.(*trillian.LogLeaf))
		e = e.Next()
	}

	dequeuedCounter.Add(float64(len(leaves)), labelForTX(t))
	return leaves, nil
}

func (t *logTreeTX) QueueLeaves(ctx context.Context, leaves []*trillian.LogLeaf, queueTimestamp time.Time) ([]*trillian.LogLeaf, error) {
	// Don't accept batches if any of the leaves are invalid.
	for _, leaf := range leaves {
		if len(leaf.LeafIdentityHash) != t.hashSizeBytes {
			return nil, fmt.Errorf("queued leaf must have a leaf ID hash of length %d", t.hashSizeBytes)
		}
	}
	queuedCounter.Add(float64(len(leaves)), labelForTX(t))
	// No deduping in this storage!
	k := unseqKey(t.treeID)
	q := t.tx.Get(k).(*kv).v.(*list.List)
	for _, l := range leaves {
		q.PushBack(l)
	}
	return make([]*trillian.LogLeaf, len(leaves)), nil
}

func (t *logTreeTX) AddSequencedLeaves(ctx context.Context, leaves []*trillian.LogLeaf, timestamp time.Time) ([]*trillian.QueuedLogLeaf, error) {
	return nil, status.Errorf(codes.Unimplemented, "AddSequencedLeaves is not implemented")
}

func (t *logTreeTX) GetLeavesByRange(ctx context.Context, start, count int64) ([]*trillian.LogLeaf, error) {
	ret := make([]*trillian.LogLeaf, 0, count)
	for i := int64(0); i < count; i++ {
		leaf := t.tx.Get(seqLeafKey(t.treeID, start+i))
		if leaf != nil {
			ret = append(ret, leaf.(*kv).v.(*trillian.LogLeaf))
		}
	}
	return ret, nil
}

func (t *logTreeTX) GetLeavesByHash(ctx context.Context, leafHashes [][]byte, orderBySequence bool) ([]*trillian.LogLeaf, error) {
	m := t.tx.Get(hashToSeqKey(t.treeID)).(*kv).v.(map[string][]int64)

	ret := make([]*trillian.LogLeaf, 0, len(leafHashes))
	for _, hash := range leafHashes {
		seq, ok := m[string(hash)]
		if !ok {
			continue
		}
		for _, s := range seq {
			l := t.tx.Get(seqLeafKey(t.treeID, s))
			if l == nil {
				continue
			}
			ret = append(ret, l.(*kv).v.(*trillian.LogLeaf))
		}
	}
	return ret, nil
}

func (t *logTreeTX) LatestSignedLogRoot(ctx context.Context) (*trillian.SignedLogRoot, error) {
	return t.slr, nil
}

// fetchLatestRoot reads the latest SignedLogRoot from the DB and returns it.
func (t *logTreeTX) fetchLatestRoot(ctx context.Context) (*trillian.SignedLogRoot, error) {
	r := t.tx.Get(sthKey(t.treeID, t.tree.currentSTH))
	if r == nil {
		return nil, storage.ErrTreeNeedsInit
	}
	return r.(*kv).v.(*trillian.SignedLogRoot), nil
}

func (t *logTreeTX) StoreSignedLogRoot(ctx context.Context, slr *trillian.SignedLogRoot) error {
	var root types.LogRootV1
	if err := root.UnmarshalBinary(slr.LogRoot); err != nil {
		return err
	}
	k := sthKey(t.treeID, root.TimestampNanos)
	k.(*kv).v = slr
	t.tx.ReplaceOrInsert(k)

	// TODO(alcutter): this breaks the transactional model
	if root.TimestampNanos > t.tree.currentSTH {
		t.tree.currentSTH = root.TimestampNanos
	}
	return nil
}

func (t *logTreeTX) UpdateSequencedLeaves(ctx context.Context, leaves []*trillian.LogLeaf) error {
	countByMerkleHash := make(map[string]int)
	for _, leaf := range leaves {
		// This should fail on insert but catch it early
		if got, want := len(leaf.LeafIdentityHash), t.hashSizeBytes; got != want {
			return fmt.Errorf("sequenced leaf has incorrect hash size: got %v, want %v", got, want)
		}
		mh := string(leaf.MerkleLeafHash)
		countByMerkleHash[mh]++
		// insert sequenced leaf:
		k := seqLeafKey(t.treeID, leaf.LeafIndex)
		k.(*kv).v = leaf
		t.tx.ReplaceOrInsert(k)
		// update merkle-to-seq mapping:
		m := t.tx.Get(hashToSeqKey(t.treeID))
		l := m.(*kv).v.(map[string][]int64)[string(leaf.MerkleLeafHash)]
		l = append(l, leaf.LeafIndex)
		m.(*kv).v.(map[string][]int64)[string(leaf.MerkleLeafHash)] = l
	}

	q := t.tx.Get(unseqKey(t.treeID)).(*kv).v.(*list.List)
	toRemove := make([]*list.Element, 0, q.Len())
	for e := q.Front(); e != nil && len(countByMerkleHash) > 0; e = e.Next() {
		h := e.Value.(*trillian.LogLeaf).MerkleLeafHash
		mh := string(h)
		if countByMerkleHash[mh] > 0 {
			countByMerkleHash[mh]--
			toRemove = append(toRemove, e)
			if countByMerkleHash[mh] == 0 {
				delete(countByMerkleHash, mh)
			}
		}
	}
	for _, e := range toRemove {
		q.Remove(e)
	}

	if unknown := len(countByMerkleHash); unknown != 0 {
		return fmt.Errorf("attempted to update %d unknown leaves: %x", unknown, countByMerkleHash)
	}

	return nil
}

func (t *logTreeTX) GetActiveLogIDs(ctx context.Context) ([]int64, error) {
	return getActiveLogIDs(t.ts.trees), nil
}
