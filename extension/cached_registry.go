package extension

import (
	"sync"

	"github.com/google/trillian/storage"
)

// cachedRegistry delegates method calls to registry, but caches the results for future invocations.
type cachedRegistry struct {
	registry Registry

	mu   sync.Mutex
	logs map[int64]storage.LogStorage
	maps map[int64]storage.MapStorage
}

func (r *cachedRegistry) GetLogStorage(treeID int64) (storage.LogStorage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	storage, ok := r.logs[treeID]
	if !ok {
		var err error
		storage, err = r.registry.GetLogStorage(treeID)
		if err != nil {
			return nil, err
		}
		r.logs[treeID] = storage
	}
	return storage, nil
}

func (r *cachedRegistry) GetMapStorage(treeID int64) (storage.MapStorage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	storage, ok := r.maps[treeID]
	if !ok {
		var err error
		storage, err = r.registry.GetMapStorage(treeID)
		if err != nil {
			return nil, err
		}
		r.maps[treeID] = storage
	}
	return storage, nil
}

// NewCachedRegistry wraps a registry into a cached implementation, which caches storages per tree
// ID.
func NewCachedRegistry(registry Registry) Registry {
	return &cachedRegistry{
		registry: registry,
		logs:     make(map[int64]storage.LogStorage),
		maps:     make(map[int64]storage.MapStorage),
	}
}
