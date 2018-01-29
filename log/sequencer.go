// Copyright 2016 Google Inc. All Rights Reserved.
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

// Package log includes code that is specific to Trillian's log mode, particularly code
// for running sequencing operations.
package log

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/util"
)

const logIDLabel = "logid"

var (
	once                   sync.Once
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
	seqCommitLatency       monitoring.Histogram
	seqCounter             monitoring.Counter
	seqMergeDelay          monitoring.Histogram

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

func createMetrics(mf monitoring.MetricFactory) {
	if mf == nil {
		mf = monitoring.InertMetricFactory{}
	}
	quota.InitMetrics(mf)
	seqBatches = mf.NewCounter("sequencer_batches", "Number of sequencer batch operations", logIDLabel)
	seqTreeSize = mf.NewGauge("sequencer_tree_size", "Size of Merkle tree", logIDLabel)
	seqLatency = mf.NewHistogram("sequencer_latency", "Latency of sequencer batch operation in seconds", logIDLabel)
	seqDequeueLatency = mf.NewHistogram("sequencer_latency_dequeue", "Latency of dequeue-leaves part of sequencer batch operation in seconds", logIDLabel)
	seqGetRootLatency = mf.NewHistogram("sequencer_latency_get_root", "Latency of get-root part of sequencer batch operation in seconds", logIDLabel)
	seqInitTreeLatency = mf.NewHistogram("sequencer_latency_init_tree", "Latency of init-tree part of sequencer batch operation in seconds", logIDLabel)
	seqWriteTreeLatency = mf.NewHistogram("sequencer_latency_write_tree", "Latency of write-tree part of sequencer batch operation in seconds", logIDLabel)
	seqUpdateLeavesLatency = mf.NewHistogram("sequencer_latency_update_leaves", "Latency of update-leaves part of sequencer batch operation in seconds", logIDLabel)
	seqSetNodesLatency = mf.NewHistogram("sequencer_latency_set_nodes", "Latency of set-nodes part of sequencer batch operation in seconds", logIDLabel)
	seqStoreRootLatency = mf.NewHistogram("sequencer_latency_store_root", "Latency of store-root part of sequencer batch operation in seconds", logIDLabel)
	seqCommitLatency = mf.NewHistogram("sequencer_latency_commit", "Latency of commit part of sequencer batch operation in seconds", logIDLabel)
	seqCounter = mf.NewCounter("sequencer_sequenced", "Number of leaves sequenced", logIDLabel)
	seqMergeDelay = mf.NewHistogram("sequencer_merge_delay", "Delay between queuing and integration of leaves", logIDLabel)
}

// TODO(Martin2112): Add admin support for safely changing params like guard window during operation
// TODO(Martin2112): Add support for enabling and controlling sequencing as part of admin API

// Sequencer instances are responsible for integrating new leaves into a single log.
// Leaves will be assigned unique sequence numbers when they are processed.
// There is no strong ordering guarantee but in general entries will be processed
// in order of submission to the log.
type Sequencer struct {
	hasher     hashers.LogHasher
	timeSource util.TimeSource
	logStorage storage.LogStorage
	signer     *crypto.Signer
	qm         quota.Manager
}

// maxTreeDepth sets an upper limit on the size of Log trees.
// TODO(al): We actually can't go beyond 2^63 entries because we use int64s,
//           but we need to calculate tree depths from a multiple of 8 due to
//           the subtrees.
const maxTreeDepth = 64

// NewSequencer creates a new Sequencer instance for the specified inputs.
func NewSequencer(
	hasher hashers.LogHasher,
	timeSource util.TimeSource,
	logStorage storage.LogStorage,
	signer *crypto.Signer,
	mf monitoring.MetricFactory,
	qm quota.Manager) *Sequencer {
	once.Do(func() {
		createMetrics(mf)
	})
	return &Sequencer{
		hasher:     hasher,
		timeSource: timeSource,
		logStorage: logStorage,
		signer:     signer,
		qm:         qm,
	}
}

// TODO: This currently doesn't use the batch api for fetching the required nodes. This
// would be more efficient but requires refactoring.
func (s Sequencer) buildMerkleTreeFromStorageAtRoot(ctx context.Context, root trillian.SignedLogRoot, tx storage.TreeTX) (*merkle.CompactMerkleTree, error) {
	mt, err := merkle.NewCompactMerkleTreeWithState(s.hasher, root.TreeSize, func(depth int, index int64) ([]byte, error) {
		nodeID, err := storage.NewNodeIDForTreeCoords(int64(depth), index, maxTreeDepth)
		if err != nil {
			glog.Warningf("%v: Failed to create nodeID: %v", root.LogId, err)
			return nil, err
		}
		nodes, err := tx.GetMerkleNodes(ctx, root.TreeRevision, []storage.NodeID{nodeID})

		if err != nil {
			glog.Warningf("%v: Failed to get Merkle nodes: %v", root.LogId, err)
			return nil, err
		}

		// We expect to get exactly one node here
		if nodes == nil || len(nodes) != 1 {
			return nil, fmt.Errorf("%v: Did not retrieve one node while loading CompactMerkleTree, got %#v for ID %v@%v", root.LogId, nodes, nodeID.String(), root.TreeRevision)
		}

		return nodes[0].Hash, nil
	}, root.RootHash)

	return mt, err
}

func (s Sequencer) buildNodesFromNodeMap(nodeMap map[string]storage.Node, newVersion int64) ([]storage.Node, error) {
	targetNodes := make([]storage.Node, len(nodeMap))
	i := 0
	for _, node := range nodeMap {
		node.NodeRevision = newVersion
		targetNodes[i] = node
		i++
	}
	return targetNodes, nil
}

func (s Sequencer) updateCompactTree(mt *merkle.CompactMerkleTree, leaves []*trillian.LogLeaf, label string) (map[string]storage.Node, error) {
	nodeMap := make(map[string]storage.Node)
	// Update the tree state by integrating the leaves one by one.
	for _, leaf := range leaves {
		seq, err := mt.AddLeafHash(leaf.MerkleLeafHash, func(depth int, index int64, hash []byte) error {
			nodeID, err := storage.NewNodeIDForTreeCoords(int64(depth), index, maxTreeDepth)
			if err != nil {
				return err
			}
			nodeMap[nodeID.String()] = storage.Node{
				NodeID: nodeID,
				Hash:   hash,
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		// The leaf should already have the correct index before it's integrated.
		if leaf.LeafIndex != seq {
			return nil, fmt.Errorf("got invalid leaf index: %v, want: %v", leaf.LeafIndex, seq)
		}
		integrateTS := s.timeSource.Now()
		leaf.IntegrateTimestamp, err = ptypes.TimestampProto(integrateTS)
		if err != nil {
			return nil, fmt.Errorf("got invalid integrate timestamp: %v", err)
		}

		// Old leaves might not have a QueueTimestamp, only calculate the merge delay if this one does.
		if leaf.QueueTimestamp != nil && leaf.QueueTimestamp.Seconds != 0 {
			queueTS, err := ptypes.Timestamp(leaf.QueueTimestamp)
			if err != nil {
				return nil, fmt.Errorf("got invalid queue timestamp: %v", queueTS)
			}
			mergeDelay := integrateTS.Sub(queueTS)
			seqMergeDelay.Observe(mergeDelay.Seconds(), label)
		}

		// Store leaf hash in the Merkle tree too:
		leafNodeID, err := storage.NewNodeIDForTreeCoords(0, seq, maxTreeDepth)
		if err != nil {
			return nil, err
		}
		nodeMap[leafNodeID.String()] = storage.Node{
			NodeID: leafNodeID,
			Hash:   leaf.MerkleLeafHash,
		}
	}

	return nodeMap, nil
}

func (s Sequencer) initMerkleTreeFromStorage(ctx context.Context, currentRoot trillian.SignedLogRoot, tx storage.LogTreeTX) (*merkle.CompactMerkleTree, error) {
	if currentRoot.TreeSize == 0 {
		return merkle.NewCompactMerkleTree(s.hasher), nil
	}

	// Initialize the compact tree state to match the latest root in the database
	return s.buildMerkleTreeFromStorageAtRoot(ctx, currentRoot, tx)
}

// sequencingTask provides sequenced LogLeaf entries, and updates storage
// according to their ordering if needed.
type sequencingTask interface {
	// fetch returns a batch of sequenced entries obtained from storage. The
	// returned leaves have consecutive LeafIndex values starting from the
	// current tree size.
	fetch(ctx context.Context, limit int, cutoff time.Time) ([]*trillian.LogLeaf, error)

	// update makes sequencing persisted in storage, if not yet.
	update(ctx context.Context, leaves []*trillian.LogLeaf) error
}

// logSequencingTask is a sequencingTask implementation for "normal" Log mode,
// which assigns consecutive sequence numbers to leaves as they are read from
// the pending unsequenced entries.
//
// TODO(pavelkalinnikov): Implement it for Pre-ordered Log mode similarly.
type logSequencingTask struct {
	label      string
	treeSize   int64
	timeSource util.TimeSource
	dequeuer   storage.LogTreeTX
}

func (s logSequencingTask) fetch(ctx context.Context, limit int, cutoff time.Time) ([]*trillian.LogLeaf, error) {
	start := s.timeSource.Now()
	// Recent leaves inside the guard window will not be available for sequencing.
	leaves, err := s.dequeuer.DequeueLeaves(ctx, limit, cutoff)
	if err != nil {
		glog.Warningf("%v: Sequencer failed to dequeue leaves: %v", s.label, err)
		return nil, err
	}
	seqDequeueLatency.Observe(util.SecondsSince(s.timeSource, start), s.label)

	// Assign leaf sequence numbers.
	for i, leaf := range leaves {
		leaf.LeafIndex = s.treeSize + int64(i)
	}
	return leaves, nil
}

func (s logSequencingTask) update(ctx context.Context, leaves []*trillian.LogLeaf) error {
	start := s.timeSource.Now()
	// Write the new sequence numbers to the leaves in the DB.
	if err := s.dequeuer.UpdateSequencedLeaves(ctx, leaves); err != nil {
		glog.Warningf("%v: Sequencer failed to update sequenced leaves: %v", s.label, err)
		return err
	}
	seqUpdateLeavesLatency.Observe(util.SecondsSince(s.timeSource, start), s.label)
	return nil
}

// IntegrateBatch wraps up all the operations needed to take a batch of queued
// leaves and integrate them into the tree.
func (s Sequencer) IntegrateBatch(ctx context.Context, logID int64, limit int, guardWindow, maxRootDurationInterval time.Duration) (int, error) {
	start := s.timeSource.Now()
	label := strconv.FormatInt(logID, 10)

	numLeaves := 0
	var newLogRoot *trillian.SignedLogRoot
	err := s.logStorage.ReadWriteTransaction(ctx, logID, func(ctx context.Context, tx storage.LogTreeTX) error {
		defer seqBatches.Inc(label)
		defer func() { seqLatency.Observe(util.SecondsSince(s.timeSource, start), label) }()

		// Very recent leaves inside the guard window will not be available for sequencing
		guardCutoffTime := s.timeSource.Now().Add(-guardWindow)
		leaves, err := tx.DequeueLeaves(ctx, limit, guardCutoffTime)
		if err != nil {
			glog.Warningf("%v: Sequencer failed to dequeue leaves: %v", logID, err)
			return err
		}
		seqDequeueLatency.Observe(util.SecondsSince(s.timeSource, stageStart), label)
		stageStart = s.timeSource.Now()

		// Get the latest known root from storage
		currentRoot, err := tx.LatestSignedLogRoot(ctx)
		if err != nil {
			glog.Warningf("%v: Sequencer failed to get latest root: %v", logID, err)
			return err
		}
		if currentRoot.RootHash == nil {
			glog.Warningf("%v: Fresh log - no previous TreeHeads exist.", logID)
			return storage.ErrTreeNeedsInit
		}
		seqGetRootLatency.Observe(util.SecondsSince(s.timeSource, stageStart), label)
		stageStart = s.timeSource.Now()

		var st sequencingTask = logSequencingTask{
			label:      label,
			treeSize:   currentRoot.TreeSize,
			timeSource: s.timeSource,
			dequeuer:   tx,
		}
		sequencedLeaves, err := st.fetch(ctx, limit, start.Add(-guardWindow))
		if err != nil {
			glog.Warningf("%v: Sequencer failed to load sequenced batch: %v", logID, err)
			return 0, err
		}
		numLeaves := len(sequencedLeaves)

		// We need to create a signed root if entries were added or the latest root
		// is too old.
		if numLeaves == 0 {
			nowNanos := s.timeSource.Now().UnixNano()
			interval := time.Duration(nowNanos - currentRoot.TimestampNanos)
			if maxRootDurationInterval == 0 || interval < maxRootDurationInterval {
				// We have nothing to integrate into the tree.
				glog.V(1).Infof("%v: No leaves sequenced in this signing operation", logID)
				return 0, tx.Commit()
			}
		}

		stageStart = s.timeSource.Now()
		merkleTree, err := s.initMerkleTreeFromStorage(ctx, currentRoot, tx)
		if err != nil {
			return 0, err
		}
		seqInitTreeLatency.Observe(util.SecondsSince(s.timeSource, stageStart), label)
		stageStart = s.timeSource.Now()

		// We've done all the reads, can now do the updates in the same transaction.
		// The schema should prevent multiple STHs being inserted with the same
		// revision number so it should not be possible for colliding updates to
		// commit.
		newVersion := tx.WriteRevision()
		if got, want := newVersion, currentRoot.TreeRevision+int64(1); got != want {
			return 0, fmt.Errorf("%v: got writeRevision of %v, but expected %v", logID, got, want)
		}

		// Collate node updates.
		nodeMap, err := s.updateCompactTree(merkleTree, sequencedLeaves, label)
		if err != nil {
			return 0, err
		}
		seqWriteTreeLatency.Observe(util.SecondsSince(s.timeSource, stageStart), label)

		// Store the sequenced batch.
		if err := st.update(ctx, sequencedLeaves); err != nil {
			return 0, err
		}
		stageStart = s.timeSource.Now()

		// Build objects for the nodes to be updated. Because we deduped via the map
		// each node can only be created / updated once in each tree revision and
		// they cannot conflict when we do the storage update.
		targetNodes, err := s.buildNodesFromNodeMap(nodeMap, newVersion)
		if err != nil {
			// Probably an internal error with map building, unexpected.
			glog.Warningf("%v: Failed to build target nodes in sequencer: %v", logID, err)
			return 0, err
		}

		// Now insert or update the nodes affected by the above, at the new tree
		// version.
		if err := tx.SetMerkleNodes(ctx, targetNodes); err != nil {
			glog.Warningf("%v: Sequencer failed to set Merkle nodes: %v", logID, err)
			return 0, err
		}
		seqSetNodesLatency.Observe(util.SecondsSince(s.timeSource, stageStart), label)
		stageStart = s.timeSource.Now()

		// Create the log root ready for signing
		seqTreeSize.Set(float64(merkleTree.Size()), label)
		newLogRoot := &trillian.SignedLogRoot{
			RootHash:       merkleTree.CurrentRoot(),
			TimestampNanos: s.timeSource.Now().UnixNano(),
			TreeSize:       merkleTree.Size(),
			LogId:          currentRoot.LogId,
			TreeRevision:   newVersion,
		}
		sig, err := s.signer.SignLogRoot(newLogRoot)
		if err != nil {
			glog.Warningf("%v: signer failed to sign root: %v", logID, err)
			return 0, err
		}
		newLogRoot.Signature = sig

		// Now insert or update the nodes affected by the above, at the new tree version
		if err := tx.SetMerkleNodes(ctx, targetNodes); err != nil {
			glog.Warningf("%v: Sequencer failed to set Merkle nodes: %v", logID, err)
			return err
		}
		seqSetNodesLatency.Observe(util.SecondsSince(s.timeSource, stageStart), label)
		stageStart = s.timeSource.Now()

		// Create the log root ready for signing
		seqTreeSize.Set(float64(merkleTree.Size()), label)
		newLogRoot = &trillian.SignedLogRoot{
			RootHash:       merkleTree.CurrentRoot(),
			TimestampNanos: s.timeSource.Now().UnixNano(),
			TreeSize:       merkleTree.Size(),
			LogId:          currentRoot.LogId,
			TreeRevision:   newVersion,
		}
		sig, err := s.signer.SignLogRoot(newLogRoot)
		if err != nil {
			glog.Warningf("%v: signer failed to sign root: %v", logID, err)
			return err
		}
		newLogRoot.Signature = sig

		if err := tx.StoreSignedLogRoot(ctx, *newLogRoot); err != nil {
			glog.Warningf("%v: failed to write updated tree root: %v", logID, err)
			return err
		}
		seqStoreRootLatency.Observe(util.SecondsSince(s.timeSource, stageStart), label)
		stageStart = s.timeSource.Now()

		// The batch is now fully sequenced and we're done
		return tx.Commit()
	})
	if err != nil {
		return 0, err
	}
	seqCommitLatency.Observe(util.SecondsSince(s.timeSource, stageStart), label)

	// Let quota.Manager know about newly-sequenced entries.
	// All possibly influenced quotas are replenished: {Tree/Global, Read/Write}.
	// Implementations are tasked with filtering quotas that shouldn't be replenished.
	// TODO(codingllama): Consider adding a source-aware replenish method
	// (eg, qm.Replenish(ctx, tokens, specs, quota.SequencerSource)), so there's no ambiguity as to
	// where the tokens come from.
	if numLeaves > 0 {
		tokens := int(float64(numLeaves) * quotaIncreaseFactor())
		specs := []quota.Spec{
			{Group: quota.Tree, Kind: quota.Read, TreeID: logID},
			{Group: quota.Tree, Kind: quota.Write, TreeID: logID},
			{Group: quota.Global, Kind: quota.Read},
			{Group: quota.Global, Kind: quota.Write},
		}
		glog.V(2).Infof("%v: Replenishing %v tokens (numLeaves = %v)", logID, tokens, numLeaves)
		err := s.qm.PutTokens(ctx, tokens, specs)
		if err != nil {
			glog.Warningf("%v: Failed to replenish %v tokens: %v", logID, tokens, err)
		}
		quota.Metrics.IncReplenished(tokens, specs, err == nil)
	}

	seqCounter.Add(float64(numLeaves), label)
	if newLogRoot != nil {
		glog.Infof("%v: sequenced %v leaves, size %v, tree-revision %v", logID, numLeaves, newLogRoot.TreeSize, newLogRoot.TreeRevision)
	} else {
		glog.Info("newLogRoot = nil")
	}
	return numLeaves, nil
}

// SignRoot wraps up all the operations for creating a new log signed root.
func (s Sequencer) SignRoot(ctx context.Context, logID int64) error {
	return s.logStorage.ReadWriteTransaction(ctx, logID, func(ctx context.Context, tx storage.LogTreeTX) error {
		// Get the latest known root from storage
		currentRoot, err := tx.LatestSignedLogRoot(ctx)
		if err != nil {
			glog.Warningf("%v: signer failed to get latest root: %v", logID, err)
			return err
		}

		// Initialize a Merkle Tree from the state in storage. This should fail if the tree is
		// in a corrupt state.
		merkleTree, err := s.initMerkleTreeFromStorage(ctx, currentRoot, tx)
		if err != nil {
			return err
		}
		newLogRoot := &trillian.SignedLogRoot{
			RootHash:       merkleTree.CurrentRoot(),
			TimestampNanos: s.timeSource.Now().UnixNano(),
			TreeSize:       merkleTree.Size(),
			LogId:          currentRoot.LogId,
			TreeRevision:   currentRoot.TreeRevision + 1,
		}
		sig, err := s.signer.SignLogRoot(newLogRoot)
		if err != nil {
			glog.Warningf("%v: signer failed to sign root: %v", logID, err)
			return err
		}
		newLogRoot.Signature = sig

		// Store the new root and we're done
		if err := tx.StoreSignedLogRoot(ctx, *newLogRoot); err != nil {
			glog.Warningf("%v: signer failed to write updated root: %v", logID, err)
			return err
		}
		glog.V(2).Infof("%v: new signed root, size %v, tree-revision %v", logID, newLogRoot.TreeSize, newLogRoot.TreeRevision)

		return tx.Commit()
	})
}
