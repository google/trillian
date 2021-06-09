// Copyright 2016 Google LLC. All Rights Reserved.
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

package log

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/merkle/compact"
	"github.com/google/trillian/merkle/rfc6962/hasher"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/tree"
	"github.com/google/trillian/types"
	"github.com/google/trillian/util/clock"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const logIDLabel = "logid"

var (
	sequencerOnce          sync.Once
	seqBatches             monitoring.Counter
	seqTreeSize            monitoring.Gauge
	seqLatency             monitoring.Histogram
	seqDequeueLatency      monitoring.Histogram
	seqGetRootLatency      monitoring.Histogram
	seqInitTreeLatency     monitoring.Histogram
	seqWriteTreeLatency    monitoring.Histogram
	seqUpdateLeavesLatency monitoring.Histogram
	seqSetNodesLatency     monitoring.Histogram
	seqStoreRootLatency    monitoring.Histogram
	seqCounter             monitoring.Counter
	seqMergeDelay          monitoring.Histogram
	seqTimestamp           monitoring.Gauge

	// QuotaIncreaseFactor is the multiplier used for the number of tokens added back to
	// sequencing-based quotas. The resulting PutTokens call is equivalent to
	// "PutTokens(_, numLeaves * QuotaIncreaseFactor, _)".
	// A factor >1 adds resilience to token leakage, on the risk of a system that's overly
	// optimistic in face of true token shortages. The higher the factor, the higher the quota
	// "optimism" is. A factor that's too high (say, >1.5) is likely a sign that the quota
	// configuration should be changed instead.
	// A factor <1 WILL lead to token shortages, therefore it'll be normalized to 1.
	QuotaIncreaseFactor = 1.1
)

func quotaIncreaseFactor() float64 {
	if QuotaIncreaseFactor < 1 {
		QuotaIncreaseFactor = 1
		return 1
	}
	return QuotaIncreaseFactor
}

// InitMetrics sets up some metrics for this package. Must be called before calling IntegrateBatch.
// Can be called more than once, but only the first call has any effect.
// TODO(pavelkalinnikov): Create all metrics in this package together.
func InitMetrics(mf monitoring.MetricFactory) {
	sequencerOnce.Do(func() {
		if mf == nil {
			mf = monitoring.InertMetricFactory{}
		}
		quota.InitMetrics(mf)
		seqBatches = mf.NewCounter("sequencer_batches", "Number of sequencer batch operations", logIDLabel)
		seqTreeSize = mf.NewGauge("sequencer_tree_size", "Tree size of last SLR signed", logIDLabel)
		seqTimestamp = mf.NewGauge("sequencer_tree_timestamp", "Time of last SLR signed in ms since epoch", logIDLabel)
		seqLatency = mf.NewHistogram("sequencer_latency", "Latency of sequencer batch operation in seconds", logIDLabel)
		seqDequeueLatency = mf.NewHistogram("sequencer_latency_dequeue", "Latency of dequeue-leaves part of sequencer batch operation in seconds", logIDLabel)
		seqGetRootLatency = mf.NewHistogram("sequencer_latency_get_root", "Latency of get-root part of sequencer batch operation in seconds", logIDLabel)
		seqInitTreeLatency = mf.NewHistogram("sequencer_latency_init_tree", "Latency of init-tree part of sequencer batch operation in seconds", logIDLabel)
		seqWriteTreeLatency = mf.NewHistogram("sequencer_latency_write_tree", "Latency of write-tree part of sequencer batch operation in seconds", logIDLabel)
		seqUpdateLeavesLatency = mf.NewHistogram("sequencer_latency_update_leaves", "Latency of update-leaves part of sequencer batch operation in seconds", logIDLabel)
		seqSetNodesLatency = mf.NewHistogram("sequencer_latency_set_nodes", "Latency of set-nodes part of sequencer batch operation in seconds", logIDLabel)
		seqStoreRootLatency = mf.NewHistogram("sequencer_latency_store_root", "Latency of store-root part of sequencer batch operation in seconds", logIDLabel)
		seqCounter = mf.NewCounter("sequencer_sequenced", "Number of leaves sequenced", logIDLabel)
		seqMergeDelay = mf.NewHistogram("sequencer_merge_delay", "Delay between queuing and integration of leaves", logIDLabel)
	})
}

