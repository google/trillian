// Copyright 2016 Google LLC. All Rights Reserved.
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

// Package storage provides general interfaces to Trillian storage layers.
package storage

//go:generate mockgen -self_package github.com/google/trillian/storage -package storage -destination mock_storage.go -imports=trillian=github.com/google/trillian,storagepb=github.com/google/trillian/storage/storagepb github.com/google/trillian/storage AdminStorage,AdminTX,LogStorage,LogTreeTX,MapStorage,MapTreeTX,ReadOnlyAdminTX,ReadOnlyLogTX,ReadOnlyLogTreeTX,ReadOnlyMapTreeTX
