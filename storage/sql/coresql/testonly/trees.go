// Copyright 2017 Google Inc. All Rights Reserved.
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
	"fmt"

	"context"
	"github.com/google/trillian"
	"github.com/google/trillian/storage/sql/coresql"
	"github.com/google/trillian/storage/sql/coresql/wrapper"
	storageto "github.com/google/trillian/storage/testonly"
)

// CreateMapForTest creates a map-type tree for tests. Returns the treeID of the new tree.
func CreateMapForTest(wrapper wrapper.DBWrapper) int64 {
	tree, err := CreateTreeForTest(wrapper, storageto.MapTree)
	if err != nil {
		panic(fmt.Sprintf("Error creating map: %v", err))
	}
	return tree.TreeId
}

// CreateLogForTest creates a log-type tree for tests. Returns the treeID of the new tree.
func CreateLogForTest(wrapper wrapper.DBWrapper) int64 {
	tree, err := CreateTreeForTest(wrapper, storageto.LogTree)
	if err != nil {
		panic(fmt.Sprintf("Error creating log: %v", err))
	}
	return tree.TreeId
}

// CreateTreeForTest creates the specified tree using AdminStorage.
func CreateTreeForTest(wrapper wrapper.DBWrapper, tree *trillian.Tree) (*trillian.Tree, error) {
	s := coresql.NewAdminStorage(wrapper)
	ctx := context.Background()
	tx, err := s.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Close()
	newTree, err := tx.CreateTree(ctx, tree)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return newTree, nil
}

// UpdateTreeForTest updates the specified tree using AdminStorage.
func UpdateTreeForTest(wrapper wrapper.DBWrapper, treeID int64, updateFn func(*trillian.Tree)) (*trillian.Tree, error) {
	s := coresql.NewAdminStorage(wrapper)
	ctx := context.Background()
	tx, err := s.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Close()
	tree, err := tx.UpdateTree(ctx, treeID, updateFn)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return tree, nil
}
