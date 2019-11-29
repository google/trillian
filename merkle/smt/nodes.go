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
		return compareHorizontal(nodes[i].ID, nodes[j].ID)
	})
	for i, last := 0, len(nodes)-1; i < last; i++ {
		if id := nodes[i].ID; id == nodes[i+1].ID {
			return fmt.Errorf("duplicate ID: %v", id)
		}
	}
	return nil
}

// compareHorizontal returns whether the first node ID is to the left from the
// second one. The result only makes sense for IDs at the same tree level.
func compareHorizontal(a, b tree.NodeID2) bool {
	if res := strings.Compare(a.FullBytes(), b.FullBytes()); res != 0 {
		return res < 0
	}
	aLast, _ := a.LastByte()
	bLast, _ := b.LastByte()
	return aLast < bLast
}
