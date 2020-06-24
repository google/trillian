// Copyright 2019 Google LLC. All Rights Reserved.
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
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/storage/tree"
)

// mapHasher is a wrapper around MapHasher bound to a specific tree ID.
type mapHasher struct {
	mh     hashers.MapHasher
	treeID int64
}

// bindHasher returns a mapHasher binding the given hasher to a tree ID.
func bindHasher(hasher hashers.MapHasher, treeID int64) mapHasher {
	return mapHasher{mh: hasher, treeID: treeID}
}

// hashEmpty returns the hash of an empty subtree with the given root ID.
func (h mapHasher) hashEmpty(id tree.NodeID2) []byte {
	oldID := tree.NewNodeIDFromID2(id)
	height := h.mh.BitLen() - oldID.PrefixLenBits
	// TODO(pavelkalinnikov): Make HashEmpty method take the NodeID2 directly.
	return h.mh.HashEmpty(h.treeID, oldID.Path, height)
}
