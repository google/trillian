// Copyright 2019 Google LLC. All Rights Reserved.
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

import "github.com/google/trillian/storage/tree"

// TODO(pavelkalinnikov, v2): These aliases were created to not break the code
// that depended on these types. We should delete this.

// NodeID is an alias to github.com/google/trillian/storage/tree.NodeID.
type NodeID = tree.NodeID

// Suffix is an alias to github.com/google/trillian/storage/tree.Suffix.
type Suffix = tree.Suffix

// These are aliases for the functions of the same name in github.com/google/trillian/storage/tree.
var (
	NewNodeIDFromHash         = tree.NewNodeIDFromHash
	NewNodeIDFromPrefix       = tree.NewNodeIDFromPrefix
	NewNodeIDForTreeCoords    = tree.NewNodeIDForTreeCoords
	NewNodeIDFromPrefixSuffix = tree.NewNodeIDFromPrefixSuffix

	EmptySuffix = tree.EmptySuffix
	ParseSuffix = tree.ParseSuffix
)
