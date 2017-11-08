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
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/crypto/sigpb"
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

func (s Sequencer) sequenceLeaves(mt *merkle.CompactMerkleTree, leaves []*trillian.LogLeaf) (map[string]storage.Node, []*trillian.LogLeaf, error) {
	nodeMap := make(map[string]storage.Node)
	// Update the tree state and sequence the leaves and assign sequence numbers to the new leaves
	for i, leaf := range leaves {
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
			return nil, nil, err
		}
		// The leaf has now been sequenced.
		leaves[i].LeafIndex = seq
		// Store leaf hash in the Merkle tree too:
		leafNodeID, err := storage.NewNodeIDForTreeCoords(0, seq, maxTreeDepth)
		if err != nil {
			return nil, nil, err
		}
		nodeMap[leafNodeID.String()] = storage.Node{
			NodeID: leafNodeID,
			Hash:   leaf.MerkleLeafHash,
		}
	}

	return nodeMap, leaves, nil
}

func (s Sequencer) initMerkleTreeFromStorage(ctx context.Context, currentRoot trillian.SignedLogRoot, tx storage.LogTreeTX) (*merkle.CompactMerkleTree, error) {
	if currentRoot.TreeSize == 0 {
		return merkle.NewCompactMerkleTree(s.hasher), nil
	}

	// Initialize the compact tree state to match the latest root in the database
	return s.buildMerkleTreeFromStorageAtRoot(ctx, currentRoot, tx)
}

func (s Sequencer) createRootSignature(ctx context.Context, root trillian.SignedLogRoot) (*sigpb.DigitallySigned, error) {
	hash, err := crypto.HashLogRoot(root)
	if err != nil {
		return nil, err
	}
	signature, err := s.signer.Sign(hash)
	if err != nil {
		glog.Warningf("%v: signer failed to sign root: %v", root.LogId, err)
		return nil, err
	}

	return signature, nil
}

