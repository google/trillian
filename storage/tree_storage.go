// Copyright 2016 Google LLC. All Rights Reserved.
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

	"github.com/google/trillian/merkle/compact"
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

	// Commit attempts to commit any reads performed under this transaction.
	Commit(context.Context) error

	// Close attempts to rollback the TX if it's open, it's a noop otherwise.
	Close() error

	// IsOpen indicates if this transaction is open. An open transaction is one for which
	// Commit() has never been called. Implementations must do all clean up in these methods
	// so transactions are assumed closed regardless of the reported success.
	IsOpen() bool
}

// TreeTX represents an in-process tree-modifying transaction.
// The transaction must end with a call to Commit or Close.
// After a call to Commit or Close, all operations on the transaction will fail.
// After a call to Commit or Close implementations must be in a clean state and have
// released any resources owned by the TreeTX.
// A TreeTX can only modify the tree specified in its creation.
type TreeTX interface {
	ReadOnlyTreeTX
	TreeWriter
}

// TreeWriter represents additional transaction methods that modify the tree.
type TreeWriter interface {
	// SetMerkleNodes writes the nodes, at the write revision.
	//
	// TODO(pavelkalinnikov): Use tiles instead, here and in GetMerkleNodes.
	SetMerkleNodes(ctx context.Context, nodes []tree.Node) error
}

// DatabaseChecker performs connectivity checks on the database.
type DatabaseChecker interface {
	// CheckDatabaseAccessible returns nil if the database is accessible, error otherwise.
	CheckDatabaseAccessible(context.Context) error
}

// NodeReader provides read-only access to the stored tree nodes, as an
// interface to allow easier testing of node manipulation.
type NodeReader interface {
	// GetMerkleNodes returns tree nodes by their IDs, in the requested order.
	GetMerkleNodes(ctx context.Context, ids []compact.NodeID) ([]tree.Node, error)
}
