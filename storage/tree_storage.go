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

	"github.com/google/trillian/storage/tree"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrTreeNeedsInit is returned when calling methods on an uninitialised tree.
var ErrTreeNeedsInit = status.Error(codes.FailedPrecondition, "tree needs initialising")

// ReadOnlyTreeTX represents a read-only transaction on a TreeStorage.
// A ReadOnlyTreeTX can only modify the tree specified in its creation.
type ReadOnlyTreeTX interface {
	NodeReader

	// ReadRevision returns the tree revision that was current at the time this
	// transaction was started.
	ReadRevision(ctx context.Context) (int64, error)

	// Commit attempts to commit any reads performed under this transaction.
	Commit(context.Context) error

	// Rollback aborts this transaction.
	Rollback() error

	// Close attempts to Rollback the TX if it's open, it's a noop otherwise.
	Close() error

	// IsOpen indicates if this transaction is open. An open transaction is one for which
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
	TreeWriter
}

// TreeWriter represents additional transaction methods that modify the tree.
type TreeWriter interface {
	// SetMerkleNodes stores the provided nodes, at the transaction's writeRevision.
	SetMerkleNodes(ctx context.Context, nodes []tree.Node) error

	// WriteRevision returns the tree revision that any writes through this TreeTX will be stored at.
	WriteRevision(ctx context.Context) (int64, error)
}

// DatabaseChecker performs connectivity checks on the database.
type DatabaseChecker interface {
	// CheckDatabaseAccessible returns nil if the database is accessible, error otherwise.
	CheckDatabaseAccessible(context.Context) error
}

// NodeReader provides read-only access to the stored tree nodes, as an interface to allow easier
// testing of node manipulation.
type NodeReader interface {
	// GetMerkleNodes looks up the set of nodes identified by ids, at
	// treeRevision, and returns them in the same order.
	GetMerkleNodes(ctx context.Context, treeRevision int64, ids []tree.NodeID) ([]tree.Node, error)
}
