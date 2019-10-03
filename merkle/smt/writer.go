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
	"errors"
	"fmt"

	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/storage/tree"
)

// NodeBatchAccessor reads and writes batches of Merkle tree node hashes. It is
// a batch interface for efficiency reasons, as it is designed to guard tree
// storage / database access. The Writer type operates on a per-shard basis,
// i.e. it calls Get and Set method exactly once for each shard.
type NodeBatchAccessor interface {
	// Get returns the hashes of the given nodes, as a map keyed by their IDs.
	// The returned hashes may be missing or be nil for empty subtrees.
	Get(ids []tree.NodeID2) (map[tree.NodeID2][]byte, error)
	// Set applies the given node hash updates.
	Set(upd []NodeUpdate) error
}

// Writer handles sharded writes to a sparse Merkle tree. The tree has two
// levels of shards: the single topmost shard spanning depths from 0 to split,
// and 2^split second-level shards each spanning levels from split to height.
// If the split height is 0 then effectively there is only one "global" shard.
type Writer struct {
	treeID int64
	hasher hashers.MapHasher
	height uint // The height of the tree.
	split  uint // The height of the top shard.
}

// NewWriter creates a new Writer for the specified tree of the given height,
// with two levels of sharding, where the upper shard is `split` levels high.
func NewWriter(treeID int64, hasher hashers.MapHasher, height, split uint) *Writer {
	if split > height {
		panic(fmt.Errorf("NewWriter: split(%d) > height(%d)", split, height))
	}
	return &Writer{treeID: treeID, hasher: hasher, height: height, split: split}
}

// Split sorts and splits the given list of node hash updates into shards, i.e.
// the subsets belonging to different subtrees. The updates must belong to the
// same tree level which is equal to the tree height.
func (w *Writer) Split(upd []NodeUpdate) ([][]NodeUpdate, error) {
	if err := sortUpdates(upd, w.height); err != nil {
		return nil, err
	}
	// TODO(pavelkalinnikov): Try estimating the capacity for this slice.
	var shards [][]NodeUpdate
	// The updates are sorted, so we can split them by prefix.
	for begin, i := 0, 0; i < len(upd); i++ {
		pref := upd[i].ID.Prefix(w.split)
		next := i + 1
		// Check if this ID ends the shard.
		if next == len(upd) || upd[next].ID.Prefix(w.split) != pref {
			shards = append(shards, upd[begin:next])
			begin = next
		}
	}
	return shards, nil
}

// Write applies the given list of updates to a single shard, and returns the
// resulting update of the shard root. It uses the given node accessor for
// reading and writing tree nodes.
//
// The typical usage pattern is as follows. For the lower shards, the input is
// the []NodeUpdate slices returned from the Split method. For the top shard,
// the input is all the NodeUpdates from the lower shards Write calls.
//
// In another case, Write can be performed without Split if the shard split
// depth is 0, which effectively means that there is only one "global" shard.
func (w *Writer) Write(upd []NodeUpdate, acc NodeBatchAccessor) (NodeUpdate, error) {
	if len(upd) == 0 {
		return NodeUpdate{}, errors.New("nothing to write")
	}
	depth := upd[0].ID.BitLen()
	top, err := w.shardTop(depth)
	if err != nil {
		return NodeUpdate{}, err
	}

	hs, err := NewHStar3(upd, w.hasher.HashChildren, depth, top)
	if err != nil {
		return NodeUpdate{}, err
	}
	nodes, err := acc.Get(hs.Prepare())
	if err != nil {
		return NodeUpdate{}, err
	}
	sa := w.newAccessor(nodes)
	topUpd, err := hs.Update(sa)
	if err != nil {
		return NodeUpdate{}, err
	} else if ln := len(topUpd); ln != 1 {
		return NodeUpdate{}, fmt.Errorf("writing across %d shards, want 1", ln)
	}
	if err := acc.Set(sa.writes); err != nil {
		return NodeUpdate{}, err
	}

	return topUpd[0], nil
}

// shardTop returns the depth of a shard top based on its bottom depth.
func (w *Writer) shardTop(depth uint) (uint, error) {
	switch depth {
	case w.height:
		return w.split, nil
	case w.split:
		return 0, nil
	}
	return 0, fmt.Errorf("unexpected depth %d", depth)
}

// newAccessor returns a NodeAccessor for HStar3 algorithm based on the set of
// preloaded node hashes.
func (w *Writer) newAccessor(nodes map[tree.NodeID2][]byte) *shardAccessor {
	// For any node that HStar3 reads, it also writes its sibling. Therefore we
	// can pre-allocate this many items for the writes slice.
	// TODO(pavelkalinnikov): The actual number of written nodes will be slightly
	// bigger by at most the number of written leaves. Try allocating precisely.
	writes := make([]NodeUpdate, 0, len(nodes))
	return &shardAccessor{w: w, reads: nodes, writes: writes}
}

// shardAccessor provides read and write access to nodes used by HStar3. It
// operates entirely in-memory.
type shardAccessor struct {
	w      *Writer
	reads  map[tree.NodeID2][]byte
	writes []NodeUpdate
}

// Get returns the hash of the given node from the preloaded map, or a hash of
// an empty subtree at this position if such node is not found.
func (s *shardAccessor) Get(id tree.NodeID2) ([]byte, error) {
	if hash, ok := s.reads[id]; ok && hash != nil {
		return hash, nil
	}
	return hashEmpty(s.w.hasher, s.w.treeID, id), nil
}

// Set adds the given node hash update to the list of writes.
func (s *shardAccessor) Set(id tree.NodeID2, hash []byte) {
	s.writes = append(s.writes, NodeUpdate{ID: id, Hash: hash})
}

// TODO(pavelkalinnikov): Make MapHasher.HashEmpty method take the id directly.
func hashEmpty(hasher hashers.MapHasher, treeID int64, id tree.NodeID2) []byte {
	index := make([]byte, hasher.Size())
	copy(index, id.FullBytes())
	if last, bits := id.LastByte(); bits != 0 {
		index[len(id.FullBytes())] = last
	}
	height := hasher.BitLen() - int(id.BitLen())
	return hasher.HashEmpty(treeID, index, height)
}
