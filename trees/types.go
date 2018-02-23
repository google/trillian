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

package trees

import "github.com/google/trillian"

// AccessType indicates how a tree is to be used and participates in permissions
// decisions.
type AccessType int

const (
	// Unknown is an access type that will always get rejected
	Unknown AccessType = iota
	// Admin access is for general administration purposes
	Admin
	// Query implies access to serve query, typically readonly
	Query
	// Queue is log specific - adding entries to the queue
	Queue
	// Sequence is log specific - integrating entries into the tree
	Sequence
	// Update is map specific - set / update leaves
	Update
)

// GetOpts contains validation options for GetTree.
type GetOpts struct {
	// TreeTypes is a set of allowed tree types. If empty, any type is allowed.
	TreeTypes map[trillian.TreeType]bool
	// Readonly is whether the tree will be used for read-only purposes.
	Readonly bool
	// Accessor indicates what operation is being performed
	Accessor AccessType
}

// NewGetOpts creates GetOps that allows the listed set of tree types, and
// optionally forces the tree to be readonly.
func NewGetOpts(accessor AccessType, readonly bool, types ...trillian.TreeType) GetOpts {
	m := make(map[trillian.TreeType]bool)
	for _, t := range types {
		m[t] = true
	}
	return GetOpts{Accessor: accessor, TreeTypes: m, Readonly: readonly}
}
