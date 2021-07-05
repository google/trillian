// Copyright 2017 Google LLC. All Rights Reserved.
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

package cache

import (
	"encoding/binary"
	"fmt"

	"github.com/golang/glog"
	"github.com/google/trillian/merkle/compact"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/storage/storagepb"
	"github.com/google/trillian/storage/tree"
)

const (
	// logStrataDepth is the strata that must be used for all log subtrees.
	logStrataDepth = 8
	// maxLogDepth is the number of bits in a log path.
	maxLogDepth = 64
)

// PopulateLogTile re-creates a log tile's InternalNodes from the Leaves map.
//
// This uses the compact Merkle tree to repopulate internal nodes, and so will
// handle imperfect (but left-hand dense) subtrees. Note that we only rebuild internal
// nodes when the subtree is fully populated. For an explanation of why see the comments
// below for prepareLogTile.
//
// TODO(pavelkalinnikov): Unexport it after the refactoring.
func PopulateLogTile(st *storagepb.SubtreeProto, hasher hashers.LogHasher) error {
	if st.Depth < 1 {
		return fmt.Errorf("populate log subtree with invalid depth: %d", st.Depth)
	}
	// maxLeaves is the number of leaves that fully populates a subtree of the depth we are
	// working with.
	maxLeaves := 1 << uint(st.Depth)

	// If the subtree is fully populated then the internal node map is expected to be nil but in
	// case it isn't we recreate it as we're about to rebuild the contents. We'll check
	// below that the number of nodes is what we expected to have.
	if st.InternalNodes == nil || len(st.Leaves) == maxLeaves {
		st.InternalNodes = make(map[string][]byte)
	}
	store := func(id compact.NodeID, hash []byte) {
		if id.Level == logStrataDepth && id.Index == 0 {
			// no space for the root in the node cache
			return
		}

		// Don't put leaves into the internal map and only update if we're rebuilding internal
		// nodes. If the subtree was saved with internal nodes then we don't touch the map.
		if id.Level > 0 && len(st.Leaves) == maxLeaves {
			st.InternalNodes[toSuffix(id)] = hash
		}
	}

	fact := compact.RangeFactory{Hash: hasher.HashChildren}
	cr := fact.NewEmptyRange(0)

	// We need to update the subtree root hash regardless of whether it's fully populated
	for leafIndex := uint64(0); leafIndex < uint64(len(st.Leaves)); leafIndex++ {
		sfxKey := toSuffix(compact.NewNodeID(0, leafIndex))
		h := st.Leaves[sfxKey]
		if h == nil {
			return fmt.Errorf("unexpectedly got nil for subtree leaf suffix %s", sfxKey)
		}
		if size, expected := cr.End(), leafIndex; size != expected {
			return fmt.Errorf("got size of %d, but expected %d", size, expected)
		}
		if err := cr.Append(h, store); err != nil {
			return err
		}
	}
	root, err := cr.GetRootHash(store)
	if err != nil {
		return fmt.Errorf("failed to compute root hash: %v", err)
	}
	st.RootHash = root

	// Additional check - after population we should have the same number of internal nodes
	// as before the subtree was written to storage. Either because they were loaded from
	// storage or just rebuilt above.
	if got, want := uint32(len(st.InternalNodes)), st.InternalNodeCount; got != want {
		// TODO(Martin2112): Possibly replace this with stronger checks on the data in
		// subtrees on disk so we can detect corruption.
		return fmt.Errorf("log repop got: %d internal nodes, want: %d", got, want)
	}

	return nil
}

// prepareLogTile prepares a log tile for writing. If it is fully populated the
// internal nodes are cleared. Otherwise they are written.
//
// To see why this is necessary consider the case where a tree has a single full subtree
// and then an additional leaf is added.
//
// This causes an extra level to be added to the tree with an internal node that is a hash
// of the root of the left full subtree and the new leaf. Note that the leaves remain at
// level zero in the overall tree coordinate space but they are now in a lower subtree stratum
// than they were before the last node was added as the tree has grown above them.
//
// Thus in the case just discussed the internal nodes cannot be correctly reconstructed
// in isolation when the tree is reloaded because of the dependency on another subtree.
//
// Fully populated subtrees don't have this problem because by definition they can only
// contain internal nodes built from their own contents.
func prepareLogTile(st *storagepb.SubtreeProto) error {
	st.InternalNodeCount = uint32(len(st.InternalNodes))
	if st.Depth < 1 {
		return fmt.Errorf("prepare subtree for log write invalid depth: %d", st.Depth)
	}
	maxLeaves := 1 << uint(st.Depth)
	// If the subtree is fully populated we can safely clear the internal nodes
	if len(st.Leaves) == maxLeaves {
		st.InternalNodes = nil
	}
	return nil
}

func toSuffix(id compact.NodeID) string {
	depth := logStrataDepth - int(id.Level)
	var index [8]byte
	binary.BigEndian.PutUint64(index[:], id.Index<<(maxLogDepth-depth))
	return tree.NewSuffix(uint8(depth), index[:]).String()
}

// newEmptySubtree creates an empty subtree for the passed-in ID.
func newEmptySubtree(id []byte) *storagepb.SubtreeProto {
	if glog.V(2) {
		glog.Infof("Creating new empty subtree for %x", id)
	}
	// Storage didn't have one for us, so we'll store an empty proto here in case
	// we try to update it later on (we won't flush it back to storage unless
	// it's been written to).
	return &storagepb.SubtreeProto{
		Prefix:        id,
		Depth:         8,
		Leaves:        make(map[string][]byte),
		InternalNodes: make(map[string][]byte),
	}
}
