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

package memory

import (
	"github.com/golang/glog"
	"github.com/google/btree"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/storagepb"
	stree "github.com/google/trillian/storage/tree"
)

// This file contains utilities that are not part of the Storage API contracts but may
// be useful for development or debugging.

// Dump ascends the tree, logging the items contained.
func Dump(t *btree.BTree) {
	t.Ascend(func(i btree.Item) bool {
		glog.Infof("%#v", i)
		return true
	})
}

// DumpSubtrees will traverse the the BTree and execute a callback on each subtree proto
// that it contains. The traversal will be 'in order' according to the BTree keys, which
// may not be useful at the application level.
func DumpSubtrees(ls storage.LogStorage, treeID int64, callback func(string, *storagepb.SubtreeProto)) {
	m := ls.(*memoryLogStorage)
	tree := m.trees[treeID]
	pi := subtreeKey(treeID, 0, stree.NodeID{})

	tree.store.AscendGreaterOrEqual(pi, func(bi btree.Item) bool {
		i := bi.(*kv)

		if _, ok := i.v.(*storagepb.SubtreeProto); !ok {
			// Then we've finished iterating over subtrees
			return false
		}
		callback(i.k, i.v.(*storagepb.SubtreeProto))
		return true
	})
}
