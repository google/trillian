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
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/util"
	"golang.org/x/net/context"
)

// TODO(Martin2112): Add admin support for safely changing params like guard window during operation
// TODO(Martin2112): Add support for enabling and controlling sequencing as part of admin API

// Sequencer instances are responsible for integrating new leaves into a single log.
// Leaves will be assigned unique sequence numbers when they are processed.
// There is no strong ordering guarantee but in general entries will be processed
// in order of submission to the log.
type Sequencer struct {
	hasher     merkle.TreeHasher
	timeSource util.TimeSource
	logStorage storage.LogStorage
	keyManager crypto.KeyManager

	// These parameters could theoretically be adjusted during operation
	// sequencerGuardWindow is used to ensure entries newer than the guard window will not be
	// sequenced until they fall outside it. By default there is no guard window.
	sequencerGuardWindow time.Duration
}

// maxTreeDepth sets an upper limit on the size of Log trees.
// TODO(al): We actually can't go beyond 2^63 entries because we use int64s,
//           but we need to calculate tree depths from a multiple of 8 due to
//           the subtrees.
const maxTreeDepth = 64

// NewSequencer creates a new Sequencer instance for the specified inputs.
func NewSequencer(hasher merkle.TreeHasher, timeSource util.TimeSource, logStorage storage.LogStorage, km crypto.KeyManager) *Sequencer {
	return &Sequencer{hasher: hasher, timeSource: timeSource, logStorage: logStorage, keyManager: km}
}

// SetGuardWindow changes the interval that must elapse between leaves being queued and them
// being eligible for sequencing. The default is a zero interval.
func (s *Sequencer) SetGuardWindow(sequencerGuardWindow time.Duration) {
	s.sequencerGuardWindow = sequencerGuardWindow
}

// TODO: This currently doesn't use the batch api for fetching the required nodes. This
// would be more efficient but requires refactoring.
func (s Sequencer) buildMerkleTreeFromStorageAtRoot(ctx context.Context, root trillian.SignedLogRoot, tx storage.TreeTX) (*merkle.CompactMerkleTree, error) {
	mt, err := merkle.NewCompactMerkleTreeWithState(s.hasher, root.TreeSize, func(depth int, index int64) ([]byte, error) {
		nodeID, err := storage.NewNodeIDForTreeCoords(int64(depth), index, maxTreeDepth)
		if err != nil {
			glog.Warningf("%s: Failed to create nodeID: %v", util.LogIDPrefix(ctx), err)
			return nil, err
		}
		nodes, err := tx.GetMerkleNodes(root.TreeRevision, []storage.NodeID{nodeID})

		if err != nil {
			glog.Warningf("%s: Failed to get Merkle nodes: %s", util.LogIDPrefix(ctx), err)
			return nil, err
		}

		// We expect to get exactly one node here
		if nodes == nil || len(nodes) != 1 {
			return nil, fmt.Errorf("%s: Did not retrieve one node while loading CompactMerkleTree, got %#v for ID %s@%d", util.LogIDPrefix(ctx), nodes, nodeID.String(), root.TreeRevision)
		}

		return nodes[0].Hash, nil
	}, root.RootHash)

	return mt, err
}

func (s Sequencer) buildNodesFromNodeMap(nodeMap map[string]storage.Node, newVersion int64) ([]storage.Node, error) {
	targetNodes := make([]storage.Node, len(nodeMap), len(nodeMap))
	i := 0
	for _, node := range nodeMap {
		node.NodeRevision = newVersion
		targetNodes[i] = node
		i++
	}
	return targetNodes, nil
}

