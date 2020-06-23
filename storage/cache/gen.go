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

// Package cache provides subtree caching functionality.
package cache

//go:generate mockgen -self_package github.com/google/trillian/storage/cache -package cache -imports github.com/google/trillian/storage/storagepb -destination mock_node_storage.go github.com/google/trillian/storage/cache NodeStorage

import (
	"context"

	"github.com/google/trillian/storage/storagepb"
	"github.com/google/trillian/storage/tree"
)

// NodeStorage provides an interface for storing and retrieving subtrees.
type NodeStorage interface {
	GetSubtree(n tree.NodeID) (*storagepb.SubtreeProto, error)
	SetSubtrees(ctx context.Context, s []*storagepb.SubtreeProto) error
}
