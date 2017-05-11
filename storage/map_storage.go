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

	"github.com/google/trillian"
)

// ReadOnlyMapTX provides a read-only view into log data.
// A ReadOnlyMapTX, unlike ReadOnlyMapTreeTX, is not tied to a particular tree.
type ReadOnlyMapTX interface {
	// Commit ensures the data read by the TX is consistent in the database. Only after Commit the
	// data read should be regarded as valid.
	Commit() error

	// Rollback discards the read-only TX.
	Rollback() error

	// Close attempts to Rollback the TX if it's open, it's a noop otherwise.
	Close() error
}

// ReadOnlyMapTreeTX provides a read-only view into the Map data.
// A ReadOnlyMapTreeTX can only read from the tree specified in its creation.
type ReadOnlyMapTreeTX interface {
	ReadOnlyTreeTX
	MapRootReader
	Getter
}

// MapTreeTX is the transactional interface for reading/modifying a Map.
// It extends the basic TreeTX interface with Map specific methods.
// After a call to Commit or Rollback implementations must be in a clean state and have
// released any resources owned by the MapTX.
// A MapTreeTX can only read from the tree specified in its creation.
type MapTreeTX interface {
	TreeTX
	MapRootReader
	MapRootWriter
	Getter
	Setter
}

// ReadOnlyMapStorage provides a narrow read-only view into a MapStorage.
type ReadOnlyMapStorage interface {
	DatabaseChecker

	// Snapshot starts a read-only transaction not tied to any particular tree.
	Snapshot(ctx context.Context) (ReadOnlyMapTX, error)

	// SnapshotForTree starts a new read-only transaction.
	// Commit must be called when the caller is finished with the returned object,
	// and values read through it should only be propagated if Commit returns
	// without error.
	SnapshotForTree(ctx context.Context, treeID int64) (ReadOnlyMapTreeTX, error)
}

// MapStorage should be implemented by concrete storage mechanisms which want to support Maps
type MapStorage interface {
	ReadOnlyMapStorage
	// BeginForTree starts a new Map transaction.
	// Either Commit or Rollback must be called when the caller is finished with
	// the returned object, and values read through it should only be propagated
	// if Commit returns without error.
	BeginForTree(ctx context.Context, treeID int64) (MapTreeTX, error)
}

// Setter allows the setting of key->value pairs on the map.
type Setter interface {
	// Set sets key to leaf
	Set(ctx context.Context, keyHash []byte, value trillian.MapLeaf) error
}

// Getter allows access to the values stored in the map.
type Getter interface {
	// Get retrieves the values associates with the keyHashes, if any, at the
	// specified revision.
	// Setting revision to -1 will fetch the latest revision.
	// The returned array of MapLeaves will only contain entries for which values
	// exist.  i.e. requesting a set of unknown keys would result in a
	// zero-length array being returned.
	Get(ctx context.Context, revision int64, keyHashes [][]byte) ([]trillian.MapLeaf, error)
}

// MapRootReader provides access to the map roots.
type MapRootReader interface {
	// GetSignedMapRoot returns the SignedMapRoot associated with the
	// specified revision.
	GetSignedMapRoot(ctx context.Context, revision int64) (trillian.SignedMapRoot, error)
	// LatestSignedMapRoot returns the most recently created SignedMapRoot.
	LatestSignedMapRoot(ctx context.Context) (trillian.SignedMapRoot, error)
}

// MapRootWriter allows the storage of new SignedMapRoots
type MapRootWriter interface {
	// StoreSignedMapRoot stores root.
	StoreSignedMapRoot(ctx context.Context, root trillian.SignedMapRoot) error
}
