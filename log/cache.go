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

package log

import (
	"bytes"

	"github.com/google/trillian/merkle/compact"
	"github.com/google/trillian/types"
)

// TreeCache caches per-tree information between sequencing runs.
//
// TODO(pavelkalinnikov): Support some cache clearing mechanism.
type TreeCache struct {
	cr map[int64]*compact.Range
}

// NewTreeCache returns a new tree cache.
func NewTreeCache() TreeCache {
	return TreeCache{cr: make(map[int64]*compact.Range)}
}

// CompactRange returns the cached compact range for the specified tree ID. If
// the passed in root doesn't match the cached tree (for example, the root has
// moved forward because another master took over sequencing in the meantime),
// then nil is returned. If the information is not found, nil is returned.
func (tc TreeCache) CompactRange(treeID int64, root *types.LogRootV1) *compact.Range {
	cr, ok := tc.cr[treeID]
	if !ok {
		return nil
	}
	if cr.Begin() != 0 || cr.End() != root.TreeSize {
		delete(tc.cr, treeID)
		return nil
	}
	hash, err := cr.GetRootHash(nil)
	if err != nil || !bytes.Equal(hash, root.RootHash) {
		delete(tc.cr, treeID)
		return nil
	}
	return cr
}

// Update puts the passed in compact tree into the cache.
func (tc TreeCache) Update(treeID int64, tree *compact.Range) {
	tc.cr[treeID] = tree
}
