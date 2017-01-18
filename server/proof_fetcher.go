package server

import (
	"errors"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/storage"
)

// fetchNodesAndBuildProof is used by both inclusion and consistency proofs. It fetches the nodes
// from storage and converts them into the proof proto that will be returned to the client.
func fetchNodesAndBuildProof(tx storage.NodeReader, treeRevision, leafIndex int64, proofNodeFetches []merkle.NodeFetch) (trillian.Proof, error) {
	// TODO(Martin2112): Implement the rehashing. Currently just fetches the nodes and ignores this
	proofNodeIDs := make([]storage.NodeID, 0, len(proofNodeFetches))

	for _, fetch := range proofNodeFetches {
		proofNodeIDs = append(proofNodeIDs, fetch.NodeID)

		// TODO(Martin2112): Remove this when rehashing is implemented
		if fetch.Rehash {
			return trillian.Proof{}, errors.New("proof requires rehashing but it's not implemented yet")
		}
	}

	proofNodes, err := tx.GetMerkleNodes(treeRevision, proofNodeIDs)
	if err != nil {
		return trillian.Proof{}, err
	}

	if len(proofNodes) != len(proofNodeIDs) {
		return trillian.Proof{}, fmt.Errorf("expected %d nodes in proof but got %d", len(proofNodeIDs), len(proofNodes))
	}

	proof := make([]*trillian.Node, 0, len(proofNodeIDs))
	for i, node := range proofNodes {
		// additional check that the correct node was returned
		if !node.NodeID.Equivalent(proofNodeIDs[i]) {
			return trillian.Proof{}, fmt.Errorf("expected node %v at proof pos %d but got %v", proofNodeIDs[i], i, node.NodeID)
		}

		idBytes, err := proto.Marshal(node.NodeID.AsProto())
		if err != nil {
			return trillian.Proof{}, err
		}

		proof = append(proof, &trillian.Node{NodeId: idBytes, NodeHash: node.Hash, NodeRevision: node.NodeRevision})
	}

	return trillian.Proof{LeafIndex: leafIndex, ProofNode: proof}, nil
}
