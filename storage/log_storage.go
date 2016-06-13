package storage

import (
	"github.com/google/trillian"
)

// ReadOnlyLogTX provides a read-only view into the Log data.
type ReadOnlyLogTX interface {
	ReadOnlyTreeTX
	LeafReader
	LogRootReader
}

// LogTX is the transactional interface for reading/updating a Log.
// It extends the basic TreeTX interface with Log specific methods.
type LogTX interface {
	TreeTX
	LogRootReader
	LogRootWriter
	LeafReader
	LeafQueuer
	LeafDequeuer
}

// ReadOnlyLogStorage represents a narrowed read-only view into a LogStorage.
type ReadOnlyLogStorage interface {
	// Snapshot starts a read-only transaction.
	// Commit must be called when the caller is finished with the returned object,
	// and values read through it should only be propagated if Commit returns
	// without error.
	Snapshot() (ReadOnlyLogTX, error)
}

// LogStorage should be implemented by concrete storage mechanisms which want to support Logs.
type LogStorage interface {
	ReadOnlyLogStorage

	// Begin starts a new Log transaction.
	// Either Commit or Rollback must be called when the caller is finished with
	// the returned object, and values read through it should only be propagated
	// if Commit returns without error.
	Begin() (LogTX, error)
}

// LeafQueuer provides a write-only interface for the queueing (but not necesarily integration) of leaves.
type LeafQueuer interface {
	// QueueLeaves enqueues leaves for later integration into the tree.
	QueueLeaves(leaves []trillian.LogLeaf) error
}

// LeafDequeuer provides an interface for reading previously queued leaves for integration into the tree.
type LeafDequeuer interface {
	// DequeueLeaves will return between [0, limit] leaves from the queue.
	// Leaves which have been dequeued within a Rolled-back Tx will become available for dequeing again.
	DequeueLeaves(limit int) ([]trillian.LogLeaf, error)
	UpdateSequencedLeaves([]trillian.LogLeaf) error
}

// LeafReader provides a read only interface to stored tree leaves
type LeafReader interface {
	// GetSequencedLeafCount returns the total number of leaves that have been integrated into the
	// tree via sequencing.
	GetSequencedLeafCount() (int64, error)
	// GetLeavesByIndex returns leaf metadata and data for a set of specified sequenced leaf indexes.
	GetLeavesByIndex(leaves []int64) ([]trillian.LogLeaf, error)
	// GetLeavesByHash looks up sequenced leaf metadata and data by their hash. If the tree permits
	// duplicate leaves callers must be prepared to handle multiple results with the same hash
	// but different sequence numbers.
	GetLeavesByHash(leafHashes []trillian.Hash) ([]trillian.LogLeaf, error)
}

// LogRootReader provides an interface for reading SignedLogRoots.
type LogRootReader interface {
	// LatestSignedLogRoot returns the most recent SignedLogRoot, if any.
	LatestSignedLogRoot() (trillian.SignedLogRoot, error)
}

// LogRootWriter provides an interface for storing new SignedLogRoots.
type LogRootWriter interface {
	// StoreSignedLogRoot stores a freshly created SignedLogRoot.
	StoreSignedLogRoot(root trillian.SignedLogRoot) error
}
