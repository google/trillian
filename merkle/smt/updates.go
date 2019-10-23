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

// NodeUpdate represents an update of a node hash in a sparse Merkle tree.
type NodeUpdate struct {
	ID   tree.NodeID2
	Hash []byte
}

// Prepare sorts the updates slice for it to be usable by HStar3 algorithm and
// the sparse Merkle tree Writer. It also verifies that the nodes are placed at
// the required depth, and there are no duplicate IDs.
//
// TODO(pavelkalinnikov): Make this algorithm independent of NodeUpdate type.
func Prepare(updates []NodeUpdate, depth uint) error {
	for i := range updates {
		if d, want := updates[i].ID.BitLen(), depth; d != want {
			return fmt.Errorf("upd #%d: invalid depth %d, want %d", i, d, want)
		}
	}
	sort.Slice(updates, func(i, j int) bool {
		return compareHorizontal(updates[i].ID, updates[j].ID)
	})
	for i, last := 0, len(updates)-1; i < last; i++ {
		if id := updates[i].ID; id == updates[i+1].ID {
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
