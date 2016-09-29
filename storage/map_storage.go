package storage

import (
	"github.com/google/trillian"
)

// ReadOnlyMapTX provides a read-only view into the Map data.
type ReadOnlyMapTX interface {
	ReadOnlyTreeTX
	MapRootReader
	Getter
}

// MapTX is the transactional interface for reading/modifying a Map.
// It extends the basic TreeTX interface with Map specific methods.
// After a call to Commit or Rollback implementations must be in a clean state and have
// released any resources owned by the MapTX.
type MapTX interface {
	TreeTX
	MapRootReader
	MapRootWriter
	Getter
	Setter
}

// ReadOnlyMapStorage provides a narrow read-only view into a MapStorage.
type ReadOnlyMapStorage interface {
	// Snapshot starts a new read-only transaction.
	// Commit must be called when the caller is finished with the returned object,
	// and values read through it should only be propagated if Commit returns
	// without error.
	Snapshot() (ReadOnlyMapTX, error)

	// Returns the MapID this storage relates to.
	MapID() trillian.MapID
}

// MapStorage should be implemented by concrete storage mechanisms which want to support Maps
type MapStorage interface {
	ReadOnlyMapStorage
	// Begin starts a new Map transaction.
	// Either Commit or Rollback must be called when the caller is finished with
	// the returned object, and values read through it should only be propagated
	// if Commit returns without error.
	Begin() (MapTX, error)
}

// Setter allows the setting of key->value pairs on the map.
type Setter interface {
	// Set sets key to leaf
	Set(keyHash trillian.Hash, value trillian.MapLeaf) error
}

// Getter allows access to the values stored in the map.
type Getter interface {
	// Get retrieves the values associates with the keyHashes, if any, at the
	// specified revision.
	// Setting revision to -1 will fetch the latest revision.
	// The returned array of MapLeaves will only contain entries for which values
	// exist.  i.e. requesting a set of unknown keys would result in a
	// zero-length array being returned.
	Get(revision int64, keyHash []trillian.Hash) ([]trillian.MapLeaf, error)
}

// MapRootReader provides access to the map roots.
type MapRootReader interface {
	// LatestSignedMapRoot returns the most recently created SignedMapRoot.
	LatestSignedMapRoot() (trillian.SignedMapRoot, error)
}

// MapRootWriter allows the storage of new SignedMapRoots
type MapRootWriter interface {
	// StoreSignedMapRoot stores root.
	StoreSignedMapRoot(root trillian.SignedMapRoot) error
}
