// Copyright 2018 Google LLC. All Rights Reserved.
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
	"fmt"
	"sync"

	"github.com/google/trillian/monitoring"
)

// NewProviderFunc is the signature of a function which can be registered to
// provide instances of storage providers.
type NewProviderFunc func(monitoring.MetricFactory) (Provider, error)

var (
	spMu     sync.RWMutex
	spByName = make(map[string]NewProviderFunc)
)

// RegisterProvider registers the given storage Provider.
func RegisterProvider(name string, sp NewProviderFunc) error {
	spMu.Lock()
	defer spMu.Unlock()

	_, exists := spByName[name]
	if exists {
		return fmt.Errorf("storage provider %v already registered", name)
	}
	spByName[name] = sp
	return nil
}

// NewProvider returns a new Provider instance of the type specified by name.
func NewProvider(name string, mf monitoring.MetricFactory) (Provider, error) {
	spMu.RLock()
	defer spMu.RUnlock()

	sp := spByName[name]
	if sp == nil {
		return nil, fmt.Errorf("no such storage provider %v", name)
	}

	return sp(mf)
}

// Providers returns a slice of all registered storage provider names.
func Providers() []string {
	spMu.RLock()
	defer spMu.RUnlock()

	r := []string{}
	for k := range spByName {
		r = append(r, k)
	}

	return r
}

// Provider is an interface which allows Trillian binaries to use different
// storage implementations.
type Provider interface {
	// LogStorage creates and returns a LogStorage implementation.
	LogStorage() LogStorage
	// MapStorage creates and returns a MapStorage implementation.
	MapStorage() MapStorage
	// AdminStorage creates and returns a AdminStorage implementation.
	AdminStorage() AdminStorage

	// Close closes the underlying storage.
	Close() error
}
