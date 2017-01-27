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
type ReadOnlyTreeTX interface {
	NodeReader
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// TreeTX represents an in-process tree-modifying transaction.
// The transaction must end with a call to Commit or Rollback.
// After a call to Commit or Rollback, all operations on the transaction will fail.
// After a call to Commit or Rollback implementations must be in a clean state and have
// released any resources owned by the TreeTX.
type TreeTX interface {
	NodeReaderWriter

	// Commit applies the operations performed to the underlying storage, or returns an error.
	Commit(ctx context.Context) error

	// Rollback aborts any performed operations. No updates must be applied to the underlying storage.
	Rollback(ctx context.Context) error

	// Open indicates if this transaction is open. An open transaction is one for which
	// Commit() or Rollback() has never been called. Implementations must do all clean up
	// in these methods so transactions are assumed closed regardless of the reported success.
	IsOpen() bool

	// WriteRevision returns the tree revision that any writes through this TreeTX will be stored at.
	WriteRevision() int64
}

// NodeReader provides a read-only interface into the stored tree nodes.
type NodeReader interface {
	// GetTreeRevisionIncludingSize returns the revision and actual size for a tree at a requested
	// size.
	//
	// It is an error to request tree sizes larger than the currently published tree size.
	// This may return a revision for any tree size at least as large as that requested. The
	// size of the tree is returned along with the corresponding revision. The caller should
	// be aware that this may differ from the requested size.
	GetTreeRevisionIncludingSize(ctx context.Context, treeSize int64) (revision, size int64, err error)
	// GetMerkleNodes looks up the set of nodes identified by ids, at treeRevision, and returns them.
	GetMerkleNodes(ctx context.Context, treeRevision int64, ids []NodeID) ([]Node, error)
}

// NodeReaderWriter provides a read-write interface into the stored tree nodes.
type NodeReaderWriter interface {
	NodeReader

	// SetMerkleNodes stores the provided nodes, at the transaction's writeRevision.
	SetMerkleNodes(ctx context.Context, nodes []Node) error
}
