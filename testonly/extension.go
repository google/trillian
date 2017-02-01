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

package testonly

import (
	"errors"

	"github.com/google/trillian/extension"
	"github.com/google/trillian/storage"
)

var errNotImplemented = errors.New("not implemented")

// GetLogStorageFunc returns a storage.LogStorage or fails.
// Used as an implementation of extension.Registry.GetLogStorage in tests.
type GetLogStorageFunc func() (storage.LogStorage, error)

// GetMapStorageFunc returns a storage.MapStorage or fails.
// Used as an implementation of extension.Registry.GetMapStorage in tests.
type GetMapStorageFunc func() (storage.MapStorage, error)

type testRegistry struct {
	getLogStorageFunc GetLogStorageFunc
	getMapStorageFunc GetMapStorageFunc
}

func defaultGetLogStorage() (storage.LogStorage, error) {
	return nil, errNotImplemented
}

func defaultGetMapStorage() (storage.MapStorage, error) {
	return nil, errNotImplemented
}

func (r testRegistry) GetLogStorage() (storage.LogStorage, error) {
	return r.getLogStorageFunc()
}

func (r testRegistry) GetMapStorage() (storage.MapStorage, error) {
	return r.getMapStorageFunc()
}

// NewRegistryWithLogStorage returns an extension.Registry backed by ls.
func NewRegistryWithLogStorage(ls storage.LogStorage) extension.Registry {
	return NewRegistryWithLogProvider(func() (storage.LogStorage, error) { return ls, nil })
}

// NewRegistryWithLogProvider returns an extension.Registry whose GetLogStorage function is
// backed by f.
func NewRegistryWithLogProvider(f GetLogStorageFunc) extension.Registry {
	return testRegistry{getLogStorageFunc: f, getMapStorageFunc: defaultGetMapStorage}
}
