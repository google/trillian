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

package storage

import (
	"fmt"

	"github.com/google/trillian/node"
	"github.com/google/trillian/storage/storagepb"
)

// Integer types to distinguish storage errors that might need to be mapped at a higher level.
const (
	DuplicateLeaf = iota
)

// Error is a typed error that the storage layer can return to give callers information
// about the error to decide how to handle it.
type Error struct {
	ErrType int
	Detail  string
	Cause   error
}

// Error formats the internal details of an Error including the original cause.
func (s Error) Error() string {
	return fmt.Sprintf("Storage: %d: %s: %v", s.ErrType, s.Detail, s.Cause)
}

// Node represents a single node in a Merkle tree.
type Node struct {
	NodeID       *node.Node
	Hash         []byte
	NodeRevision int64
}

// PopulateSubtreeFunc is a function which knows how to re-populate a subtree
// from just its leaf nodes.
type PopulateSubtreeFunc func(*storagepb.SubtreeProto) error

// PrepareSubtreeWriteFunc is a function that carries out any required tree type specific
// manipulation of a subtree before it's written to storage
type PrepareSubtreeWriteFunc func(*storagepb.SubtreeProto) error