// initCompactRangeFromStorage builds a compact range that matches the latest
// data in the database. Ensures that the root hash matches the passed in root.
func initCompactRangeFromStorage(ctx context.Context, root *types.LogRootV1, tx storage.TreeTX) (*compact.Range, error) {
	fact := compact.RangeFactory{Hash: hasher.DefaultHasher.HashChildren}
	if root.TreeSize == 0 {
		return fact.NewEmptyRange(0), nil
	}

	ids := compact.RangeNodes(0, root.TreeSize)
	nodes, err := tx.GetMerkleNodes(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to read tree nodes: %v", err)
	}
	if got, want := len(nodes), len(ids); got != want {
		return nil, fmt.Errorf("failed to get %d nodes, got %d", want, got)
	}

	hashes := make([][]byte, len(nodes))
	for i, node := range nodes {
		hashes[i] = node.Hash
	}
	cr, err := fact.NewRange(0, root.TreeSize, hashes)
	if err != nil {
		return nil, fmt.Errorf("failed to create compact.Range: %v", err)
	}
	hash, err := cr.GetRootHash(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to compute the root hash: %v", err)
	}
	// Note: Tree size != 0 at this point, so we don't consider the empty hash.
	if want := root.RootHash; !bytes.Equal(hash, want) {
		return nil, fmt.Errorf("root hash mismatch: got %x, want %x", hash, want)
	}
	return cr, nil
}

func buildNodesFromNodeMap(nodeMap map[compact.NodeID][]byte) []tree.Node {
	nodes := make([]tree.Node, 0, len(nodeMap))
	for id, hash := range nodeMap {
		nodes = append(nodes, tree.Node{ID: id, Hash: hash})
	}
	return nodes
}

func prepareLeaves(leaves []*trillian.LogLeaf, begin uint64, label string, timeSource clock.TimeSource) error {
	now := timeSource.Now()
	integrateAt := timestamppb.New(now)
	if err := integrateAt.CheckValid(); err != nil {
		return fmt.Errorf("got invalid integrate timestamp: %w", err)
	}
	for i, leaf := range leaves {
		// The leaf should already have the correct index before it's integrated.
		if got, want := leaf.LeafIndex, begin+uint64(i); got < 0 || got != int64(want) {
			return fmt.Errorf("got invalid leaf index: %v, want: %v", got, want)
		}
		leaf.IntegrateTimestamp = integrateAt

		// Old leaves might not have a QueueTimestamp, only calculate the merge
		// delay if this one does.
		if leaf.QueueTimestamp != nil && leaf.QueueTimestamp.Seconds != 0 {
			if err := leaf.QueueTimestamp.CheckValid(); err != nil {
				return fmt.Errorf("got invalid queue timestamp: %w", err)
			}
			queueTS := leaf.QueueTimestamp.AsTime()
			mergeDelay := now.Sub(queueTS)
			seqMergeDelay.Observe(mergeDelay.Seconds(), label)
		}
	}
	return nil
}

// updateCompactRange adds the passed in leaves to the compact range. Returns a
// map of all updated tree nodes, and the new root hash.
func updateCompactRange(cr *compact.Range, leaves []*trillian.LogLeaf, label string) (map[compact.NodeID][]byte, []byte, error) {
	nodeMap := make(map[compact.NodeID][]byte)
	store := func(id compact.NodeID, hash []byte) { nodeMap[id] = hash }

	// Update the tree state by integrating the leaves one by one.
	for _, leaf := range leaves {
		idx := leaf.LeafIndex
		if size := cr.End(); idx < 0 || idx != int64(size) {
			return nil, nil, fmt.Errorf("leaf index mismatch: got %d, want %d", idx, size)
		}
		// Store the leaf hash in the Merkle tree.
		store(compact.NewNodeID(0, uint64(idx)), leaf.MerkleLeafHash)
		// Store all the new internal nodes.
		if err := cr.Append(leaf.MerkleLeafHash, store); err != nil {
			return nil, nil, err
		}
	}
	// Store ephemeral nodes on the right border of the tree as well.
	hash, err := cr.GetRootHash(store)
	if err != nil {
		return nil, nil, err
	}
	return nodeMap, hash, nil
}

