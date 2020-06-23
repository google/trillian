// Copyright 2016 Google Inc. All Rights Reserved.
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

package storage

import (
	"context"
	"time"

	"github.com/google/trillian"
)

// ReadOnlyLogTX provides a read-only view into log data.
// A ReadOnlyLogTX, unlike ReadOnlyLogTreeTX, is not tied to a particular tree.
type ReadOnlyLogTX interface {
	LogMetadata

	// Commit ensures the data read by the TX is consistent in the database. Only after Commit the
	// data read should be regarded as valid.
	Commit(context.Context) error

	// Rollback discards the read-only TX.
	Rollback() error

	// Close attempts to Rollback the TX if it's open, it's a noop otherwise.
	Close() error
}

// ReadOnlyLogTreeTX provides a read-only view into the Log data.
// A ReadOnlyLogTreeTX can only read from the tree specified in its creation.
type ReadOnlyLogTreeTX interface {
	ReadOnlyTreeTX

	// GetSequencedLeafCount returns the total number of leaves that have been integrated into the
	// tree via sequencing.
	GetSequencedLeafCount(ctx context.Context) (int64, error)
	// GetLeavesByIndex returns leaf metadata and data for a set of specified sequenced leaf indexes.
	GetLeavesByIndex(ctx context.Context, leaves []int64) ([]*trillian.LogLeaf, error)
	// GetLeavesByRange returns leaf data for a range of indexes. The returned
	// slice is a contiguous prefix of leaves in [start, start+count) ordered by
	// LeafIndex. It will be shorter than `count` if the requested range has
	// missing entries (e.g., it extends beyond the size of a LOG tree), or
	// `count` is too big to handle in one go.
	// For PREORDERED_LOG trees, *must* return leaves beyond the tree size if
	// they are stored, in order to allow integrating them into the tree.
	GetLeavesByRange(ctx context.Context, start, count int64) ([]*trillian.LogLeaf, error)
	// GetLeavesByHash looks up sequenced leaf metadata and data by their Merkle leaf hash. If the
	// tree permits duplicate leaves callers must be prepared to handle multiple results with the
	// same hash but different sequence numbers. If orderBySequence is true then the returned data
	// will be in ascending sequence number order.
	GetLeavesByHash(ctx context.Context, leafHashes [][]byte, orderBySequence bool) ([]*trillian.LogLeaf, error)
	// LatestSignedLogRoot returns the most recent SignedLogRoot, if any.
	LatestSignedLogRoot(ctx context.Context) (*trillian.SignedLogRoot, error)
}

// LogTreeTX is the transactional interface for reading/updating a Log.
// It extends the basic TreeTX interface with Log specific methods.
// After a call to Commit or Rollback implementations must be in a clean state and have
// released any resources owned by the LogTX.
// A LogTreeTX can only modify the tree specified in its creation.
type LogTreeTX interface {
	ReadOnlyLogTreeTX
	TreeWriter

	// StoreSignedLogRoot stores a freshly created SignedLogRoot.
	StoreSignedLogRoot(ctx context.Context, root *trillian.SignedLogRoot) error

	// DequeueLeaves returns between [0, limit] leaves to be integrated to the
	// tree.
	//
	// For LOG trees:
	// - The leaves are taken from the queue.
	// - If the Tx is rolled back, they become available for dequeueing again.
	//
	// For PREORDERED_LOG trees:
	// - The leaves are taken from the head of as yet un-integrated part of the
	//   sequenced entries, immediately following the current SignedLogRoot tree
	//   size.
	// - The operation is a no-op with regards to the sequenced entries.
	//
	// Leaves queued more recently than the cutoff time will not be returned.
	// This allows for guard intervals to be configured, and (in case of
	// PREORDERED_LOG trees) avoiding contention between log signer and writers
	// appending new entries.
	//
	// This method is not required to return fully populated LogLeaf structures,
	// but it *must* include MerkleLeafHash, QueueTimestamp, and LeafIndex (for
	// PREORDERED_LOG trees). Storage implementations might apply optimizations
	// employing this property. Consult the call sites of this method to be sure.
	DequeueLeaves(ctx context.Context, limit int, cutoff time.Time) ([]*trillian.LogLeaf, error)

	// AddSequencedLeaves stores the passed in leaves at the log positions
	// specified in the `LeafIndex` field. The indices must be contiguous.
	//
	// See LogStorage.AddSequencedLeaves comment for more details.
	AddSequencedLeaves(ctx context.Context, leaves []*trillian.LogLeaf, timestamp time.Time) ([]*trillian.QueuedLogLeaf, error)

	// UpdateSequencedLeaves associates the leaves with the sequence numbers
	// assigned to them.
	UpdateSequencedLeaves(ctx context.Context, leaves []*trillian.LogLeaf) error
}

