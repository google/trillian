package storage

// ReadOnlyTreeTX represents a read-only transaction on a TreeStorage.
type ReadOnlyTreeTX interface {
	NodeReader
	Commit() error
	Rollback() error
}

// TreeTX represents an in-process tree-modifying transaction.
// The transaction must end with a call to Commit or Rollback.
// After a call to Commit or Rollback, all operations on the transaction will fail.
// After a call to Commit or Rollback implementations must be in a clean state and have
// released any resources owned by the TreeTX.
type TreeTX interface {
	NodeReaderWriter

	// Commit applies the operations performed to the underlying storage, or returns an error.
	Commit() error

	// Rollback aborts any performed operations. No updates must be applied to the underlying storage.
	Rollback() error

	// Open indicates if this transaction is open. An open transaction is one for which
	// Commit() or Rollback() has never been called. Implementations must do all clean up
	// in these methods so transactions are assumed closed regardless of the reported success.
	IsOpen() bool

	// WriteRevision returns the tree revision that any writes through this TreeTX will be stored at.
	WriteRevision() int64
}

// NodeReader provides a read-only interface into the stored tree nodes.
type NodeReader interface {
	// GetTreeRevisionAtSize returns the max node version for a tree at a particular size.
	// It is an error to request tree sizes larger than the currently published tree size.
	// If exact is true then a tree revision will only be returned if one exists for the specified
	// tree size. If exact is false then the revision for an arbitrary larger size is returned.
	GetTreeRevisionAtSize(treeSize int64, exact bool) (int64, error)
	// GetMerkleNodes looks up the set of nodes identified by ids, at treeRevision, and returns them.
	GetMerkleNodes(treeRevision int64, ids []NodeID) ([]Node, error)
}

// NodeReaderWriter provides a read-write interface into the stored tree nodes.
type NodeReaderWriter interface {
	NodeReader

	// SetMerkleNodes stores the provided nodes, at the transaction's writeRevision.
	SetMerkleNodes(nodes []Node) error
}
