// Copyright 2018 Google Inc. All Rights Reserved.
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

package server

import (
	"flag"
	"fmt"
	"sync"

	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/storage"
)

// NewStorageProviderFunc is the signature of a function which can be registered
// to provide instances of storage providers.
type NewStorageProviderFunc func(monitoring.MetricFactory) (StorageProvider, error)

var (
	storageSystem = flag.String("storage_system", "mysql", fmt.Sprintf("Storage system to use. One of: %v", storageProviders()))

	spMu     sync.RWMutex
	spOnce   sync.Once
	spByName map[string]NewStorageProviderFunc
)

// RegisterStorageProvider registers the provided StorageProvider.
func RegisterStorageProvider(name string, sp NewStorageProviderFunc) error {
	spMu.Lock()
	defer spMu.Unlock()

	spOnce.Do(func() {
		spByName = make(map[string]NewStorageProviderFunc)
	})

	_, exists := spByName[name]
	if exists {
		return fmt.Errorf("storage provider %v already registered", name)
	}
	spByName[name] = sp
	return nil
}

// NewStorageProviderFromFlags returns a new StorageProvider instance of the type
// specified by flag.
func NewStorageProviderFromFlags(mf monitoring.MetricFactory) (StorageProvider, error) {
	return NewStorageProvider(*storageSystem, mf)
}

// NewStorageProvider returns a new StorageProvider instance of the type
// specified by name.
func NewStorageProvider(name string, mf monitoring.MetricFactory) (StorageProvider, error) {
	spMu.RLock()
	defer spMu.RUnlock()

	sp := spByName[name]
	if sp == nil {
		return nil, fmt.Errorf("no such storage provider %v", name)
	}

	return sp(mf)
}

// storageProviders returns a slice of all registered storage provider names.
func storageProviders() []string {
	spMu.RLock()
	defer spMu.RUnlock()

	r := []string{}
	for k := range spByName {
		r = append(r, k)
	}

	return r
}

// StorageProvider is an interface which allows trillian binaries to use
// different storage implementations.
type StorageProvider interface {
	// LogStorage creates and returns a LogStorage implementation.
	LogStorage() storage.LogStorage
	// MapStorage creates and returns a MapStorage implementation.
	MapStorage() storage.MapStorage
	// AdminStorage creates and returns a AdminStorage implementation.
	AdminStorage() storage.AdminStorage

	// Close closes the underlying storage.
	Close() error
}
