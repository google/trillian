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
	"fmt"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/storage"
)

// fetchNodesAndBuildProof is used by both inclusion and consistency proofs. It fetches the nodes
// from storage and converts them into the proof proto that will be returned to the client.
// This includes rehashing where necessary to serve proofs for tree sizes between stored tree
// revisions. This code only relies on the NodeReader interface so can be tested without
// a complete storage implementation.
func fetchNodesAndBuildProof(tx storage.NodeReader, treeRevision, leafIndex int64, proofNodeFetches []merkle.NodeFetch) (trillian.Proof, error) {
	proofNodes, err := dedupAndFetchNodes(tx, treeRevision, proofNodeFetches)
	if err != nil {
		return trillian.Proof{}, err
	}

	r := newRehasher()
	for i, node := range proofNodes {
		r.process(node, proofNodeFetches[i])
	}

	return r.rehashedProof(leafIndex)
}

// rehasher bundles the rehashing logic into a simple state machine
type rehasher struct {
	th         merkle.TreeHasher
	rehashing  bool
	rehashNode storage.Node
	proof      []*trillian.Node
	proofError error
}

// init must be called before the rehasher is used or reused
func newRehasher() *rehasher {
	return &rehasher{
		// TODO(Martin2112): TreeHasher must be selected based on log config.
		th: merkle.NewRFC6962TreeHasher(crypto.NewSHA256()),
	}
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
	idBytes, err := proto.Marshal(node.NodeID.AsProto())
	if err != nil {
		r.proofError = err
	}
	r.proof = append(r.proof, &trillian.Node{NodeId: idBytes, NodeHash: node.Hash, NodeRevision: node.NodeRevision})
}

func (r *rehasher) startRehashing(node storage.Node) {
	r.rehashNode = storage.Node{Hash: node.Hash}
	r.rehashing = true
}

func (r *rehasher) endRehashing() {
	if r.rehashing {
		r.proof = append(r.proof, &trillian.Node{NodeHash: r.rehashNode.Hash})
		r.rehashing = false
	}
}

func (r *rehasher) rehashedProof(leafIndex int64) (trillian.Proof, error) {
	r.endRehashing()
	return trillian.Proof{LeafIndex: leafIndex, ProofNode: r.proof}, r.proofError
}

// dedupAndFetchNodes() removes duplicates from the set of fetches and then passes the result to
// storage. After writng this code I realised I don't have a solid proof that fetches for Merkle
// paths could involve duplicate nodes so it could be that this isn't ever useful. Further
// thought is required.
func dedupAndFetchNodes(tx storage.NodeReader, treeRevision int64, fetches []merkle.NodeFetch) ([]storage.Node, error) {
	// To start with we remove any duplicate fetches
	m := make(map[string]storage.NodeID)
	proofNodeIDs := make([]storage.NodeID, 0, len(fetches))

	// Remove duplicates, preserving the order of the input otherwise
	for _, fetch := range fetches {
		if _, ok := m[fetch.NodeID.String()]; !ok {
			// First time we've seen the ID
			m[fetch.NodeID.String()] = fetch.NodeID
			proofNodeIDs = append(proofNodeIDs, fetch.NodeID)
		}
	}

	if len(proofNodeIDs) < len(fetches) {
		glog.V(2).Infof("deduplication saved %d node fetch(es)", len(fetches) - len(proofNodeIDs))
	}

	// Use the deduplicated list of nodeIDs in the storage fetch
	proofNodes, err := tx.GetMerkleNodes(treeRevision, proofNodeIDs)
	if err != nil {
		return []storage.Node{}, err
	}

	if len(proofNodes) != len(proofNodeIDs) {
		return []storage.Node{}, fmt.Errorf("expected %d nodes from storage but got %d", len(proofNodeIDs), len(proofNodes))
	}

	// Then rebuild what we would have got if we'd fetched the duplicates
	nodes := make([]storage.Node, 0, len(fetches))
	nm := make(map[string]storage.Node)

	for _, node := range proofNodes {
		nm[node.NodeID.String()] = node
	}

	for _, fetch := range fetches {
		node, ok := nm[fetch.NodeID.String()]

		if !ok {
			return []storage.Node{}, fmt.Errorf("wanted node ID: %s but storage didn't return it", fetch.NodeID.String())
		}

		nodes = append(nodes, node)
	}

	for i, node := range nodes {
		// additional check that the correct node was returned
		if !node.NodeID.Equivalent(fetches[i].NodeID) {
			return []storage.Node{}, fmt.Errorf("expected node %v at proof pos %d but got %v", fetches[i], i, node.NodeID)
		}
	}

	return nodes, nil
}
