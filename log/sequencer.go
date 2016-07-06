// Runs sequencing operations
package log

import (
	"errors"
	"fmt"

	"github.com/golang/glog"
	"github.com/google/trillian"
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
}

func NewSequencer(hasher merkle.TreeHasher, timeSource util.TimeSource, logStorage storage.LogStorage) *Sequencer {
	return &Sequencer{hasher, timeSource, logStorage}
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

// Can possibly improve by deferring a function that attempts to rollback, which will
// fail if the tx was committed. Should only do this if we can hide the details of
// the underlying storage transactions.
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

	var merkleTree *merkle.CompactMerkleTree

	if currentRoot.TreeSize == 0 {
		// This should be an empty tree
		if currentRoot.TreeRevision > 0 {
			// TODO(Martin2112): Remove panic and return error when we implement proper multi
			// tenant scheduling.
			panic(fmt.Errorf("MT has zero size but non zero revision: %v", *merkleTree))
		}
		merkleTree = merkle.NewCompactMerkleTree(s.hasher)
	} else {
		// Initialize the compact tree state to match the latest root in the database
		merkleTree, err = s.buildMerkleTreeFromStorageAtRoot(currentRoot, tx)

		if err != nil {
			tx.Rollback()
			return 0, err
		}
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

	// Write an updated root back to the tree. currently signing is done separately to
	// sequencing, though that's not finalized yet.
	newLogRoot := trillian.SignedLogRoot{
		RootHash:       merkleTree.CurrentRoot(),
		TimestampNanos: s.timeSource.Now().UnixNano(),
		TreeSize:       merkleTree.Size(),
		Signature:      &trillian.DigitallySigned{},
		LogId:          currentRoot.LogId,
		TreeRevision:   newVersion,
	}

	err = tx.StoreSignedLogRoot(newLogRoot)

	if err != nil {
		glog.Warningf("Failed to updated tree root: %s", err)
		tx.Rollback()
		return 0, err
	}

	// The batch is now fully sequenced and we're done
	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return len(leaves), nil
}