// SequenceBatch wraps up all the operations needed to take a batch of queued leaves
// and integrate them into the tree.
// TODO(Martin2112): Can possibly improve by deferring a function that attempts to rollback,
// which will fail if the tx was committed. Should only do this if we can hide the details of
// the underlying storage transactions and it doesn't create other problems.
func (s Sequencer) SequenceBatch(ctx context.Context, logID int64, limit int, guardWindow, maxRootDurationInterval time.Duration) (int, error) {
	start := s.timeSource.Now()
	stageStart := start
	label := strconv.FormatInt(logID, 10)
	tx, err := s.logStorage.BeginForTree(ctx, logID)
	if err != nil {
		glog.Warningf("%v: Sequencer failed to start tx: %v", logID, err)
		return 0, err
	}
	defer tx.Close()
	defer seqBatches.Inc(label)
	defer func() { seqLatency.Observe(s.since(start), label) }()

	// Very recent leaves inside the guard window will not be available for sequencing
	guardCutoffTime := s.timeSource.Now().Add(-guardWindow)
	leaves, err := tx.DequeueLeaves(ctx, limit, guardCutoffTime)
	if err != nil {
		glog.Warningf("%v: Sequencer failed to dequeue leaves: %v", logID, err)
		return 0, err
	}
	seqDequeueLatency.Observe(s.since(stageStart), label)
	stageStart = s.timeSource.Now()

	// Get the latest known root from storage
	currentRoot, err := tx.LatestSignedLogRoot(ctx)
	if err != nil {
		glog.Warningf("%v: Sequencer failed to get latest root: %v", logID, err)
		return 0, err
	}
	seqGetRootLatency.Observe(s.since(stageStart), label)
	stageStart = s.timeSource.Now()

	// TODO(al): Have a better detection mechanism for there being no stored root.
	// TODO(mhs): Might be better to create empty root in provisioning API when it exists
	if currentRoot.RootHash == nil {
		glog.Warningf("%v: Fresh log - no previous TreeHeads exist.", logID)
		// SignRoot starts a new transaction, and we've got one open here until
		// this function returns.
		// This explicit Close() is a work-around for the in-memory storage which
		// locks the tree for each TX.
		// TODO(al): Producing the first signed root for a new tree should be
		// handled by the provisioning, move it there.
		tx.Close()
		return 0, s.SignRoot(ctx, logID)
	}

	// There might be no work to be done. But we possibly still need to create an signed root if the
	// current one is too old. If there's work to be done then we'll be creating a root anyway.
	numLeaves := len(leaves)
	if numLeaves == 0 {
		nowNanos := s.timeSource.Now().UnixNano()
		interval := time.Duration(nowNanos - currentRoot.TimestampNanos)
		if maxRootDurationInterval == 0 || interval < maxRootDurationInterval {
			// We have nothing to integrate into the tree
			glog.V(1).Infof("%v: No leaves sequenced in this signing operation", logID)
			return 0, tx.Commit()
		}
		glog.Infof("%v: Force new root generation as %v since last root", logID, interval)
	}

	merkleTree, err := s.initMerkleTreeFromStorage(ctx, currentRoot, tx)
	if err != nil {
		return 0, err
	}
	seqInitTreeLatency.Observe(s.since(stageStart), label)
	stageStart = s.timeSource.Now()

	// We've done all the reads, can now do the updates in the same transaction.
	// The schema should prevent multiple STHs being inserted with the same revision
	// number so it should not be possible for colliding updates to commit.
	newVersion := tx.WriteRevision()
	if got, want := newVersion, currentRoot.TreeRevision+int64(1); got != want {
		return 0, fmt.Errorf("%v: got writeRevision of %v, but expected %v", logID, got, want)
	}

	// Assign leaf sequence numbers and collate node updates
	nodeMap, sequencedLeaves, err := s.sequenceLeaves(merkleTree, leaves)
	if err != nil {
		return 0, err
	}
	seqWriteTreeLatency.Observe(s.since(stageStart), label)
	stageStart = s.timeSource.Now()

	// We should still have the same number of leaves
	if want := len(sequencedLeaves); numLeaves != want {
		return 0, fmt.Errorf("%v: wanted: %v leaves after sequencing but we got: %v", logID, want, numLeaves)
	}

	// Write the new sequence numbers to the leaves in the DB
	if err := tx.UpdateSequencedLeaves(ctx, sequencedLeaves); err != nil {
		glog.Warningf("%v: Sequencer failed to update sequenced leaves: %v", logID, err)
		return 0, err
	}
	seqUpdateLeavesLatency.Observe(s.since(stageStart), label)
	stageStart = s.timeSource.Now()

	// Build objects for the nodes to be updated. Because we deduped via the map each
	// node can only be created / updated once in each tree revision and they cannot
	// conflict when we do the storage update.
	targetNodes, err := s.buildNodesFromNodeMap(nodeMap, newVersion)
	if err != nil {
		// probably an internal error with map building, unexpected
		glog.Warningf("%v: Failed to build target nodes in sequencer: %v", logID, err)
		return 0, err
	}

	// Now insert or update the nodes affected by the above, at the new tree version
	if err := tx.SetMerkleNodes(ctx, targetNodes); err != nil {
		glog.Warningf("%v: Sequencer failed to set Merkle nodes: %v", logID, err)
		return 0, err
	}
	seqSetNodesLatency.Observe(s.since(stageStart), label)
	stageStart = s.timeSource.Now()

	// Create the log root ready for signing
	newLogRoot := trillian.SignedLogRoot{
		RootHash:       merkleTree.CurrentRoot(),
		TimestampNanos: s.timeSource.Now().UnixNano(),
		TreeSize:       merkleTree.Size(),
		LogId:          currentRoot.LogId,
		TreeRevision:   newVersion,
	}
	seqTreeSize.Set(float64(merkleTree.Size()), label)

	// Hash and sign the root, update it with the signature
	signature, err := s.createRootSignature(ctx, newLogRoot)
	if err != nil {
		glog.Warningf("%v: signer failed to sign root: %v", logID, err)
		return 0, err
	}

	newLogRoot.Signature = signature

	if err := tx.StoreSignedLogRoot(ctx, newLogRoot); err != nil {
		glog.Warningf("%v: failed to write updated tree root: %v", logID, err)
		return 0, err
	}
	seqStoreRootLatency.Observe(s.since(stageStart), label)
	stageStart = s.timeSource.Now()

	// The batch is now fully sequenced and we're done
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	seqCommitLatency.Observe(s.since(stageStart), label)

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
	glog.Infof("%v: sequenced %v leaves, size %v, tree-revision %v", logID, numLeaves, newLogRoot.TreeSize, newLogRoot.TreeRevision)
	return numLeaves, nil
}

// SignRoot wraps up all the operations for creating a new log signed root.
func (s Sequencer) SignRoot(ctx context.Context, logID int64) error {
	tx, err := s.logStorage.BeginForTree(ctx, logID)
	if err != nil {
		glog.Warningf("%v: signer failed to start tx: %v", logID, err)
		return err
	}
	defer tx.Close()

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

	// Build the updated root, ready for signing
	newLogRoot := trillian.SignedLogRoot{
		RootHash:       merkleTree.CurrentRoot(),
		TimestampNanos: s.timeSource.Now().UnixNano(),
		TreeSize:       merkleTree.Size(),
		LogId:          currentRoot.LogId,
		TreeRevision:   currentRoot.TreeRevision + 1,
	}

	// Hash and sign the root
	signature, err := s.createRootSignature(ctx, newLogRoot)
	if err != nil {
		glog.Warningf("%v: signer failed to sign root: %v", logID, err)
		return err
	}
	newLogRoot.Signature = signature

	// Store the new root and we're done
	if err := tx.StoreSignedLogRoot(ctx, newLogRoot); err != nil {
		glog.Warningf("%v: signer failed to write updated root: %v", logID, err)
		return err
	}
	glog.V(2).Infof("%v: new signed root, size %v, tree-revision %v", logID, newLogRoot.TreeSize, newLogRoot.TreeRevision)

	return tx.Commit()
}

// since() returns the time in seconds since a particular time, according to
// the TimeSource used by this sequencer.
func (s *Sequencer) since(start time.Time) float64 {
	return s.timeSource.Now().Sub(start).Seconds()
}
