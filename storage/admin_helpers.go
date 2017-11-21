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

package storage

import (
	"context"

	"github.com/google/trillian"
)

// GetTree reads a tree from storage using a snapshot transaction.
// It's a convenience wrapper around RunInAdminSnapshot and AdminReader's GetTree.
// See RunInAdminSnapshot if you need to perform more than one action per transaction.
func GetTree(ctx context.Context, admin AdminStorage, treeID int64) (*trillian.Tree, error) {
	var tree *trillian.Tree
	err := RunInAdminSnapshot(ctx, admin, func(tx ReadOnlyAdminTX) (err error) {
		tree, err = tx.GetTree(ctx, treeID)
		return
	})
	return tree, err
}

// ListTrees reads trees from storage using a snapshot transaction.
// It's a convenience wrapper around RunInAdminSnapshot and AdminReader's ListTrees.
// See RunInAdminSnapshot if you need to perform more than one action per transaction.
func ListTrees(ctx context.Context, admin AdminStorage, includeDeleted bool) ([]*trillian.Tree, error) {
	var resp []*trillian.Tree
	err := RunInAdminSnapshot(ctx, admin, func(tx ReadOnlyAdminTX) (err error) {
		resp, err = tx.ListTrees(ctx, includeDeleted)
		return
	})
	return resp, err
}

// CreateTree creates a tree in storage.
// It's a convenience wrapper around RunInAdminTX and AdminWriter's CreateTree.
// See RunInAdminTX if you need to perform more than one action per transaction.
func CreateTree(ctx context.Context, admin AdminStorage, tree *trillian.Tree) (*trillian.Tree, error) {
	var createdTree *trillian.Tree
	err := RunInAdminTX(ctx, admin, func(tx AdminTX) (err error) {
		createdTree, err = tx.CreateTree(ctx, tree)
		return
	})
	return createdTree, err
}

// UpdateTree updates a tree in storage.
// It's a convenience wrapper around RunInAdminTX and AdminWriter's UpdateTree.
// See RunInAdminTX if you need to perform more than one action per transaction.
func UpdateTree(ctx context.Context, admin AdminStorage, treeID int64, fn func(*trillian.Tree)) (*trillian.Tree, error) {
	var updatedTree *trillian.Tree
	err := RunInAdminTX(ctx, admin, func(tx AdminTX) (err error) {
		updatedTree, err = tx.UpdateTree(ctx, treeID, fn)
		return
	})
	return updatedTree, err
}

// SoftDeleteTree soft-deletes a tree in storage.
// It's a convenience wrapper around RunInAdminTX and AdminWriter's SoftDeleteTree.
// See RunInAdminTX if you need to perform more than one action per transaction.
func SoftDeleteTree(ctx context.Context, admin AdminStorage, treeID int64) (*trillian.Tree, error) {
	var tree *trillian.Tree
	err := RunInAdminTX(ctx, admin, func(tx AdminTX) (err error) {
		tree, err = tx.SoftDeleteTree(ctx, treeID)
		return
	})
	return tree, err
}

// HardDeleteTree hard-deletes a tree from storage.
// It's a convenience wrapper around RunInAdminTX and AdminWriter's HardDeleteTree.
// See RunInAdminTX if you need to perform more than one action per transaction.
func HardDeleteTree(ctx context.Context, admin AdminStorage, treeID int64) error {
	return RunInAdminTX(ctx, admin, func(tx AdminTX) error {
		return tx.HardDeleteTree(ctx, treeID)
	})
}

// UndeleteTree undeletes a tree in storage.
// It's a convenience wrapper around RunInAdminTX and AdminWriter's UndeleteTree.
// See RunInAdminTX if you need to perform more than one action per transaction.
func UndeleteTree(ctx context.Context, admin AdminStorage, treeID int64) (*trillian.Tree, error) {
	var tree *trillian.Tree
	err := RunInAdminTX(ctx, admin, func(tx AdminTX) (err error) {
		tree, err = tx.UndeleteTree(ctx, treeID)
		return
	})
	return tree, err
}
