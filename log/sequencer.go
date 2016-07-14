// Runs sequencing operations
package log

import (
	"errors"
	"fmt"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/util"
)

// Sequencer instances are responsible for integrating new leaves into a log.
// Leaves will be assigned unique sequence numbers when they are processed.
// There is no strong ordering guarantee but in general entries will be processed
// in order of submission to the log.
type Sequencer struct {
	hasher     merkle.TreeHasher
	timeSource util.TimeSource
	logStorage storage.LogStorage
	keyManager crypto.KeyManager
}

func NewSequencer(hasher merkle.TreeHasher, timeSource util.TimeSource, logStorage storage.LogStorage, km crypto.KeyManager) *Sequencer {
	return &Sequencer{hasher, timeSource, logStorage, km}
}

// TODO: This currently doesn't use the batch api for fetching the required nodes. This
// would be more efficient but requires refactoring.
func (s Sequencer) buildMerkleTreeFromStorageAtRoot(root trillian.SignedLogRoot, tx storage.TreeTX) (*merkle.CompactMerkleTree, error) {
	mt, err := merkle.NewCompactMerkleTreeWithState(s.hasher, root.TreeSize, func(depth int, index int64) (trillian.Hash, error) {
		nodeId := storage.NewNodeIDForTreeCoords(int64(depth), index, int(s.hasher.Size()))
		nodes, err := tx.GetMerkleNodes(root.TreeRevision, []storage.NodeID{nodeId})

		if err != nil {
			glog.Warningf("Failed to get merkle nodes: %s", err)
			return nil, err
		}

		// We expect to get exactly one node here
		if nodes == nil || len(nodes) != 1 {
			return nil, errors.New("Did not retrieve one node while loading CompactMerkleTree")
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

func (s Sequencer) sequenceLeaves(mt *merkle.CompactMerkleTree, leaves []trillian.LogLeaf) (map[string]storage.Node, []int64) {
	nodeMap := make(map[string]storage.Node)
	sequenceNumbers := make([]int64, 0, len(leaves))

	// Update the tree state and sequence the leaves, tracking the node updates that need to be
	// made and assign sequence numbers to the new leaves
	for _, leaf := range leaves {
		seq := mt.AddLeafHash(leaf.LeafHash, func(depth int, index int64, hash trillian.Hash) {
			nodeId := storage.NewNodeIDForTreeCoords(int64(depth), index, len(leaf.LeafHash))
			nodeMap[nodeId.String()] = storage.Node{
				NodeID:       nodeId,
				Hash:         hash,
				NodeRevision: -1,
			}
		})

		sequenceNumbers = append(sequenceNumbers, seq)
	}

	return nodeMap, sequenceNumbers
}

func (s Sequencer) initMerkleTreeFromStorage(currentRoot trillian.SignedLogRoot, tx storage.LogTX) (*merkle.CompactMerkleTree, error) {
	var merkleTree *merkle.CompactMerkleTree

	if currentRoot.TreeSize == 0 {
		// This should be an empty tree
		if currentRoot.TreeRevision > 0 {
			return nil, fmt.Errorf("MT has zero size but non zero revision: %v", *merkleTree)
		}
		return merkle.NewCompactMerkleTree(s.hasher), nil
	}

	// Initialize the compact tree state to match the latest root in the database
	return s.buildMerkleTreeFromStorageAtRoot(currentRoot, tx)
}

func (s Sequencer) signRoot(root trillian.SignedLogRoot) (trillian.DigitallySigned, error) {
	signer, err := s.keyManager.Signer()

	if err != nil {
		glog.Warningf("key manager failed to create crypto.Signer: %v", err)
		return trillian.DigitallySigned{}, err
	}

	// TODO(Martin2112): Signature algorithm shouldn't be fixed here
	trillianSigner := crypto.NewTrillianSigner(s.hasher.Hasher, trillian.SignatureAlgorithm_ECDSA, signer)

	signature, err := trillianSigner.SignLogRoot(root)

	if err != nil {
		glog.Warningf("signer failed to sign root: %v", err)
		return trillian.DigitallySigned{}, err
	}

	return signature, nil
}

// SequenceBatch wraps up all the operations needed to take a batch of queued leaves
// and integrate them into the tree.
// TODO(Martin2112): Can possibly improve by deferring a function that attempts to rollback,
// which will fail if the tx was committed. Should only do this if we can hide the details of
// the underlying storage transactions and it doesn't create other problems.
func (s Sequencer) SequenceBatch(limit int) (int, error) {
	tx, err := s.logStorage.Begin()

	if err != nil {
		glog.Warningf("Sequencer failed to start tx: %s", err)
		return 0, err
	}

	leaves, err := tx.DequeueLeaves(limit)

	if err != nil {
		glog.Warningf("Sequencer failed to dequeue leaves: %s", err)
		tx.Rollback()
		return 0, err
	}

	// There might be no work to be done
	if len(leaves) == 0 {
		tx.Commit()
		return 0, nil
	}

	// Get the latest known root from storage
	currentRoot, err := tx.LatestSignedLogRoot()

	if err != nil {
		glog.Warningf("Sequencer failed to get latest root: %s", err)
		tx.Rollback()
		return 0, err
	}

	merkleTree, err := s.initMerkleTreeFromStorage(currentRoot, tx)

	if err != nil {
		tx.Rollback()
		return 0, err
	}

	// We've done all the reads, can now do the updates.
	// TODO: This relies on us being the only process updating the map, which isn't enforced yet
	// though the schema should now prevent multiple STHs being inserted with the same revision
	// number so it should not be possible for colliding updates to commit.
	newVersion := currentRoot.TreeRevision + int64(1)

	// Assign leaf sequence numbers and collate node updates
	nodeMap, sequenceNumbers := s.sequenceLeaves(merkleTree, leaves)

	if len(sequenceNumbers) != len(leaves) {
		panic(fmt.Sprintf("Sequencer returned %d sequence numbers for %d leaves", len(sequenceNumbers),
			len(leaves)))
	}

	for index, _ := range sequenceNumbers {
		leaves[index].SequenceNumber = sequenceNumbers[index]
	}

	// Write the new sequence numbers to the leaves in the DB
	err = tx.UpdateSequencedLeaves(leaves)

	if err != nil {
		glog.Warningf("Sequencer failed to update sequenced leaves: %s", err)
		tx.Rollback()
		return 0, err
	}

	// Build objects for the nodes to be updated. Because we deduped via the map each
	// node can only be created / updated once in each tree revision and they cannot
	// conflict when we do the storage update.
	targetNodes, err := s.buildNodesFromNodeMap(nodeMap, newVersion)

	if err != nil {
		// probably an internal error with map building, unexpected
		glog.Warningf("Failed to build target nodes in sequencer: %s", err)
		tx.Rollback()
		return 0, err
	}

	// Now insert or update the nodes affected by the above, at the new tree version
	err = tx.SetMerkleNodes(newVersion, targetNodes)

	if err != nil {
		glog.Warningf("Sequencer failed to set merkle nodes: %s", err)
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
	signature, err := s.signRoot(newLogRoot)

	if err != nil {
		glog.Warningf("signer failed to sign root: %v", err)
		tx.Rollback()
		return 0, err
	}

	newLogRoot.Signature = &signature

	err = tx.StoreSignedLogRoot(newLogRoot)

	if err != nil {
		glog.Warningf("failed to write updated tree root: %s", err)
		tx.Rollback()
		return 0, err
	}

	// The batch is now fully sequenced and we're done
	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return len(leaves), nil
}

// SignRoot wraps up all the operations for creating a new log signed root.
func (s Sequencer) SignRoot() error {
	tx, err := s.logStorage.Begin()

	if err != nil {
		glog.Warningf("signer failed to start tx: %s", err)
		return err
	}

	// Get the latest known root from storage
	currentRoot, err := tx.LatestSignedLogRoot()

	if err != nil {
		glog.Warningf("signer failed to get latest root: %s", err)
		tx.Rollback()
		return err
	}

	// Initialize a Merkle Tree from the state in storage. This should fail if the tree is
	// in a corrupt state.
	merkleTree, err := s.initMerkleTreeFromStorage(currentRoot, tx)

	if err != nil {
		tx.Rollback()
		return err
	}

	// Build the updated root, ready for signing
	// TODO(Martin2112): *** Consolidate sequencer and signer so only one entity produces new
	// tree revisions. ***
	newLogRoot := trillian.SignedLogRoot{
		RootHash:       merkleTree.CurrentRoot(),
		TimestampNanos: s.timeSource.Now().UnixNano(),
		TreeSize:       merkleTree.Size(),
		LogId:          currentRoot.LogId,
		TreeRevision:   currentRoot.TreeRevision + 1,
	}

	// Hash and sign the root
	signature, err := s.signRoot(newLogRoot)

	if err != nil {
		glog.Warningf("signer failed to sign root: %v", err)
		tx.Rollback()
		return err
	}

	newLogRoot.Signature = &signature

	// Store the new root and we're done
	if err := tx.StoreSignedLogRoot(newLogRoot); err != nil {
		glog.Warningf("signer failed to write updated root: %v", err)
		tx.Rollback()
		return err
	}

	return tx.Commit()
}
