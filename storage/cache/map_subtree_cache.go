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
	"encoding/base64"
	"fmt"
	"math/big"

	"github.com/golang/glog"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/storagepb"
)

// NewMapSubtreeCache creates and returns a SubtreeCache appropriate for use with a map
// tree. The caller must supply the strata depths to be used, the treeID and a suitable MapHasher.
func NewMapSubtreeCache(mapStrata []int, treeID int64, hasher hashers.MapHasher) SubtreeCache {
	return NewSubtreeCache(mapStrata, populateMapSubtreeNodes(treeID, hasher), prepareMapSubtreeWrite())
}

// populateMapSubtreeNodes re-creates Map subtree's InternalNodes from the
// subtree Leaves map.
//
// This uses HStar2 to repopulate internal nodes.
func populateMapSubtreeNodes(treeID int64, hasher hashers.MapHasher) storage.PopulateSubtreeFunc {
	return func(st *storagepb.SubtreeProto) error {
		st.InternalNodes = make(map[string][]byte)
		leaves := make([]merkle.HStar2LeafHash, 0, len(st.Leaves))
		for k64, v := range st.Leaves {
			sfx, err := storage.ParseSuffix(k64)
			if err != nil {
				return err
			}
			// TODO(gdbelvin): test against subtree depth.
			if sfx.Bits()%depthQuantum != 0 {
				return fmt.Errorf("unexpected non-leaf suffix found: %x", sfx.Bits())
			}

			leaves = append(leaves, merkle.HStar2LeafHash{
				Index:    storage.NewNodeIDFromPrefixSuffix(st.Prefix, sfx, hasher.BitLen()).BigInt(),
				LeafHash: v,
			})
		}
		hs2 := merkle.NewHStar2(treeID, hasher)
		root, err := hs2.HStar2Nodes(st.Prefix, int(st.Depth), leaves, nil,
			func(depth int, index *big.Int, h []byte) error {
				if depth == len(st.Prefix)*8 && len(st.Prefix) > 0 {
					// no space for the root in the node cache
					return nil
				}
				nodeID := storage.NewNodeIDFromBigInt(depth, index, hasher.BitLen())
				_, sfx := nodeID.Split(len(st.Prefix), int(st.Depth))
				sfxKey := sfx.String()
				if glog.V(4) {
					b, err := base64.StdEncoding.DecodeString(sfxKey)
					if err != nil {
						glog.Errorf("base64.DecodeString(%v): %v", sfxKey, err)
					}
					glog.Infof("PopulateMapSubtreeNodes.Set(%x, %d) suffix: %x: %x", index.Bytes(), depth, b, h)
				}
				st.InternalNodes[sfxKey] = h
				return nil
			})
		if err != nil {
			return err
		}
		st.RootHash = root
		return err
	}
}

// prepareMapSubtreeWrite prepares a map subtree for writing. For maps the internal
// nodes are never written to storage and are thus always cleared.
func prepareMapSubtreeWrite() storage.PrepareSubtreeWriteFunc {
	return func(st *storagepb.SubtreeProto) error {
		st.InternalNodes = nil
		// We don't check the node count for map subtrees but ensure it's zero for consistency
		st.InternalNodeCount = 0
		return nil
	}
}
