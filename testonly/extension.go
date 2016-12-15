package testonly

import (
	"fmt"

	"github.com/google/trillian/extension"
	"github.com/google/trillian/storage"
)

type GetLogStorageFunc func(int64) (storage.LogStorage, error)
type GetMapStorageFunc func(int64) (storage.MapStorage, error)

type testRegistry struct {
	getLogStorageFunc GetLogStorageFunc
	getMapStorageFunc GetMapStorageFunc
}

func defaultGetLogStorage(int64) (storage.LogStorage, error) {
	return nil, fmt.Errorf("Not implemented")
}

func defaultGetMapStorage(int64) (storage.MapStorage, error) {
	return nil, fmt.Errorf("Not implemented")
}

func (r testRegistry) GetLogStorage(treeID int64) (storage.LogStorage, error) {
	return r.getLogStorageFunc(treeID)
}

func (r testRegistry) GetMapStorage(treeID int64) (storage.MapStorage, error) {
	return r.getMapStorageFunc(treeID)
}

func NewRegistryForLogTests(ls storage.LogStorage) extension.ExtensionRegistry {
	getLogStorageFunc := func(int64) (storage.LogStorage, error) {
		return ls, nil
	}
	return testRegistry{getLogStorageFunc: getLogStorageFunc, getMapStorageFunc: defaultGetMapStorage}
}

func NewRegistryWithLogStorage(ls storage.LogStorage) extension.ExtensionRegistry {
	getLogStorage := func(int64) (storage.LogStorage, error) {
		return ls, nil
	}
	return testRegistry{getLogStorageFunc: getLogStorage, getMapStorageFunc: defaultGetMapStorage}
}

func NewRegistryWithLogProvider(f GetLogStorageFunc) extension.ExtensionRegistry {
	return testRegistry{getLogStorageFunc: f, getMapStorageFunc: defaultGetMapStorage}
}
