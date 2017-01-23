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
