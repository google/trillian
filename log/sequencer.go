// Runs sequencing operations
package log

import (
	"errors"
	"fmt"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
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
	hasher     trillian.Hasher
	timeSource util.TimeSource
	logStorage storage.LogStorage
}

func NewSequencer(hasher trillian.Hasher, timeSource util.TimeSource, logStorage storage.LogStorage) *Sequencer {
	return &Sequencer{hasher, timeSource, logStorage}
}

// TODO: This currently doesn't use the batch api for fetching the required nodes. This
// would be more efficient but requires refactoring.
func (s Sequencer) buildMerkleTreeFromStorageAtRoot(root trillian.SignedLogRoot, tx storage.TreeTX) (*merkle.CompactMerkleTree, error) {
	if root.TreeSize == nil {
		return nil, errors.New("invalid root; TreeSize unset")
	}
	if root.TreeRevision == nil {
		return nil, errors.New("invalid root; TreeRevision unset")
	}
	mt, err := merkle.NewCompactMerkleTreeWithState(s.hasher, *root.TreeSize, func(depth int, index int64) (trillian.Hash, error) {
		nodeId := storage.NewNodeIDForTreeCoords(int64(depth), index, int(s.hasher.Size()))
		nodes, err := tx.GetMerkleNodes(*root.TreeRevision, []storage.NodeID{nodeId})

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
func (s Sequencer) SequenceBatch(limit int) error {
	tx, err := s.logStorage.Begin()

	if err != nil {
		glog.Warningf("Sequencer failed to start tx: %s", err)
		return err
	}

	leaves, err := tx.DequeueLeaves(limit)

	if err != nil {
		glog.Warningf("Sequencer failed to dequeue leaves: %s", err)
		tx.Rollback()
		return err
	}

	// There might be no work to be done
	if len(leaves) == 0 {
		tx.Rollback()
		return nil
	}

	// Get the latest known root from storage
	currentRoot, err := tx.LatestSignedLogRoot()

	if err != nil {
		glog.Warningf("Sequencer failed to get latest root: %s", err)
		tx.Rollback()
		return err
	}

	// Initialize the compact tree state to match the latest root in the database
	mt, err := s.buildMerkleTreeFromStorageAtRoot(currentRoot, tx)

	if err != nil {
		tx.Rollback()
		return err
	}

	// We've done all the reads, can now do the updates.
	// TODO: This relies on us being the only process updating the map, which isn't enforced yet
	// though the schema should now prevent multiple STHs being inserted with the same revision
	// number so it should not be possible for colliding updates to commit.
	newVersion := *currentRoot.TreeRevision + 1

	// Assign leaf sequence numbers and collate node updates
	nodeMap, sequenceNumbers := s.sequenceLeaves(mt, leaves)

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
		return err
	}

	// Build objects for the nodes to be updated. Because we deduped via the map each
	// node can only be created / updated once in each tree revision and they cannot
	// conflict when we do the storage update.
	targetNodes, err := s.buildNodesFromNodeMap(nodeMap, newVersion)

	if err != nil {
		// probably an internal error with map building, unexpected
		glog.Warningf("Failed to build target nodes in sequencer: %s", err)
		tx.Rollback()
		return err
	}

	// Now insert or update the nodes affected by the above, at the new tree version
	err = tx.SetMerkleNodes(newVersion, targetNodes)

	if err != nil {
		glog.Warningf("Sequencer failed to set merkle nodes: %s", err)
		tx.Rollback()
		return err
	}

	// Write an updated root back to the tree. currently signing is done separately to
	// sequencing, though that's not finalized yet.
	newLogRoot := trillian.SignedLogRoot{
		RootHash:       mt.CurrentRoot(),
		TimestampNanos: proto.Int64(s.timeSource.Now().UnixNano()),
		TreeSize:       proto.Int64(0),
		Signature:      &trillian.DigitallySigned{},
		LogId:          currentRoot.LogId,
		TreeRevision:   proto.Int64(*currentRoot.TreeRevision + 1),
	}

	err = tx.StoreSignedLogRoot(newLogRoot)

	if err != nil {
		glog.Warningf("Failed to updated tree root: %s", err)
		tx.Rollback()
		return err
	}

	// The batch is now fully sequenced and we're done
	return tx.Commit()
}
