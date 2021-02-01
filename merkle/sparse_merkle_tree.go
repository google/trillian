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

package merkle

import (
	"context"
	"errors"
	"fmt"

	"github.com/golang/glog"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/tree"
)

// For more information about how Sparse Merkle Trees work see the Revocation Transparency
// paper in the docs directory. Note that applications are not limited to X.509 certificates
// and this implementation handles arbitrary data.

// SparseMerkleTreeReader knows how to read data from a TreeStorage transaction
// to provide proofs etc.
type SparseMerkleTreeReader struct {
	tx           storage.ReadOnlyTreeTX
	hasher       hashers.MapHasher
	treeRevision int64
}

// TXRunner supplies the RunTX function.
// TXRunner can be passed as the last argument to MapStorage.ReadWriteTransaction.
//
// TODO(pavelkalinnikov): This interface violates layering, because it assumes
// existence of transactions. It must be part of the storage package.
type TXRunner interface {
	// RunTX executes f and supplies a transaction object to operate on.
	RunTX(ctx context.Context, f func(context.Context, storage.MapTreeTX) error) error
}

// ErrNoSuchRevision is returned when a request is made for information about
// a tree revision which does not exist.
var ErrNoSuchRevision = errors.New("no such revision")

// NewSparseMerkleTreeReader returns a new SparseMerkleTreeReader, reading at
// the specified tree revision, using the passed in MapHasher for calculating
// and verifying tree hashes read via tx.
func NewSparseMerkleTreeReader(rev int64, h hashers.MapHasher, tx storage.ReadOnlyTreeTX) *SparseMerkleTreeReader {
	return &SparseMerkleTreeReader{
		tx:           tx,
		hasher:       h,
		treeRevision: rev,
	}
}

// InclusionProof returns an inclusion (or non-inclusion) proof for the
// specified key at the specified revision.
// If the revision does not exist it will return ErrNoSuchRevision error.
func (s SparseMerkleTreeReader) InclusionProof(ctx context.Context, rev int64, index []byte) ([][]byte, error) {
	ctx, spanEnd := spanFor(ctx, "InclusionProof")
	defer spanEnd()

	proofs, err := s.BatchInclusionProof(ctx, rev, [][]byte{index})
	if err != nil {
		return nil, err
	}
	return proofs[string(index)], nil
}

// BatchInclusionProof returns an inclusion (or non-inclusion) proof for each of the specified keys
// at the specified revision. The return value is a map of the string form of the key to the
// inclusion proof for that key.
func (s SparseMerkleTreeReader) BatchInclusionProof(ctx context.Context, rev int64, indices [][]byte) (map[string][][]byte, error) {
	ctx, spanEnd := spanFor(ctx, "BatchInclusionProof")
	defer spanEnd()

	_, calculateSpanEnd := spanFor(ctx, "binc.calculateNodes")
	indexToSibs := make(map[string][]tree.NodeID)
	allSibs := make([]tree.NodeID, 0, len(indices)*s.hasher.BitLen())
	includedNodes := map[string]bool{}
	for _, index := range indices {
		nid := tree.NewNodeIDFromHash(index)
		sibs := nid.Siblings()
		indexToSibs[string(index)] = sibs
		for _, sib := range sibs {
			if sibID := sib.AsKey(); !includedNodes[sibID] {
				includedNodes[sibID] = true
				allSibs = append(allSibs, sib)
			}
		}
	}
	calculateSpanEnd()

	gnCtx, getNodesSpanEnd := spanFor(ctx, "binc.getMerkleNodes")
	nodes, err := s.tx.GetMerkleNodes(gnCtx, rev, allSibs)
	if err != nil {
		return nil, err
	}
	getNodesSpanEnd()

	_, postprocessSpanEnd := spanFor(ctx, "binc.postprocess")
	nodeMap := make(map[string]*tree.Node)
	for i, n := range nodes {
		if glog.V(2) {
			glog.Infof("   %x, %d: %x", n.NodeID.Path, len(n.NodeID.AsKey()), n.Hash)
		}
		nodeMap[n.NodeID.AsKey()] = &nodes[i]
	}

	r := map[string][][]byte{}
	for _, index := range indices {
		// We're building a full proof from a combination of whichever nodes we got
		// back from the storage layer, and the set of "null" hashes.
		sibs := indexToSibs[string(index)]
		ri := make([][]byte, len(sibs))
		// For each proof element:
		for i := range ri {
			proofID := sibs[i]
			pNode := nodeMap[proofID.AsKey()]
			if pNode == nil {
				// No node for this level from storage, so use the nil hash.
				continue
			}
			ri[i] = pNode.Hash
		}
		r[string(index)] = ri
	}
	postprocessSpanEnd()

	return r, nil
}

func spanFor(ctx context.Context, name string) (context.Context, func()) {
	return monitoring.StartSpan(ctx, fmt.Sprintf("/trillian/m_sparse.%s", name))
}
