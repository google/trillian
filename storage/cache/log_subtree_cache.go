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

package cache

import (
	"fmt"

	"github.com/google/trillian/merkle/compact"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/storagepb"
)

// NewLogSubtreeCache creates and returns a SubtreeCache appropriate for use with a log
// tree. The caller must supply the strata depths to be used and a suitable LogHasher.
func NewLogSubtreeCache(logStrata []int, hasher hashers.LogHasher) SubtreeCache {
	return NewSubtreeCache(logStrata, populateLogSubtreeNodes(hasher), prepareLogSubtreeWrite())
}

// LogPopulateFunc obtains a log storage population function based on a supplied LogHasher.
// This is intended for use by storage utilities.
func LogPopulateFunc(hasher hashers.LogHasher) storage.PopulateSubtreeFunc {
	return populateLogSubtreeNodes(hasher)
}

// populateLogSubtreeNodes re-creates a Log subtree's InternalNodes from the
// subtree Leaves map.
//
// This uses the compact Merkle tree to repopulate internal nodes, and so will
// handle imperfect (but left-hand dense) subtrees. Note that we only rebuild internal
// nodes when the subtree is fully populated. For an explanation of why see the comments
// below for PrepareLogSubtreeWrite.
func populateLogSubtreeNodes(hasher hashers.LogHasher) storage.PopulateSubtreeFunc {
	return func(st *storagepb.SubtreeProto) error {
		cmt := compact.NewTree(hasher)
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

		// We need to update the subtree root hash regardless of whether it's fully populated
		for leafIndex := int64(0); leafIndex < int64(len(st.Leaves)); leafIndex++ {
			nodeID := storage.NewNodeIDFromPrefix(st.Prefix, logStrataDepth, leafIndex, logStrataDepth, maxLogDepth)
			_, sfx := nodeID.Split(len(st.Prefix), int(st.Depth))
			sfxKey := sfx.String()
			h := st.Leaves[sfxKey]
			if h == nil {
				return fmt.Errorf("unexpectedly got nil for subtree leaf suffix %s", sfx)
			}
			seq, err := cmt.AddLeafHash(h, func(level uint, index uint64, h []byte) {
				if level == logStrataDepth && index == 0 {
					// no space for the root in the node cache
					return
				}

				// Don't put leaves into the internal map and only update if we're rebuilding internal
				// nodes. If the subtree was saved with internal nodes then we don't touch the map.
				if level > 0 && len(st.Leaves) == maxLeaves {
					subDepth := logStrataDepth - int(level)
					// TODO(Martin2112): See if we can possibly avoid the expense hiding inside NewNodeIDFromPrefix.
					nodeID := storage.NewNodeIDFromPrefix(st.Prefix, subDepth, int64(index), logStrataDepth, maxLogDepth)
					_, sfx := nodeID.Split(len(st.Prefix), int(st.Depth))
					sfxKey := sfx.String()
					st.InternalNodes[sfxKey] = h
				}
			})
			if err != nil {
				return err
			}
			if got, expected := seq, leafIndex; got != expected {
				return fmt.Errorf("got seq of %d, but expected %d", got, expected)
			}
		}
		st.RootHash = cmt.CurrentRoot()

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
}

// prepareLogSubtreeWrite prepares a log subtree for writing. If the subtree is fully
// populated the internal nodes are cleared. Otherwise they are written.
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
func prepareLogSubtreeWrite() storage.PrepareSubtreeWriteFunc {
	return func(st *storagepb.SubtreeProto) error {
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
}
