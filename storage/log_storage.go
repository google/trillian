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
	Commit() error

	// Rollback discards the read-only TX.
	Rollback() error

	// Close attempts to Rollback the TX if it's open, it's a noop otherwise.
	Close() error
}

// ReadOnlyLogTreeTX provides a read-only view into the Log data.
// A ReadOnlyLogTreeTX can only read from the tree specified in its creation.
type ReadOnlyLogTreeTX interface {
	ReadOnlyTreeTX
	LeafReader
	LogRootReader
}

// LogTreeTX is the transactional interface for reading/updating a Log.
// It extends the basic TreeTX interface with Log specific methods.
// After a call to Commit or Rollback implementations must be in a clean state and have
// released any resources owned by the LogTX.
// A LogTreeTX can only modify the tree specified in its creation.
type LogTreeTX interface {
	TreeTX
	LogRootReader
	LogRootWriter
	LeafReader
	LeafQueuer
	LeafDequeuer
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
	SnapshotForTree(ctx context.Context, treeID int64) (ReadOnlyLogTreeTX, error)
}

// LogStorage should be implemented by concrete storage mechanisms which want to support Logs.
type LogStorage interface {
	ReadOnlyLogStorage

	// BeginForTree starts a transaction for the specified treeID.
	// Either Commit or Rollback must be called when the caller is finished with
	// the returned object, and values read through it should only be propagated
	// if Commit returns without error.
	BeginForTree(ctx context.Context, treeID int64) (LogTreeTX, error)
}

// LeafQueuer provides a write-only interface for the queueing (but not necessarily integration) of leaves.
type LeafQueuer interface {
	// QueueLeaves enqueues leaves for later integration into the tree.
	// If error is nil, the returned slice of leaves will be the same size as the
	// input, and each entry will hold:
	//  - the existing leaf entry if a duplicate has been submitted
	//  - nil otherwise.
	// Duplicates are only reported if the underlying tree does not permit duplicates, and are
	// considered duplicate if their leaf.LeafIdentityHash matches.
	QueueLeaves(ctx context.Context, leaves []*trillian.LogLeaf, queueTimestamp time.Time) ([]*trillian.LogLeaf, error)
}

// LeafDequeuer provides an interface for reading previously queued leaves for integration into the tree.
type LeafDequeuer interface {
	// DequeueLeaves will return between [0, limit] leaves from the queue.
	// Leaves which have been dequeued within a Rolled-back Tx will become available for dequeing again.
	// Leaves queued more recently than the cutoff time will not be returned. This allows for
	// guard intervals to be configured.
	DequeueLeaves(ctx context.Context, limit int, cutoffTime time.Time) ([]*trillian.LogLeaf, error)
	UpdateSequencedLeaves(ctx context.Context, leaves []*trillian.LogLeaf) error
}

// LeafReader provides a read only interface to stored tree leaves
type LeafReader interface {
	// GetSequencedLeafCount returns the total number of leaves that have been integrated into the
	// tree via sequencing.
	GetSequencedLeafCount(ctx context.Context) (int64, error)
	// GetLeavesByIndex returns leaf metadata and data for a set of specified sequenced leaf indexes.
	GetLeavesByIndex(ctx context.Context, leaves []int64) ([]*trillian.LogLeaf, error)
	// GetLeavesByHash looks up sequenced leaf metadata and data by their Merkle leaf hash. If the
	// tree permits duplicate leaves callers must be prepared to handle multiple results with the
	// same hash but different sequence numbers. If orderBySequence is true then the returned data
	// will be in ascending sequence number order.
	GetLeavesByHash(ctx context.Context, leafHashes [][]byte, orderBySequence bool) ([]*trillian.LogLeaf, error)
}

// LogRootReader provides an interface for reading SignedLogRoots.
type LogRootReader interface {
	// LatestSignedLogRoot returns the most recent SignedLogRoot, if any.
	LatestSignedLogRoot(ctx context.Context) (trillian.SignedLogRoot, error)
}

// LogRootWriter provides an interface for storing new SignedLogRoots.
type LogRootWriter interface {
	// StoreSignedLogRoot stores a freshly created SignedLogRoot.
	StoreSignedLogRoot(ctx context.Context, root trillian.SignedLogRoot) error
}

// CountByLogID is a map of total number of items keyed by log ID.
type CountByLogID map[int64]int64

// LogMetadata provides access to information about the logs in storage
type LogMetadata interface {
	// GetActiveLogs returns a list of the IDs of all the logs that are configured in storage
	GetActiveLogIDs(ctx context.Context) ([]int64, error)

	// GetUnsequencedCounts returns a map of the number of unsequenced entries
	// by log ID.
	//
	// This call is likely to be VERY expensive and take a long time to complete.
	// Consider carefully whether you really need to call it!
	GetUnsequencedCounts(ctx context.Context) (CountByLogID, error)
}
