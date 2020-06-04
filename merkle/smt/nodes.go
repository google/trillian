// Copyright 2019 Google Inc. All Rights Reserved.
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

package smt

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/trillian/storage/tree"
)

// Node represents a sparse Merkle tree node.
type Node struct {
	ID   tree.NodeID2
	Hash []byte
}

// Prepare sorts the nodes slice for it to be usable by HStar3 algorithm and
// the sparse Merkle tree Writer. It also verifies that the nodes are placed at
// the required depth, and there are no duplicate IDs.
//
// TODO(pavelkalinnikov): Make this algorithm independent of Node type.
func Prepare(nodes []Node, depth uint) error {
	for i := range nodes {
		if d, want := nodes[i].ID.BitLen(), depth; d != want {
			return fmt.Errorf("node #%d: invalid depth %d, want %d", i, d, want)
		}
	}
	sort.Slice(nodes, func(i, j int) bool {
		return compareHorizontal(nodes[i].ID, nodes[j].ID) < 0
	})
	for i, last := 0, len(nodes)-1; i < last; i++ {
		if id := nodes[i].ID; id == nodes[i+1].ID {
			return fmt.Errorf("duplicate ID: %v", id)
		}
	}
	return nil
}

// compareHorizontal compares relative position of two node IDs at the same
// tree level. Returns -1 if the first node is to the left from the second one,
// 1 if the first node is to the right, and 0 if IDs are the same. The result
// is undefined if nodes are not at the same level.
func compareHorizontal(a, b tree.NodeID2) int {
	if res := strings.Compare(a.FullBytes(), b.FullBytes()); res != 0 {
		return res
	}
	aLast, _ := a.LastByte()
	bLast, _ := b.LastByte()
	if aLast == bLast {
		return 0
	} else if aLast < bLast {
		return -1
	}
	return 1
}
