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

package server

import (
	"github.com/google/trillian/storage"
)

type getLogStorageFunc func() (storage.LogStorage, error)
type getMapStorageFunc func() (storage.MapStorage, error)

// testRegistry implements extension.Registry.
// It delegates its implementation to its member funcs.
// It is intended as a stub for testing.
type testRegistry struct {
	logStorageFunc getLogStorageFunc
	mapStorageFunc getMapStorageFunc
}

func (r *testRegistry) GetLogStorage() (storage.LogStorage, error) {
	return r.logStorageFunc()
}

func (r *testRegistry) GetMapStorage() (storage.MapStorage, error) {
	return r.mapStorageFunc()
}
