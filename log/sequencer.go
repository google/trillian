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
	"github.com/google/trillian/merkle/compact"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/types"
	"github.com/google/trillian/util/clock"

	tcrypto "github.com/google/trillian/crypto"
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
	seqCounter = mf.NewCounter("sequencer_sequenced", "Number of leaves sequenced", logIDLabel)
	seqMergeDelay = mf.NewHistogram("sequencer_merge_delay", "Delay between queuing and integration of leaves", logIDLabel)
}

// Sequencer instances are responsible for integrating new leaves into a single log.
// Leaves will be assigned unique sequence numbers when they are processed.
// There is no strong ordering guarantee but in general entries will be processed
// in order of submission to the log.
type Sequencer struct {
	hasher     hashers.LogHasher
	timeSource clock.TimeSource
	logStorage storage.LogStorage
	signer     *tcrypto.Signer
	qm         quota.Manager
}

// maxTreeDepth sets an upper limit on the size of Log trees.
// Note: We actually can't go beyond 2^63 entries because we use int64s,
// but we need to calculate tree depths from a multiple of 8 due to the
// subtree assumptions.
const maxTreeDepth = 64

// NewSequencer creates a new Sequencer instance for the specified inputs.
func NewSequencer(
	hasher hashers.LogHasher,
	timeSource clock.TimeSource,
	logStorage storage.LogStorage,
	signer *tcrypto.Signer,
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
func (s Sequencer) buildMerkleTreeFromStorageAtRoot(ctx context.Context, root *types.LogRootV1, tx storage.TreeTX) (*compact.Tree, error) {
	mt, err := compact.NewTreeWithState(s.hasher, int64(root.TreeSize), func(depth int, index int64) ([]byte, error) {
		nodeID, err := storage.NewNodeIDForTreeCoords(int64(depth), index, maxTreeDepth)
		if err != nil {
			return nil, fmt.Errorf("failed to create nodeID: %v", err)
		}
		nodes, err := tx.GetMerkleNodes(ctx, int64(root.Revision), []storage.NodeID{nodeID})

		if err != nil {
			return nil, fmt.Errorf("failed to get Merkle nodes: %v", err)
		}

		// We expect to get exactly one node here
		if nodes == nil || len(nodes) != 1 {
			return nil, fmt.Errorf("did not retrieve one node while loading compact Merkle tree, got %#v for ID %v@%v", nodes, nodeID.String(), root.Revision)
		}

		return nodes[0].Hash, nil
	}, root.RootHash)

	if err != nil {
		return nil, fmt.Errorf("%x: %v", s.signer.KeyHint, err)
	}
	return mt, nil
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

func (s Sequencer) updateCompactTree(mt *compact.Tree, leaves []*trillian.LogLeaf, label string) (map[string]storage.Node, error) {
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

func (s Sequencer) initMerkleTreeFromStorage(ctx context.Context, currentRoot *types.LogRootV1, tx storage.LogTreeTX) (*compact.Tree, error) {
	if currentRoot.TreeSize == 0 {
		return compact.NewTree(s.hasher), nil
	}

	// Initialize the compact tree state to match the latest root in the database
	return s.buildMerkleTreeFromStorageAtRoot(ctx, currentRoot, tx)
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
	treeSize   int64
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
		leaf.LeafIndex = s.treeSize + int64(i)
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
func (s Sequencer) IntegrateBatch(ctx context.Context, tree *trillian.Tree, limit int, guardWindow, maxRootDurationInterval time.Duration) (int, error) {
	start := s.timeSource.Now()
	label := strconv.FormatInt(tree.TreeId, 10)

	numLeaves := 0
	var newLogRoot *types.LogRootV1
	var newSLR *trillian.SignedLogRoot
	err := s.logStorage.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.LogTreeTX) error {
		stageStart := s.timeSource.Now()
		defer seqBatches.Inc(label)
		defer func() { seqLatency.Observe(clock.SecondsSince(s.timeSource, start), label) }()

		// Get the latest known root from storage
		sth, err := tx.LatestSignedLogRoot(ctx)
		if err != nil {
			return fmt.Errorf("%v: Sequencer failed to get latest root: %v", tree.TreeId, err)
		}
		// There is no trust boundary between the signer and the
		// database, so we skip signature verification.
		// TODO(gbelvin): Add signature checking as a santity check.
		var currentRoot types.LogRootV1
		if err := currentRoot.UnmarshalBinary(sth.LogRoot); err != nil {
			return fmt.Errorf("%v: Sequencer failed to unmarshal latest root: %v", tree.TreeId, err)
		}
		seqGetRootLatency.Observe(clock.SecondsSince(s.timeSource, stageStart), label)
		seqTreeSize.Set(float64(currentRoot.TreeSize), label)

		if currentRoot.RootHash == nil {
			glog.Warningf("%v: Fresh log - no previous TreeHeads exist.", tree.TreeId)
			return storage.ErrTreeNeedsInit
		}

		taskData := &sequencingTaskData{
			label:      label,
			treeSize:   int64(currentRoot.TreeSize),
			timeSource: s.timeSource,
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
			nowNanos := s.timeSource.Now().UnixNano()
			interval := time.Duration(nowNanos - int64(currentRoot.TimestampNanos))
			if maxRootDurationInterval == 0 || interval < maxRootDurationInterval {
				// We have nothing to integrate into the tree.
				glog.V(1).Infof("%v: No leaves sequenced in this signing operation", tree.TreeId)
				return nil
			}
			glog.Infof("%v: Force new root generation as %v since last root", tree.TreeId, interval)
		}

		stageStart = s.timeSource.Now()
		merkleTree, err := s.initMerkleTreeFromStorage(ctx, &currentRoot, tx)
		if err != nil {
			return err
		}
		seqInitTreeLatency.Observe(clock.SecondsSince(s.timeSource, stageStart), label)
		stageStart = s.timeSource.Now()

		// We've done all the reads, can now do the updates in the same transaction.
		// The schema should prevent multiple STHs being inserted with the same
		// revision number so it should not be possible for colliding updates to
		// commit.
		newVersion, err := tx.WriteRevision(ctx)
		if err != nil {
			return err
		}
		if got, want := newVersion, int64(currentRoot.Revision)+1; got != want {
			return fmt.Errorf("%v: got writeRevision of %v, but expected %v", tree.TreeId, got, want)
		}

		// Collate node updates.
		nodeMap, err := s.updateCompactTree(merkleTree, sequencedLeaves, label)
		if err != nil {
			return err
		}
		seqWriteTreeLatency.Observe(clock.SecondsSince(s.timeSource, stageStart), label)

		// Store the sequenced batch.
		if err := st.update(ctx, sequencedLeaves); err != nil {
			return err
		}
		stageStart = s.timeSource.Now()

		// Build objects for the nodes to be updated. Because we deduped via the map
		// each node can only be created / updated once in each tree revision and
		// they cannot conflict when we do the storage update.
		targetNodes, err := s.buildNodesFromNodeMap(nodeMap, newVersion)
		if err != nil {
			// Probably an internal error with map building, unexpected.
			return fmt.Errorf("%v: Failed to build target nodes in sequencer: %v", tree.TreeId, err)
		}

		// Now insert or update the nodes affected by the above, at the new tree
		// version.
		if err := tx.SetMerkleNodes(ctx, targetNodes); err != nil {
			return fmt.Errorf("%v: Sequencer failed to set Merkle nodes: %v", tree.TreeId, err)
		}
		seqSetNodesLatency.Observe(clock.SecondsSince(s.timeSource, stageStart), label)
		stageStart = s.timeSource.Now()

		// Create the log root ready for signing
		seqTreeSize.Set(float64(merkleTree.Size()), label)
		newLogRoot = &types.LogRootV1{
			RootHash:       merkleTree.CurrentRoot(),
			TimestampNanos: uint64(s.timeSource.Now().UnixNano()),
			TreeSize:       uint64(merkleTree.Size()),
			Revision:       uint64(newVersion),
		}

		if newLogRoot.TimestampNanos <= currentRoot.TimestampNanos {
			return fmt.Errorf("%v: refusing to sign root with timestamp earlier than previous root (%d <= %d)", tree.TreeId, newLogRoot.TimestampNanos, currentRoot.TimestampNanos)
		}

		newSLR, err = s.signer.SignLogRoot(newLogRoot)
		if err != nil {
			return fmt.Errorf("%v: signer failed to sign root: %v", tree.TreeId, err)
		}

		if err := tx.StoreSignedLogRoot(ctx, *newSLR); err != nil {
			return fmt.Errorf("%v: failed to write updated tree root: %v", tree.TreeId, err)
		}
		seqStoreRootLatency.Observe(clock.SecondsSince(s.timeSource, stageStart), label)

		return nil
	})
	if err != nil {
		return 0, err
	}

	// Let quota.Manager know about newly-sequenced entries.
	// All possibly influenced quotas are replenished: {Tree/Global, Read/Write}.
	// Implementations are tasked with filtering quotas that shouldn't be replenished.
	// TODO(codingllama): Consider adding a source-aware replenish method
	// (eg, qm.Replenish(ctx, tokens, specs, quota.SequencerSource)), so there's no ambiguity as to
	// where the tokens come from.
	if numLeaves > 0 {
		tokens := int(float64(numLeaves) * quotaIncreaseFactor())
		specs := []quota.Spec{
			{Group: quota.Tree, Kind: quota.Read, TreeID: tree.TreeId},
			{Group: quota.Tree, Kind: quota.Write, TreeID: tree.TreeId},
			{Group: quota.Global, Kind: quota.Read},
			{Group: quota.Global, Kind: quota.Write},
		}
		glog.V(2).Infof("%v: Replenishing %v tokens (numLeaves = %v)", tree.TreeId, tokens, numLeaves)
		err := s.qm.PutTokens(ctx, tokens, specs)
		if err != nil {
			glog.Warningf("%v: Failed to replenish %v tokens: %v", tree.TreeId, tokens, err)
		}
		quota.Metrics.IncReplenished(tokens, specs, err == nil)
	}

	seqCounter.Add(float64(numLeaves), label)
	if newSLR != nil {
		glog.Infof("%v: sequenced %v leaves, size %v, tree-revision %v", tree.TreeId, numLeaves, newLogRoot.TreeSize, newLogRoot.Revision)
	}
	return numLeaves, nil
}
