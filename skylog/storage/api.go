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

// Package storage contains Skylog storage API and helpers.
package storage

import (
	"context"

	"github.com/google/trillian/merkle/compact"
)

// SequenceReader allows reading from a sequence storage.
type SequenceReader interface {
	// Read fetches the specified [begin, end) range of entries, and returns them
	// in order. May return a prefix of the requested range (potentially empty).
	Read(ctx context.Context, begin, end uint64) ([]Entry, error)
}

// SequenceWriter allows writing to a sequence storage.
type SequenceWriter interface {
	// Write stores all the passed-in entries to the sequence starting at the
	// specified begin index.
	Write(ctx context.Context, begin uint64, entries []Entry) error
}

// TreeReader allows reading from a tree storage.
type TreeReader interface {
	// Read fetches and returns the Merkle tree hashes of the specified nodes
	// from the storage. It is expected that the number of nodes is relatively
	// small, e.g. O(log N) for vertical proof-like reads. The returned slice
	// contains nil hashes for the nodes that are not found.
	Read(ctx context.Context, ids []compact.NodeID) ([][]byte, error)
}

// TreeWriter allows writing to a tree storage. The tree is append-only and
// immutable. This means that each node hash can be written only once,
// effectively when the corresponding subtree is fully populated to be a
// perfect binary tree, and its hash becomes final.
type TreeWriter interface {
	// Write stores all the passed in nodes of the Merkle tree. It must never be
	// called with the same node ID, but different hashes. This call may succeed
	// only partially and leave side effects, so the caller must ensure that they
	// can reliably reconstruct and retry the request even if they fail/restart.
	Write(ctx context.Context, nodes []Node) error
}
