package testonly

import (
	"errors"

	"github.com/google/trillian/extension"
	"github.com/google/trillian/storage"
)

var errNotImplemented = errors.New("not implemented")

// GetLogStorageFunc returns a storage.LogStorage or fails.
// Used as an implementation of extension.Registry.GetLogStorage in tests.
type GetLogStorageFunc func(int64) (storage.LogStorage, error)

// GetMapStorageFunc returns a storage.MapStorage or fails.
// Used as an implementation of extension.Registry.GetMapStorage in tests.
type GetMapStorageFunc func(int64) (storage.MapStorage, error)

type testRegistry struct {
	getLogStorageFunc GetLogStorageFunc
	getMapStorageFunc GetMapStorageFunc
}

func defaultGetLogStorage(int64) (storage.LogStorage, error) {
	return nil, errNotImplemented
}

func defaultGetMapStorage(int64) (storage.MapStorage, error) {
	return nil, errNotImplemented
}

func (r testRegistry) GetLogStorage(treeID int64) (storage.LogStorage, error) {
	return r.getLogStorageFunc(treeID)
}

func (r testRegistry) GetMapStorage(treeID int64) (storage.MapStorage, error) {
	return r.getMapStorageFunc(treeID)
}

// NewRegistryWithLogStorage returns an extension.Registry backed by ls.
func NewRegistryWithLogStorage(ls storage.LogStorage) extension.Registry {
	return NewRegistryWithLogProvider(func(int64) (storage.LogStorage, error) { return ls, nil })
}

// NewRegistryWithLogProvider returns an extension.Registry whose GetLogStorage function is
// backed by f.
func NewRegistryWithLogProvider(f GetLogStorageFunc) extension.Registry {
	return testRegistry{getLogStorageFunc: f, getMapStorageFunc: defaultGetMapStorage}
}
