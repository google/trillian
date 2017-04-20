// Copyright 2017 Google Inc. All Rights Reserved.
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

package testonly

import (
	"bytes"
	"fmt"

	"github.com/google/trillian"
	"github.com/google/trillian/storage"
)

// CheckLeafContents compares two log leaves to see if they match. Returns nil if they do
// otherwise a descriptive error.
func CheckLeafContents(leaf *trillian.LogLeaf, seq int64, rawHash, hash, data, extraData []byte) error {
	if got, want := leaf.MerkleLeafHash, hash; !bytes.Equal(got, want) {
		return fmt.Errorf("wrong leaf hash in returned leaf got\n%v\nwant:\n%v", got, want)
	}

	if got, want := leaf.LeafIdentityHash, rawHash; !bytes.Equal(got, want) {
		return fmt.Errorf("wrong raw leaf hash in returned leaf got\n%v\nwant:\n%v", got, want)
	}

	if got, want := seq, leaf.LeafIndex; got != want {
		return fmt.Errorf("bad sequence number in returned leaf got: %d, want:%d", got, want)
	}

	if got, want := leaf.LeafValue, data; !bytes.Equal(got, want) {
		return fmt.Errorf("unxpected data in returned leaf. got:\n%v\nwant:\n%v", got, want)
	}

	if got, want := leaf.ExtraData, extraData; !bytes.Equal(got, want) {
		return fmt.Errorf("unxpected data in returned leaf. got:\n%v\nwant:\n%v", got, want)
	}

	return nil
}

// LeafInBatch tests if a log leaf is contained in a slice of log leaves.
func LeafInBatch(leaf *trillian.LogLeaf, batch []*trillian.LogLeaf) bool {
	for _, bl := range batch {
		if bytes.Equal(bl.LeafIdentityHash, leaf.LeafIdentityHash) {
			return true
		}
	}

	return false
}

// EnsureAllLeavesDistinct checks that the supplied leaves do not contain any duplicate
// LeafIdentityHash Values. All the leaf hashes should be distinct because the leaves were
// created with distinct leaf data. Returns an error on the first duplicate hash found.
func EnsureAllLeavesDistinct(leaves []*trillian.LogLeaf) error {
	for i := range leaves {
		for j := range leaves {
			if i != j && bytes.Equal(leaves[i].LeafIdentityHash, leaves[j].LeafIdentityHash) {
				return fmt.Errorf("unexpectedly got a duplicate leaf hash: %v %v",
					leaves[i].LeafIdentityHash, leaves[j].LeafIdentityHash)
			}
		}
	}

	return nil
}

// NodesAreEqual tests if two slices of storage nodes contain the same set of nodes.
func NodesAreEqual(lhs []storage.Node, rhs []storage.Node) error {
	if ls, rs := len(lhs), len(rhs); ls != rs {
		return fmt.Errorf("different number of nodes, %d vs %d", ls, rs)
	}
	for i := range lhs {
		if l, r := lhs[i].NodeID.String(), rhs[i].NodeID.String(); l != r {
			return fmt.Errorf("node IDs are not the same,\nlhs = %v,\nrhs = %v", l, r)
		}
		if l, r := lhs[i].Hash, rhs[i].Hash; !bytes.Equal(l, r) {
			return fmt.Errorf("hashes are not the same for %s,\nlhs = %v,\nrhs = %v", lhs[i].NodeID.CoordString(), l, r)
		}
	}
	return nil
}

// DiffNodes compares two slices of storage nodes and returns slices containing missing and
// extra nodes (if any). If the slices contain the same data both returned slices will
// be empty. Does not handle duplicate values, returns an error if it detects dups in the
// supplied slices.
func DiffNodes(got, want []storage.Node) ([]storage.Node, []storage.Node, error) {
	missing := []storage.Node{}
	gotMap := makeNodeMap(got)
	if got, want := len(gotMap), len(got); got != want {
		return nil, nil, fmt.Errorf("dups in got slice. %d values but %d distinct values", got, want)
	}
	wantMap := makeNodeMap(want)
	if got, want := len(wantMap), len(want); got != want {
		return nil, nil, fmt.Errorf("dups in want slice. %d values but %d distinct values", got, want)
	}
	for _, n := range want {
		_, ok := gotMap[n.NodeID.String()]
		if !ok {
			missing = append(missing, n)
		}
		delete(gotMap, n.NodeID.String())
	}
	// Unpack the extra nodes to return both as slices
	extra := make([]storage.Node, 0, len(gotMap))
	for _, v := range gotMap {
		extra = append(extra, v)
	}
	return missing, extra, nil
}

func makeNodeMap(nodes []storage.Node) map[string]storage.Node {
	nodeMap := make(map[string]storage.Node)
	for _, n := range nodes {
		nodeMap[n.NodeID.String()] = n
	}
	return nodeMap
}
