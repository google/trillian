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

// Package cloudspanner provides implementation of the Skylog storage API in
// Cloud Spanner.
package cloudspanner

import (
	"context"

	"cloud.google.com/go/spanner"
	"github.com/google/trillian/merkle/compact"
	"github.com/google/trillian/skylog/storage"
)

// TreeStorage allows reading from and writing to a tree storage.
type TreeStorage struct {
	c    *spanner.Client
	id   int64
	opts TreeOpts
}

// NewTreeStorage returns a new TreeStorage for the specified tree and options.
func NewTreeStorage(c *spanner.Client, treeID int64, opts TreeOpts) *TreeStorage {
	return &TreeStorage{c: c, id: treeID, opts: opts}
}

// Read fetches Merkle tree hashes of the passed in nodes from the storage.
// TODO(pavelkalinnikov): Add nodes cache.
func (t *TreeStorage) Read(ctx context.Context, ids []compact.NodeID) ([][]byte, error) {
	keys := make([]spanner.KeySet, 0, len(ids))
	for _, id := range ids {
		keys = append(keys, spanner.Key{t.id, t.opts.shardID(id), packNodeID(id)})
	}
	keySet := spanner.KeySets(keys...)
	hashes := make([][]byte, 0, len(ids))

	iter := t.c.Single().Read(ctx, "TreeNodes", keySet, []string{"NodeHash"})
	if err := iter.Do(func(r *spanner.Row) error {
		var hash []byte
		if err := r.Column(0, &hash); err != nil {
			return err
		}
		hashes = append(hashes, hash)
		return nil
	}); err != nil {
		return nil, err
	}
	return hashes, nil
}

// Write stores all the passed-in nodes in the tree storage.
func (t *TreeStorage) Write(ctx context.Context, nodes []storage.Node) error {
	ms := make([]*spanner.Mutation, 0, len(nodes))
	for _, node := range nodes {
		// TODO(pavelkalinnikov): Consider doing just Insert when it is clear what
		// semantic the callers need.
		ms = append(ms, spanner.InsertOrUpdate("TreeNodes",
			[]string{"TreeID", "ShardID", "NodeID", "NodeHash"},
			[]interface{}{t.id, t.opts.shardID(node.ID), packNodeID(node.ID), node.Hash}))
	}
	_, err := t.c.Apply(ctx, ms)
	return err
}

// TreeOpts stores sharding parameters for tree storage.
//
// The sharding scheme is as follows. The lower ShardLevels levels are split
// into LeafShards shards, where each shard stores a periodic sub-structure of
// perfect subtrees. For example, if ShardLevels is 2, and LeafShards is 3 then
// the lower 2 levels are sharded as shown below:
//
//    0   1   2   0   1
//   / \ / \ / \ / \ / \
//   0 0 1 1 2 2 0 0 1 1 ...
//
// Additionally, a single shard number 3 is created for all the nodes from the
// levels above.
//
// Such schema optimizes for the case when nodes are written to the tree in a
// nearly sequential way. If many concurrent writes are happening, all shards
// will be involved in parallel, and Cloud Spanner will add splits in between.
//
// TODO(pavelkalinnikov): Shard higher levels as well for more scalability.
// TODO(pavelkalinnikov): Achieve better vertical locality with stratification.
// TODO(pavelkalinnikov): Store the parameters in per-tree metadata.
type TreeOpts struct {
	ShardLevels uint // Between 1 and 65.
	LeafShards  int32
}

func (o TreeOpts) shardID(id compact.NodeID) int32 {
	if id.Level >= o.ShardLevels {
		return o.LeafShards
	}
	offset := id.Index >> (o.ShardLevels - id.Level - 1)
	return int32(offset % uint64(o.LeafShards))
}

// packNodeID encodes the ID of the node into a single integer. The numbering
// scheme assumes that we have a tree of up to 63 levels, i.e. 2^63 leaves, as
// on the diagram below:
//
//   Level 63:          1
//                     / \
//   Level 62:        2   3
//                   / \ / \
//   Level 61:      4  5 6  7
//     ...       ...   ...   ...
//   Level  0:  (2^63) ... (2^64 - 1)
//
// TODO(pavelkalinnikov): Check bounds.
func packNodeID(id compact.NodeID) uint64 {
	return uint64(1)<<(63-id.Level) | id.Index
}