// sequencingTask provides sequenced LogLeaf entries, and updates storage
// according to their ordering if needed.
type sequencingTask interface {
	// fetch returns a batch of sequenced entries obtained from storage, sized up
	// to the specified limit. The returned leaves have consecutive LeafIndex
	// values starting from the current tree size.
	fetch(ctx context.Context, limit int, cutoff time.Time) ([]*trillian.LogLeaf, error)

	// update makes sequencing persisted in storage, if not yet.
	update(ctx context.Context, leaves []*trillian.LogLeaf) error
}

type sequencingTaskData struct {
	label      string
	treeSize   uint64
	timeSource clock.TimeSource
	tx         storage.LogTreeTX
}

// logSequencingTask is a sequencingTask implementation for "normal" Log mode,
// which assigns consecutive sequence numbers to leaves as they are read from
// the pending unsequenced entries.
type logSequencingTask sequencingTaskData

func (s *logSequencingTask) fetch(ctx context.Context, limit int, cutoff time.Time) ([]*trillian.LogLeaf, error) {
	start := s.timeSource.Now()
	// Recent leaves inside the guard window will not be available for sequencing.
	leaves, err := s.tx.DequeueLeaves(ctx, limit, cutoff)
	if err != nil {
		return nil, fmt.Errorf("%v: Sequencer failed to dequeue leaves: %v", s.label, err)
	}
	seqDequeueLatency.Observe(clock.SecondsSince(s.timeSource, start), s.label)

	// Assign leaf sequence numbers.
	for i, leaf := range leaves {
		leaf.LeafIndex = int64(s.treeSize + uint64(i))
		if got := leaf.LeafIndex; got < 0 {
			return nil, fmt.Errorf("%v: leaf index overflow: %d", s.label, got)
		}
	}
	return leaves, nil
}

func (s *logSequencingTask) update(ctx context.Context, leaves []*trillian.LogLeaf) error {
	start := s.timeSource.Now()
	// Write the new sequence numbers to the leaves in the DB.
	if err := s.tx.UpdateSequencedLeaves(ctx, leaves); err != nil {
		return fmt.Errorf("%v: Sequencer failed to update sequenced leaves: %v", s.label, err)
	}
	seqUpdateLeavesLatency.Observe(clock.SecondsSince(s.timeSource, start), s.label)
	return nil
}

// preorderedLogSequencingTask is a sequencingTask implementation for
// Pre-ordered Log mode. It reads sequenced entries past the tree size which
// are already in the storage.
type preorderedLogSequencingTask sequencingTaskData

func (s *preorderedLogSequencingTask) fetch(ctx context.Context, limit int, cutoff time.Time) ([]*trillian.LogLeaf, error) {
	start := s.timeSource.Now()
	leaves, err := s.tx.DequeueLeaves(ctx, limit, cutoff)
	if err != nil {
		return nil, fmt.Errorf("%v: Sequencer failed to load sequenced leaves: %v", s.label, err)
	}
	seqDequeueLatency.Observe(clock.SecondsSince(s.timeSource, start), s.label)
	return leaves, nil
}

func (s *preorderedLogSequencingTask) update(ctx context.Context, leaves []*trillian.LogLeaf) error {
	// TODO(pavelkalinnikov): Update integration timestamps.
	return nil
}

