// Copyright 2019 Google Inc. All Rights Reserved.
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

import "github.com/google/trillian/storage"

// TODO(pavelkalinnikov): This file contains type/function aliases for backward
// compatibility purposes. It will be removed with the next major version bump.

// NewStorageProviderFunc is the signature of a function which can be
// registered to provide instances of storage providers.
//
// Deprecated: storage.NewProviderFunc should be used directly.
type NewStorageProviderFunc = storage.NewProviderFunc

// StorageProvider is an interface which allows Trillian binaries to use
// different storage implementations.
//
// Deprecated: storage.Provider should be used directly.
type StorageProvider = storage.Provider

// RegisterStorageProvider registers the provided StorageProvider.
//
// Deprecated: storage.RegisterProvider should be used directly.
var RegisterStorageProvider = storage.RegisterProvider

// NewStorageProviderFromFlags returns a new StorageProvider instance of the
// type specified by flag.
//
// Deprecated: storage.NewProviderFromFlags should be used directly.
var NewStorageProviderFromFlags = storage.NewProviderFromFlags

// NewStorageProvider returns a new StorageProvider instance of the type
// specified by name.
//
// Deprecated: storage.NewProvider should be used directly.
var NewStorageProvider = storage.NewProvider
