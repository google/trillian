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

package server

import (
	"context"
	"fmt"

	"github.com/google/trillian"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/storage"
)

// fetchNodesAndBuildProof is used by both inclusion and consistency proofs. It fetches the nodes
// from storage and converts them into the proof proto that will be returned to the client.
// This includes rehashing where necessary to serve proofs for tree sizes between stored tree
// revisions. This code only relies on the NodeReader interface so can be tested without
// a complete storage implementation.
func fetchNodesAndBuildProof(ctx context.Context, tx storage.NodeReader, th hashers.LogHasher, treeRevision, leafIndex int64, proofNodeFetches []merkle.NodeFetch) (trillian.Proof, error) {
	ctx, span := spanFor(ctx, "fetchNodesAndBuildProof")
	defer span.End()
	proofNodes, err := fetchNodes(ctx, tx, treeRevision, proofNodeFetches)
	if err != nil {
		return trillian.Proof{}, err
	}

	r := &rehasher{th: th}
	for i, node := range proofNodes {
		r.process(node, proofNodeFetches[i])
	}

	return r.rehashedProof(leafIndex)
}

// rehasher bundles the rehashing logic into a simple state machine
type rehasher struct {
	th         hashers.LogHasher
	rehashing  bool
	rehashNode storage.Node
	proof      [][]byte
	proofError error
}

func (r *rehasher) process(node storage.Node, fetch merkle.NodeFetch) {
	switch {
	case !r.rehashing && fetch.Rehash:
		// Start of a rehashing chain
		r.startRehashing(node)

	case r.rehashing && !fetch.Rehash:
		// End of a rehash chain, resulting in a rehashed proof node
		r.endRehashing()
		// And the current node needs to be added to the proof
		r.emitNode(node)

	case r.rehashing && fetch.Rehash:
		// Continue with rehashing, update the node we're recomputing
		r.rehashNode.Hash = r.th.HashChildren(node.Hash, r.rehashNode.Hash)

	default:
		// Not rehashing, just pass the node through
		r.emitNode(node)
	}
}

func (r *rehasher) emitNode(node storage.Node) {
	r.proof = append(r.proof, node.Hash)
}

func (r *rehasher) startRehashing(node storage.Node) {
	r.rehashNode = storage.Node{Hash: node.Hash}
	r.rehashing = true
}

func (r *rehasher) endRehashing() {
	if r.rehashing {
		r.proof = append(r.proof, r.rehashNode.Hash)
		r.rehashing = false
	}
}

func (r *rehasher) rehashedProof(leafIndex int64) (trillian.Proof, error) {
	r.endRehashing()
	return trillian.Proof{
		LeafIndex: leafIndex,
		Hashes:    r.proof,
	}, r.proofError
}

// fetchNodes extracts the NodeIDs from a list of NodeFetch structs and passes them
// to storage, returning the result after some additional validation checks.
func fetchNodes(ctx context.Context, tx storage.NodeReader, treeRevision int64, fetches []merkle.NodeFetch) ([]storage.Node, error) {
	ctx, span := spanFor(ctx, "fetchNodes")
	defer span.End()
	proofNodeIDs := make([]storage.NodeID, 0, len(fetches))

	for _, fetch := range fetches {
		proofNodeIDs = append(proofNodeIDs, fetch.NodeID)
	}

	proofNodes, err := tx.GetMerkleNodes(ctx, treeRevision, proofNodeIDs)
	if err != nil {
		return nil, err
	}

	if len(proofNodes) != len(proofNodeIDs) {
		return nil, fmt.Errorf("expected %d nodes from storage but got %d", len(proofNodeIDs), len(proofNodes))
	}

	for i, node := range proofNodes {
		// additional check that the correct node was returned
		if !node.NodeID.Equivalent(fetches[i].NodeID) {
			return []storage.Node{}, fmt.Errorf("expected node %v at proof pos %d but got %v", fetches[i], i, node.NodeID)
		}
	}

	return proofNodes, nil
}
