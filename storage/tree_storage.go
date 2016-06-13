package storage

// ReadOnlyTreeTX represents a read-only transaction on a TreeStorage.
type ReadOnlyTreeTX interface {
	NodeReader
	Commit() error
}

// TreeTX represents an in-process tree-modifying transaction.
// The transaction must end with a call to Commit or Rollback.
// After a call to Commit or Rollback, all operations on the transaction will fail.
type TreeTX interface {
	NodeReaderWriter

	// Commit applies the operations performed to the underlying storage, or returns an error.
	Commit() error

	// Rollback aborts any performed operations. No updates must be applied to the underlying storage.
	Rollback() error
}

// NodeReader provides a read-only interface into the stored tree nodes.
type NodeReader interface {
	// GetMerkleNodes looks up the set of nodes identified by ids, at treeRevision, and returns them.
	GetMerkleNodes(treeRevision int64, ids []NodeID) ([]Node, error)
}

// NodeReaderWriter provides a read-write interface into the stored tree nodes.
type NodeReaderWriter interface {
	NodeReader

	// SetMerkleNodes stores the provided nodes, at treeRevision.
	SetMerkleNodes(treeRevision int64, nodes []Node) error
}