func (s Sequencer) sequenceLeaves(mt *merkle.CompactMerkleTree, leaves []trillian.LogLeaf) (map[string]storage.Node, []trillian.LogLeaf, error) {
	nodeMap := make(map[string]storage.Node)
	// Update the tree state and sequence the leaves and assign sequence numbers to the new leaves
	for i, leaf := range leaves {
		seq := mt.AddLeafHash(leaf.MerkleLeafHash, func(depth int, index int64, hash []byte) {
			nodeID, err := storage.NewNodeIDForTreeCoords(int64(depth), index, maxTreeDepth)
			if err != nil {
				return
			}
			nodeMap[nodeID.String()] = storage.Node{
				NodeID: nodeID,
				Hash:   hash,
			}
		})
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

func (s Sequencer) initMerkleTreeFromStorage(ctx context.Context, currentRoot trillian.SignedLogRoot, tx storage.LogTX) (*merkle.CompactMerkleTree, error) {
	if currentRoot.TreeSize == 0 {
		return merkle.NewCompactMerkleTree(s.hasher), nil
	}

	// Initialize the compact tree state to match the latest root in the database
	return s.buildMerkleTreeFromStorageAtRoot(ctx, currentRoot, tx)
}

func (s Sequencer) createRootSignature(ctx context.Context, root trillian.SignedLogRoot) (trillian.DigitallySigned, error) {
	signer, err := s.keyManager.Signer()
	if err != nil {
		glog.Warningf("%s: key manager failed to create crypto.Signer: %v", util.LogIDPrefix(ctx), err)
		return trillian.DigitallySigned{}, err
	}

	trillianSigner := crypto.NewSigner(s.hasher.Hasher, s.keyManager.SignatureAlgorithm(), signer)

	signature, err := trillianSigner.SignLogRoot(root)
	if err != nil {
		glog.Warningf("%s: signer failed to sign root: %v", util.LogIDPrefix(ctx), err)
		return trillian.DigitallySigned{}, err
	}

	return signature, nil
}

// SequenceBatch wraps up all the operations needed to take a batch of queued leaves
// and integrate them into the tree.
// TODO(Martin2112): Can possibly improve by deferring a function that attempts to rollback,
// which will fail if the tx was committed. Should only do this if we can hide the details of
// the underlying storage transactions and it doesn't create other problems.
func (s Sequencer) SequenceBatch(ctx context.Context, limit int) (int, error) {
	tx, err := s.logStorage.Begin()

	if err != nil {
		glog.Warningf("%s: Sequencer failed to start tx: %s", util.LogIDPrefix(ctx), err)
		return 0, err
	}

	// Very recent leaves inside the guard window will not be available for sequencing
	guardCutoffTime := s.timeSource.Now().Add(-s.sequencerGuardWindow)
	leaves, err := tx.DequeueLeaves(limit, guardCutoffTime)

	if err != nil {
		glog.Warningf("%s: Sequencer failed to dequeue leaves: %s", util.LogIDPrefix(ctx), err)
		tx.Rollback()
		return 0, err
	}

	// Get the latest known root from storage
	currentRoot, err := tx.LatestSignedLogRoot()

	if err != nil {
		glog.Warningf("%s: Sequencer failed to get latest root: %s", util.LogIDPrefix(ctx), err)
		tx.Rollback()
		return 0, err
	}

	// TODO(al): Have a better detection mechanism for there being no stored root.
	// TODO(mhs): Might be better to create empty root in provisioning API when it exists
	if currentRoot.RootHash == nil {
		glog.Warningf("%s: Fresh log - no previous TreeHeads exist.", util.LogIDPrefix(ctx))
		return 0, s.SignRoot(ctx)
	}

	// There might be no work to be done. But we possibly still need to create an STH if the
	// current one is too old. If there's work to be done then we'll be creating a root anyway.
	if len(leaves) == 0 {
		// We have nothing to integrate into the tree
		return 0, tx.Commit()
	}

	merkleTree, err := s.initMerkleTreeFromStorage(ctx, currentRoot, tx)

	if err != nil {
		tx.Rollback()
		return 0, err
	}

	// We've done all the reads, can now do the updates.
	// TODO: This relies on us being the only process updating the map, which isn't enforced yet
	// though the schema should now prevent multiple STHs being inserted with the same revision
	// number so it should not be possible for colliding updates to commit.
	newVersion := tx.WriteRevision()
	if got, want := newVersion, currentRoot.TreeRevision+int64(1); got != want {
		tx.Rollback()
		return 0, fmt.Errorf("%s: got writeRevision of %d, but expected %d", util.LogIDPrefix(ctx), got, want)
	}

	// Assign leaf sequence numbers and collate node updates
	nodeMap, sequencedLeaves, err := s.sequenceLeaves(merkleTree, leaves)
	if err != nil {
		tx.Rollback()
		return 0, err
	}

	// We should still have the same number of leaves
	if want, got := len(leaves), len(sequencedLeaves); want != got {
		tx.Rollback()
		return 0, fmt.Errorf("%s: wanted: %d leaves after sequencing but we got: %d", util.LogIDPrefix(ctx), want, got)
	}

	// Write the new sequence numbers to the leaves in the DB
	err = tx.UpdateSequencedLeaves(sequencedLeaves)

	if err != nil {
		glog.Warningf("%s: Sequencer failed to update sequenced leaves: %s", util.LogIDPrefix(ctx), err)
		tx.Rollback()
		return 0, err
	}

	// Build objects for the nodes to be updated. Because we deduped via the map each
	// node can only be created / updated once in each tree revision and they cannot
	// conflict when we do the storage update.
	targetNodes, err := s.buildNodesFromNodeMap(nodeMap, newVersion)

	if err != nil {
		// probably an internal error with map building, unexpected
		glog.Warningf("%s: Failed to build target nodes in sequencer: %s", util.LogIDPrefix(ctx), err)
		tx.Rollback()
		return 0, err
	}

	// Now insert or update the nodes affected by the above, at the new tree version
	err = tx.SetMerkleNodes(targetNodes)

	if err != nil {
		glog.Warningf("%s: Sequencer failed to set Merkle nodes: %s", util.LogIDPrefix(ctx), err)
		tx.Rollback()
		return 0, err
	}

	// Create the log root ready for signing
	newLogRoot := trillian.SignedLogRoot{
		RootHash:       merkleTree.CurrentRoot(),
		TimestampNanos: s.timeSource.Now().UnixNano(),
		TreeSize:       merkleTree.Size(),
		LogId:          currentRoot.LogId,
		TreeRevision:   newVersion,
	}

	// Hash and sign the root, update it with the signature
	signature, err := s.createRootSignature(ctx, newLogRoot)

	if err != nil {
		glog.Warningf("%s: signer failed to sign root: %v", util.LogIDPrefix(ctx), err)
		tx.Rollback()
		return 0, err
	}

	newLogRoot.Signature = &signature

	err = tx.StoreSignedLogRoot(newLogRoot)

	if err != nil {
		glog.Warningf("%s: failed to write updated tree root: %s", util.LogIDPrefix(ctx), err)
		tx.Rollback()
		return 0, err
	}

	// The batch is now fully sequenced and we're done
	if err := tx.Commit(); err != nil {
		return 0, err
	}

	glog.Infof("%s: sequenced %d leaves, size %d, tree-revision %d", util.LogIDPrefix(ctx), len(leaves), newLogRoot.TreeSize, newLogRoot.TreeRevision)
	return len(leaves), nil
}

// SignRoot wraps up all the operations for creating a new log signed root.
func (s Sequencer) SignRoot(ctx context.Context) error {
	tx, err := s.logStorage.Begin()

	if err != nil {
		glog.Warningf("%s: signer failed to start tx: %s", util.LogIDPrefix(ctx), err)
		return err
	}

	// Get the latest known root from storage
	currentRoot, err := tx.LatestSignedLogRoot()

	if err != nil {
		glog.Warningf("%s: signer failed to get latest root: %s", util.LogIDPrefix(ctx), err)
		tx.Rollback()
		return err
	}

	// Initialize a Merkle Tree from the state in storage. This should fail if the tree is
	// in a corrupt state.
	merkleTree, err := s.initMerkleTreeFromStorage(ctx, currentRoot, tx)

	if err != nil {
		tx.Rollback()
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
		glog.Warningf("%s: signer failed to sign root: %v", util.LogIDPrefix(ctx), err)
		tx.Rollback()
		return err
	}

	newLogRoot.Signature = &signature

	// Store the new root and we're done
	if err := tx.StoreSignedLogRoot(newLogRoot); err != nil {
		glog.Warningf("%s: signer failed to write updated root: %v", util.LogIDPrefix(ctx), err)
		tx.Rollback()
		return err
	}
	glog.V(2).Infof("%s: new signed root, size %d, tree-revision %d", util.LogIDPrefix(ctx), newLogRoot.TreeSize, newLogRoot.TreeRevision)

	return tx.Commit()
}
