// Package storage contains Skylog storage API and helpers.
package storage

import (
	"context"

	"github.com/google/trillian/merkle/compact"
)

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
