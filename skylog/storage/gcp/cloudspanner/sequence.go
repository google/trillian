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

package cloudspanner

import (
	"context"
	"errors"

	"cloud.google.com/go/spanner"
	"github.com/google/trillian/skylog/storage"
)

var errBreak = errors.New("break")

// SequenceOpts configures the sequence storage sharding mechanism.
//
// Log entries are split into the configured number of Shards, where each shard
// stores a periodic sub-sequence of batches of BatchSize. For example, if
// Shards is 3, and BatchSize is 2 then the entries are sharded as follows:
//
//   0 0 1 1 2 2 0 0 1 1 ...
//
// Such schema optimizes for the case when entries are written in a nearly
// sequential way. If many concurrent writes are happening, all shards will be
// involved in parallel, and Cloud Spanner will add splits in between.
//
// TODO(pavelkalinnikov): Store the parameters in per-tree metadata.
type SequenceOpts struct {
	BatchSize uint64
	Shards    uint64
}

// SequenceStorage allows reading from and writing to a sequence storage.
type SequenceStorage struct {
	c    *spanner.Client
	id   int64
	opts SequenceOpts
}

// NewSequenceStorage returns a new SequenceStorage.
func NewSequenceStorage(c *spanner.Client, treeID int64, opts SequenceOpts) *SequenceStorage {
	return &SequenceStorage{c: c, id: treeID, opts: opts}
}

// Read fetches the specified [begin, end) range of entries, and returns them
// in order. May return a prefix of the requested range if it spans multiple
// shards or some entries are missing.
func (s *SequenceStorage) Read(ctx context.Context, begin, end uint64) ([]storage.Entry, error) {
	if end <= begin { // Empty range.
		return nil, nil
	}
	// TODO(pavelkalinnikov): Restrict the range length.

	// We only read entries from the shard that includes the begin-th entry.
	// TODO(pavelkalinnikov): Unit-test this logic.
	offset := begin / s.opts.BatchSize
	if next := (offset + 1) * s.opts.BatchSize; next < end {
		end = next
	}
	shardID := int64(offset % s.opts.Shards)

	keys := spanner.KeyRange{
		Start: spanner.Key{s.id, shardID, int64(begin)},
		End:   spanner.Key{s.id, shardID, int64(end)},
		Kind:  spanner.ClosedOpen,
	}
	iter := s.c.Single().Read(ctx, "Entries", keys, []string{"EntryIndex", "Data", "Extra"})

	next := int64(begin)
	ret := make([]storage.Entry, 0, end-begin)
	if err := iter.Do(func(r *spanner.Row) error {
		var index int64
		var data, extra []byte
		if err := r.Columns(&index, &data, &extra); err != nil {
			return err
		}
		if index != next {
			return errBreak
		}
		next++
		ret = append(ret, storage.Entry{Data: data, Extra: extra})
		return nil
	}); err != nil && err != errBreak {
		return nil, err
	}

	return ret, nil
}

// Write stores all the passed-in entries to the sequence starting at the
// specified begin index.
func (s *SequenceStorage) Write(ctx context.Context, begin uint64, entries []storage.Entry) error {
	ms := make([]*spanner.Mutation, 0, len(entries))
	for i, entry := range entries {
		index := int64(begin) + int64(i)
		// TODO(pavelkalinnikov): Split the transaction by shard ID.
		shardID := index / int64(s.opts.BatchSize) % int64(s.opts.Shards)
		// TODO(pavelkalinnikov): Consider doing just Insert when it is clear what
		// semantic the callers need.
		ms = append(ms, spanner.InsertOrUpdate("Entries",
			[]string{"TreeID", "ShardID", "EntryIndex", "Data", "Extra"},
			[]interface{}{s.id, shardID, index, entry.Data, entry.Extra}))
	}
	_, err := s.c.Apply(ctx, ms)
	return err
}
