package extension

import (
	"sync"

	"github.com/google/trillian/storage"
)

type cachedRegistry struct {
	registry Registry

	mu              sync.Mutex
	logStorageCache map[int64]storage.LogStorage
	mapStorageCache map[int64]storage.MapStorage
}

func (r *cachedRegistry) GetLogStorage(treeID int64) (storage.LogStorage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	storage, ok := r.logStorageCache[treeID]
	if !ok {
		var err error
		storage, err = r.registry.GetLogStorage(treeID)
		if err != nil {
			return nil, err
		}
		r.logStorageCache[treeID] = storage
	}
	return storage, nil
}

func (r *cachedRegistry) GetMapStorage(treeID int64) (storage.MapStorage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	storage, ok := r.mapStorageCache[treeID]
	if !ok {
		var err error
		storage, err = r.registry.GetMapStorage(treeID)
		if err != nil {
			return nil, err
		}
		r.mapStorageCache[treeID] = storage
	}
	return storage, nil
}

// NewCachedRegistry wraps a registry into a cached implementation, which caches storages per tree
// ID.
func NewCachedRegistry(registry Registry) Registry {
	return &cachedRegistry{
		registry:        registry,
		logStorageCache: make(map[int64]storage.LogStorage),
		mapStorageCache: make(map[int64]storage.MapStorage),
	}
}