// IntegrateBatch wraps up all the operations needed to take a batch of queued
// or sequenced leaves and integrate them into the tree.
func IntegrateBatch(ctx context.Context, tree *trillian.Tree, limit int, guardWindow, maxRootDurationInterval time.Duration, ts clock.TimeSource, ls storage.LogStorage, qm quota.Manager) (int, error) {
	start := ts.Now()
	label := strconv.FormatInt(tree.TreeId, 10)

	numLeaves := 0
	var newLogRoot *types.LogRootV1
	var newSLR *trillian.SignedLogRoot
	err := ls.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.LogTreeTX) error {
		stageStart := ts.Now()
		defer seqBatches.Inc(label)
		defer func() { seqLatency.Observe(clock.SecondsSince(ts, start), label) }()

		// Get the latest known root from storage
		sth, err := tx.LatestSignedLogRoot(ctx)
		if err != nil || sth == nil {
			return fmt.Errorf("%v: Sequencer failed to get latest root: %v", tree.TreeId, err)
		}
		// There is no trust boundary between the signer and the
		// database, so we skip signature verification.
		// TODO(gbelvin): Add signature checking as a santity check.
		var currentRoot types.LogRootV1
		if err := currentRoot.UnmarshalBinary(sth.LogRoot); err != nil {
			return fmt.Errorf("%v: Sequencer failed to unmarshal latest root: %v", tree.TreeId, err)
		}
		seqGetRootLatency.Observe(clock.SecondsSince(ts, stageStart), label)
		seqTreeSize.Set(float64(currentRoot.TreeSize), label)

		if currentRoot.RootHash == nil {
			glog.Warningf("%v: Fresh log - no previous TreeHeads exist.", tree.TreeId)
			return storage.ErrTreeNeedsInit
		}

		taskData := &sequencingTaskData{
			label:      label,
			treeSize:   currentRoot.TreeSize,
			timeSource: ts,
			tx:         tx,
		}
		var st sequencingTask
		switch tree.TreeType {
		case trillian.TreeType_LOG:
			st = (*logSequencingTask)(taskData)
		case trillian.TreeType_PREORDERED_LOG:
			st = (*preorderedLogSequencingTask)(taskData)
		default:
			return fmt.Errorf("IntegrateBatch not supported for TreeType %v", tree.TreeType)
		}

		sequencedLeaves, err := st.fetch(ctx, limit, start.Add(-guardWindow))
		if err != nil {
			return fmt.Errorf("%v: Sequencer failed to load sequenced batch: %v", tree.TreeId, err)
		}
		numLeaves = len(sequencedLeaves)

		// We need to create a signed root if entries were added or the latest root
		// is too old.
		if numLeaves == 0 {
			nowNanos := ts.Now().UnixNano()
			interval := time.Duration(nowNanos - int64(currentRoot.TimestampNanos))
			if maxRootDurationInterval == 0 || interval < maxRootDurationInterval {
				// We have nothing to integrate into the tree.
				glog.V(1).Infof("%v: No leaves sequenced in this signing operation", tree.TreeId)
				return nil
			}
			glog.Infof("%v: Force new root generation as %v since last root", tree.TreeId, interval)
		}

		stageStart = ts.Now()
		cr, err := initCompactRangeFromStorage(ctx, &currentRoot, tx)
		if err != nil {
			return fmt.Errorf("%v: compact range init failed: %v", tree.TreeId, err)
		}
		seqInitTreeLatency.Observe(clock.SecondsSince(ts, stageStart), label)
		stageStart = ts.Now()

		// We've done all the reads, can now do the updates in the same
		// transaction. Collate node updates.
		if err := prepareLeaves(sequencedLeaves, cr.End(), label, ts); err != nil {
			return err
		}
		nodeMap, newRoot, err := updateCompactRange(cr, sequencedLeaves, label)
		if err != nil {
			return err
		}
		seqWriteTreeLatency.Observe(clock.SecondsSince(ts, stageStart), label)

		// Store the sequenced batch.
		if err := st.update(ctx, sequencedLeaves); err != nil {
			return err
		}
		stageStart = ts.Now()

		// Build objects for the nodes to be updated. Because we deduped via the map
		// each node can only be created / updated once in each tree revision and
		// they cannot conflict when we do the storage update.
		targetNodes := buildNodesFromNodeMap(nodeMap)

		// Now insert or update the nodes affected by the above, at the new tree
		// version.
		if err := tx.SetMerkleNodes(ctx, targetNodes); err != nil {
			return fmt.Errorf("%v: Sequencer failed to set Merkle nodes: %v", tree.TreeId, err)
		}
		seqSetNodesLatency.Observe(clock.SecondsSince(ts, stageStart), label)
		stageStart = ts.Now()

		// Create the log root ready for signing.
		if cr.End() == 0 {
			// Override the nil root hash returned by the compact range.
			newRoot = hasher.DefaultHasher.EmptyRoot()
		}
		newLogRoot = &types.LogRootV1{
			RootHash:       newRoot,
			TimestampNanos: uint64(ts.Now().UnixNano()),
			TreeSize:       cr.End(),
		}
		seqTreeSize.Set(float64(newLogRoot.TreeSize), label)
		seqTimestamp.Set(float64(time.Duration(newLogRoot.TimestampNanos)*time.Nanosecond/
			time.Millisecond), label)

		if newLogRoot.TimestampNanos <= currentRoot.TimestampNanos {
			return fmt.Errorf("%v: refusing to sign root with timestamp earlier than previous root (%d <= %d)", tree.TreeId, newLogRoot.TimestampNanos, currentRoot.TimestampNanos)
		}

		logRoot, err := newLogRoot.MarshalBinary()
		if err != nil {
			return fmt.Errorf("%v: signer failed to marshal root: %v", tree.TreeId, err)
		}
		newSLR := &trillian.SignedLogRoot{LogRoot: logRoot}

		if err := tx.StoreSignedLogRoot(ctx, newSLR); err != nil {
			return fmt.Errorf("%v: failed to write updated tree root: %v", tree.TreeId, err)
		}
		seqStoreRootLatency.Observe(clock.SecondsSince(ts, stageStart), label)
		return nil
	})
	if err != nil {
		return 0, err
	}

	// Let quota.Manager know about newly-sequenced entries.
	replenishQuota(ctx, numLeaves, tree.TreeId, qm)

	seqCounter.Add(float64(numLeaves), label)
	if newSLR != nil {
		glog.Infof("%v: sequenced %v leaves, size %v", tree.TreeId, numLeaves, newLogRoot.TreeSize)
	}
	return numLeaves, nil
}

