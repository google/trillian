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

package server

import (
	"context"
	"fmt"

	"github.com/google/trillian"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/tree"
)

// proofMaxBitLen is the max depth of a tree. Used for tree.NodeID creation.
const proofMaxBitLen = 64

// fetchNodesAndBuildProof is used by both inclusion and consistency proofs. It fetches the nodes
// from storage and converts them into the proof proto that will be returned to the client.
// This includes rehashing where necessary to serve proofs for tree sizes between stored tree
// revisions. This code only relies on the NodeReader interface so can be tested without
// a complete storage implementation.
func fetchNodesAndBuildProof(ctx context.Context, tx storage.NodeReader, th hashers.LogHasher, treeRevision, leafIndex int64, proofNodeFetches []merkle.NodeFetch) (*trillian.Proof, error) {
	ctx, spanEnd := spanFor(ctx, "fetchNodesAndBuildProof")
	defer spanEnd()
	proofNodes, err := fetchNodes(ctx, tx, treeRevision, proofNodeFetches)
	if err != nil {
		return nil, err
	}

	r := merkle.Rehasher{Hash: th.HashChildren}
	for i, node := range proofNodes {
		r.Process(node.Hash, proofNodeFetches[i].Rehash)
	}

	proof, err := r.RehashedProof()
	return &trillian.Proof{
		LeafIndex: leafIndex,
		Hashes:    proof,
	}, err
}

// fetchNodes extracts the NodeIDs from a list of NodeFetch structs and passes them
// to storage, returning the result after some additional validation checks.
func fetchNodes(ctx context.Context, tx storage.NodeReader, treeRevision int64, fetches []merkle.NodeFetch) ([]tree.Node, error) {
	ctx, spanEnd := spanFor(ctx, "fetchNodes")
	defer spanEnd()
	proofNodeIDs := make([]tree.NodeID, 0, len(fetches))

	for _, fetch := range fetches {
		id, err := tree.NewNodeIDForTreeCoords(int64(fetch.ID.Level), int64(fetch.ID.Index), proofMaxBitLen)
		if err != nil {
			return nil, err
		}
		proofNodeIDs = append(proofNodeIDs, id)
	}

	proofNodes, err := tx.GetMerkleNodes(ctx, treeRevision, proofNodeIDs)
	if err != nil {
		return nil, err
	}

	if len(proofNodes) != len(proofNodeIDs) {
		return nil, fmt.Errorf("expected %d nodes from storage but got %d", len(proofNodeIDs), len(proofNodes))
	}

	for i, node := range proofNodes {
		// Additional check that the correct node was returned.
		if !node.NodeID.Equivalent(proofNodeIDs[i]) {
			return []tree.Node{}, fmt.Errorf("expected node %v at proof pos %d but got %v", proofNodeIDs[i], i, node.NodeID)
		}
	}

	return proofNodes, nil
}
