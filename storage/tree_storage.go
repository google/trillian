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
)

// ReadOnlyTreeTX represents a read-only transaction on a TreeStorage.
// A ReadOnlyTreeTX can only modify the tree specified in its creation.
type ReadOnlyTreeTX interface {
	NodeReader

	// ReadRevision returns the tree revision that was current at the time this
	// transaction was started.
	ReadRevision() int64

	// Commit attempts to commit any reads performed under this transaction.
	Commit() error

	// Rollback aborts this transaction.
	Rollback() error

	// Close attempts to Rollback the TX if it's open, it's a noop otherwise.
	Close() error

	// Open indicates if this transaction is open. An open transaction is one for which
	// Commit() or Rollback() has never been called. Implementations must do all clean up
	// in these methods so transactions are assumed closed regardless of the reported success.
	IsOpen() bool
}

// TreeTX represents an in-process tree-modifying transaction.
// The transaction must end with a call to Commit or Rollback.
// After a call to Commit or Rollback, all operations on the transaction will fail.
// After a call to Commit or Rollback implementations must be in a clean state and have
// released any resources owned by the TreeTX.
// A TreeTX can only modify the tree specified in its creation.
type TreeTX interface {
	ReadOnlyTreeTX
	NodeWriter

	// WriteRevision returns the tree revision that any writes through this TreeTX will be stored at.
	WriteRevision() int64
}

// DatabaseChecker performs connectivity checks on the database.
type DatabaseChecker interface {
	// CheckDatabaseAccessible returns nil if the database is accessible, error otherwise.
	CheckDatabaseAccessible(context.Context) error
}

// NodeReader provides a read-only interface into the stored tree nodes.
type NodeReader interface {
	// GetMerkleNodes looks up the set of nodes identified by ids, at treeRevision, and returns them.
	GetMerkleNodes(ctx context.Context, treeRevision int64, ids []NodeID) ([]Node, error)
}

// NodeWriter provides a write interface into the stored tree nodes.
type NodeWriter interface {
	// SetMerkleNodes stores the provided nodes, at the transaction's writeRevision.
	SetMerkleNodes(ctx context.Context, nodes []Node) error
}