// replenishQuota replenishes all quotas, such as {Tree/Global, Read/Write},
// that are possibly influenced by sequencing numLeaves entries for the passed
// in tree ID. Implementations are tasked with filtering quotas that shouldn't
// be replenished.
//
// TODO(codingllama): Consider adding a source-aware replenish method (e.g.,
// qm.Replenish(ctx, tokens, specs, quota.SequencerSource)), so there's no
// ambiguity as to where the tokens come from.
func replenishQuota(ctx context.Context, numLeaves int, treeID int64, qm quota.Manager) {
	if numLeaves > 0 {
		tokens := int(float64(numLeaves) * quotaIncreaseFactor())
		specs := []quota.Spec{
			{Group: quota.Tree, Kind: quota.Read, TreeID: treeID},
			{Group: quota.Tree, Kind: quota.Write, TreeID: treeID},
			{Group: quota.Global, Kind: quota.Read},
			{Group: quota.Global, Kind: quota.Write},
		}
		glog.V(2).Infof("%v: replenishing %d tokens (numLeaves = %d)", treeID, tokens, numLeaves)
		err := qm.PutTokens(ctx, tokens, specs)
		if err != nil {
			glog.Warningf("%v: failed to replenish %d tokens: %v", treeID, tokens, err)
		}
		quota.Metrics.IncReplenished(tokens, specs, err == nil)
	}
}
