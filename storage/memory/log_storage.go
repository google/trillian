// Copyright 2017 Google Inc. All Rights Reserved.
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
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/google/btree"
	"github.com/google/trillian"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/cache"
	"github.com/google/trillian/trees"
)

const logIDLabel = "logid"

var (
	defaultLogStrata = []int{8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8}

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
func sthKey(treeID, timestamp int64) btree.Item {
	return &kv{k: fmt.Sprintf("/%d/sth/%020d", treeID, timestamp)}
}

type memoryLogStorage struct {
	*memoryTreeStorage
	admin         storage.AdminStorage
	metricFactory monitoring.MetricFactory
}

// NewLogStorage creates an in-memory LogStorage instance.
func NewLogStorage(mf monitoring.MetricFactory) storage.LogStorage {
	if mf == nil {
		mf = monitoring.InertMetricFactory{}
	}
	ret := &memoryLogStorage{
		memoryTreeStorage: newTreeStorage(),
		metricFactory:     mf,
	}
	ret.admin = NewAdminStorage(ret)
	return ret
}

func (m *memoryLogStorage) CheckDatabaseAccessible(ctx context.Context) error {
	return nil
}

type readOnlyLogTX struct {
	ms *memoryTreeStorage
}

func (m *memoryLogStorage) Snapshot(ctx context.Context) (storage.ReadOnlyLogTX, error) {
	return &readOnlyLogTX{m.memoryTreeStorage}, nil
}

func (t *readOnlyLogTX) Commit() error {
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

	ret := make([]int64, 0, len(t.ms.trees))
	for k := range t.ms.trees {
		ret = append(ret, k)
	}
	return ret, nil
}

func (m *memoryLogStorage) beginInternal(ctx context.Context, treeID int64, opts trees.GetOpts) (storage.LogTreeTX, error) {
	once.Do(func() {
		createMetrics(m.metricFactory)
	})
	if opts.TreeType != trillian.TreeType_LOG {
		return nil, fmt.Errorf("beginInternal tree id %d, got: %v, want TreeType_LOG,", treeID, opts.TreeType)
	}
	tree, err := trees.GetTree(ctx, storage.GetterFor(m.admin), treeID, opts)
	if err != nil {
		return nil, err
	}
	hasher, err := hashers.NewLogHasher(tree.HashStrategy)
	if err != nil {
		return nil, err
	}

	stCache := cache.NewLogSubtreeCache(defaultLogStrata, hasher)
	ttx, err := m.memoryTreeStorage.beginTreeTX(ctx, opts, treeID, hasher.Size(), stCache)
	if err != nil {
		return nil, err
	}

	ltx := &logTreeTX{
		treeTX: ttx,
		ls:     m,
	}

	ltx.root, err = ltx.fetchLatestRoot(ctx)
	if err != nil && err != storage.ErrTreeNeedsInit {
		ttx.Rollback()
		return nil, err
	}
	if err == storage.ErrTreeNeedsInit {
		return ltx, err
	}

	ltx.treeTX.writeRevision = ltx.root.TreeRevision + 1

	return ltx, nil
}

func (m *memoryLogStorage) BeginForTree(ctx context.Context, treeID int64, opts trees.GetOpts) (storage.LogTreeTX, error) {
	return m.beginInternal(ctx, treeID, opts)
}

func (m *memoryLogStorage) SnapshotForTree(ctx context.Context, treeID int64, opts trees.GetOpts) (storage.ReadOnlyLogTreeTX, error) {
	if !opts.Readonly {
		return nil, errors.New("memory SnapshotForTree(): got: readonly false, want true")
	}
	tx, err := m.beginInternal(ctx, treeID, opts)
	if err != nil {
		return nil, err
	}
	return tx.(storage.ReadOnlyLogTreeTX), err
}

type logTreeTX struct {
	treeTX
	ls   *memoryLogStorage
	root trillian.SignedLogRoot
}

func (t *logTreeTX) ReadRevision() int64 {
	return t.root.TreeRevision
}

func (t *logTreeTX) WriteRevision() int64 {
	return t.treeTX.writeRevision
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
	return []*trillian.LogLeaf{}, nil
}

func (t *logTreeTX) GetSequencedLeafCount(ctx context.Context) (int64, error) {
	var sequencedLeafCount int64

	t.tx.DescendRange(seqLeafKey(t.treeID, math.MaxInt64), seqLeafKey(t.treeID, 0), func(i btree.Item) bool {
		sequencedLeafCount = i.(*kv).v.(*trillian.LogLeaf).LeafIndex + 1
		return false
	})
	return sequencedLeafCount, nil
}

func (t *logTreeTX) GetLeavesByIndex(ctx context.Context, leaves []int64) ([]*trillian.LogLeaf, error) {
	ret := make([]*trillian.LogLeaf, 0, len(leaves))
	for _, seq := range leaves {
		leaf := t.tx.Get(seqLeafKey(t.treeID, seq))
		if leaf != nil {
			ret = append(ret, leaf.(*kv).v.(*trillian.LogLeaf))
		}
	}
	return ret, nil
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
	for hash := range leafHashes {
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

func (t *logTreeTX) LatestSignedLogRoot(ctx context.Context) (trillian.SignedLogRoot, error) {
	return t.root, nil
}

// fetchLatestRoot reads the latest SignedLogRoot from the DB and returns it.
func (t *logTreeTX) fetchLatestRoot(ctx context.Context) (trillian.SignedLogRoot, error) {
	r := t.tx.Get(sthKey(t.treeID, t.tree.currentSTH))
	if r == nil {
		return trillian.SignedLogRoot{}, storage.ErrTreeNeedsInit
	}
	return r.(*kv).v.(trillian.SignedLogRoot), nil
}

func (t *logTreeTX) StoreSignedLogRoot(ctx context.Context, root trillian.SignedLogRoot) error {
	k := sthKey(t.treeID, root.TimestampNanos)
	k.(*kv).v = root
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

func (t *logTreeTX) getActiveLogIDs(ctx context.Context) ([]int64, error) {
	var ret []int64
	for k := range t.ts.trees {
		ret = append(ret, k)
	}
	return ret, nil
}

// GetActiveLogIDs returns a list of the IDs of all configured logs
func (t *logTreeTX) GetActiveLogIDs(ctx context.Context) ([]int64, error) {
	return t.getActiveLogIDs(ctx)
}

func (t *readOnlyLogTX) GetUnsequencedCounts(ctx context.Context) (storage.CountByLogID, error) {
	t.ms.mu.RLock()
	defer t.ms.mu.RUnlock()

	ret := make(map[int64]int64)
	for id, tree := range t.ms.trees {
		tree.RLock()
		defer tree.RUnlock() // OK to hold until method returns.

		k := unseqKey(id)
		queue := tree.store.Get(k).(*kv).v.(*list.List)
		ret[id] = int64(queue.Len())
	}
	return ret, nil
}