// ReadOnlyLogStorage represents a narrowed read-only view into a LogStorage.
type ReadOnlyLogStorage interface {
	DatabaseChecker

	// Snapshot starts a read-only transaction not tied to any particular tree.
	Snapshot(ctx context.Context) (ReadOnlyLogTX, error)

	// SnapshotForTree starts a read-only transaction for the specified treeID.
	// Commit must be called when the caller is finished with the returned object,
	// and values read through it should only be propagated if Commit returns
	// without error.
	SnapshotForTree(ctx context.Context, tree *trillian.Tree) (ReadOnlyLogTreeTX, error)
}

// LogTXFunc is the func signature for passing into ReadWriteTransaction.
type LogTXFunc func(context.Context, LogTreeTX) error

// LogStorage should be implemented by concrete storage mechanisms which want to support Logs.
type LogStorage interface {
	ReadOnlyLogStorage

	// ReadWriteTransaction starts a RW transaction on the underlying storage, and
	// calls f with it.
	// If f fails and returns an error, the storage implementation may optionally
	// retry with a new transaction, and f MUST NOT keep state across calls.
	ReadWriteTransaction(ctx context.Context, tree *trillian.Tree, f LogTXFunc) error

	// QueueLeaves enqueues leaves for later integration into the tree.
	// If error is nil, the returned slice of leaves will be the same size as the
	// input, and each entry will hold a passed-in leaf struct and a Status
	// representing the outcome for that particular leaf:
	//  * a status of OK indicates that the leaf was successfully queued.
	//  * a status of AlreadyExists indicates that the leaf was a duplicate, in this case
	//    the returned leaf data is that of the original.
	// Other status values may be returned in error cases.
	//
	// Duplicates are only reported if the underlying tree does not permit duplicates, and are
	// considered duplicate if their leaf.LeafIdentityHash matches.
	QueueLeaves(ctx context.Context, tree *trillian.Tree, leaves []*trillian.LogLeaf, queueTimestamp time.Time) ([]*trillian.QueuedLogLeaf, error)

	// AddSequencedLeaves stores the `leaves` and associates them with the log
	// positions according to their `LeafIndex` field. The indices must be
	// contiguous.
	//
	// If error is nil, the returned slice is the same size as the input, entries
	// correspond to the `leaves` in the same order. Each entry describes the
	// result of adding the corresponding leaf.
	//
	// Possible `QueuedLogLeaf.status` values with their semantics:
	//  - OK: The leaf has been successfully stored.
	//  - AlreadyExists: The storage already contains an identical leaf at the
	//    specified `LeafIndex`. That leaf is returned in `QueuedLogLeaf.leaf`.
	//  - FailedPrecondition: There is another leaf with the same `LeafIndex`,
	//    but a different value. That leaf is returned in `QueuedLogLeaf.leaf`.
	//  - OutOfRange: The leaf can not be stored at the specified `LeafIndex`.
	//    For example, the storage might not support non-sequential writes.
	//  - Internal, etc: A storage-specific error.
	//
	// TODO(pavelkalinnikov): Make returning the resulting/conflicting leaves
	// optional. Channel these options to the top-level Log API.
	// TODO(pavelkalinnikov): Not checking values of the occupied indices might
	// be a good optimization. Could also be optional.
	AddSequencedLeaves(ctx context.Context, tree *trillian.Tree, leaves []*trillian.LogLeaf, timestamp time.Time) ([]*trillian.QueuedLogLeaf, error)
}

// CountByLogID is a map of total number of items keyed by log ID.
type CountByLogID map[int64]int64

// LogMetadata provides access to information about the logs in storage
type LogMetadata interface {
	// GetActiveLogIDs returns a list of the IDs of all the logs that are
	// configured in storage and are eligible to have entries sequenced.
	GetActiveLogIDs(ctx context.Context) ([]int64, error)
}
