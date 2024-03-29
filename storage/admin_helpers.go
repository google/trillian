// Copyright 2017 Google LLC. All Rights Reserved.
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
	"fmt"

	"github.com/google/trillian"
	"github.com/google/trillian/monitoring"
	"k8s.io/klog/v2"
)

const traceSpanRoot = "/trillian/storage"

// GetTree reads a tree from storage using a snapshot transaction.
// It's a convenience wrapper around RunInAdminSnapshot and ReadOnlyAdminTX's GetTree.
// See RunInAdminSnapshot if you need to perform more than one action per transaction.
func GetTree(ctx context.Context, admin AdminStorage, treeID int64) (*trillian.Tree, error) {
	ctx, spanEnd := spanFor(ctx, "GetTree")
	defer spanEnd()
	var tree *trillian.Tree
	err := RunInAdminSnapshot(ctx, admin, func(tx ReadOnlyAdminTX) error {
		var err error
		tree, err = tx.GetTree(ctx, treeID)
		return err
	})
	return tree, err
}

// ListTrees reads trees from storage using a snapshot transaction.
// It's a convenience wrapper around RunInAdminSnapshot and ReadOnlyAdminTX's ListTrees.
// See RunInAdminSnapshot if you need to perform more than one action per transaction.
func ListTrees(ctx context.Context, admin AdminStorage, includeDeleted bool) ([]*trillian.Tree, error) {
	ctx, spanEnd := spanFor(ctx, "ListTrees")
	defer spanEnd()
	var resp []*trillian.Tree
	err := RunInAdminSnapshot(ctx, admin, func(tx ReadOnlyAdminTX) error {
		var err error
		resp, err = tx.ListTrees(ctx, includeDeleted)
		return err
	})
	return resp, err
}

// CreateTree creates a tree in storage.
// It's a convenience wrapper around ReadWriteTransaction and AdminWriter's CreateTree.
// See ReadWriteTransaction if you need to perform more than one action per transaction.
func CreateTree(ctx context.Context, admin AdminStorage, tree *trillian.Tree) (*trillian.Tree, error) {
	ctx, spanEnd := spanFor(ctx, "CreateTree")
	defer spanEnd()
	var createdTree *trillian.Tree
	err := admin.ReadWriteTransaction(ctx, func(ctx context.Context, tx AdminTX) error {
		var err error
		createdTree, err = tx.CreateTree(ctx, tree)
		return err
	})
	return createdTree, err
}

// UpdateTree updates a tree in storage.
// It's a convenience wrapper around ReadWriteTransaction and AdminWriter's UpdateTree.
// See ReadWriteTransaction if you need to perform more than one action per transaction.
func UpdateTree(ctx context.Context, admin AdminStorage, treeID int64, fn func(*trillian.Tree)) (*trillian.Tree, error) {
	ctx, spanEnd := spanFor(ctx, "UpdateTree")
	defer spanEnd()
	var updatedTree *trillian.Tree
	err := admin.ReadWriteTransaction(ctx, func(ctx context.Context, tx AdminTX) error {
		var err error
		updatedTree, err = tx.UpdateTree(ctx, treeID, fn)
		return err
	})
	return updatedTree, err
}

// SoftDeleteTree soft-deletes a tree in storage.
// It's a convenience wrapper around ReadWriteTransaction and AdminWriter's SoftDeleteTree.
// See ReadWriteTransaction if you need to perform more than one action per transaction.
func SoftDeleteTree(ctx context.Context, admin AdminStorage, treeID int64) (*trillian.Tree, error) {
	ctx, spanEnd := spanFor(ctx, "SoftDeleteTree")
	defer spanEnd()
	var tree *trillian.Tree
	err := admin.ReadWriteTransaction(ctx, func(ctx context.Context, tx AdminTX) error {
		var err error
		tree, err = tx.SoftDeleteTree(ctx, treeID)
		return err
	})
	return tree, err
}

// HardDeleteTree hard-deletes a tree from storage.
// It's a convenience wrapper around ReadWriteTransaction and AdminWriter's HardDeleteTree.
// See ReadWriteTransaction if you need to perform more than one action per transaction.
func HardDeleteTree(ctx context.Context, admin AdminStorage, treeID int64) error {
	ctx, spanEnd := spanFor(ctx, "HardDeleteTree")
	defer spanEnd()
	return admin.ReadWriteTransaction(ctx, func(ctx context.Context, tx AdminTX) error {
		return tx.HardDeleteTree(ctx, treeID)
	})
}

// UndeleteTree undeletes a tree in storage.
// It's a convenience wrapper around ReadWriteTransaction and AdminWriter's UndeleteTree.
// See ReadWriteTransaction if you need to perform more than one action per transaction.
func UndeleteTree(ctx context.Context, admin AdminStorage, treeID int64) (*trillian.Tree, error) {
	ctx, spanEnd := spanFor(ctx, "UndeleteTree")
	defer spanEnd()
	var tree *trillian.Tree
	err := admin.ReadWriteTransaction(ctx, func(ctx context.Context, tx AdminTX) error {
		var err error
		tree, err = tx.UndeleteTree(ctx, treeID)
		return err
	})
	return tree, err
}

// RunInAdminSnapshot runs fn against a ReadOnlyAdminTX and commits if no error is returned.
func RunInAdminSnapshot(ctx context.Context, admin AdminStorage, fn func(tx ReadOnlyAdminTX) error) error {
	tx, err := admin.Snapshot(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err := tx.Close(); err != nil {
			klog.Errorf("tx.Close(): %v", err)
		}
	}()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func spanFor(ctx context.Context, name string) (context.Context, func()) {
	return monitoring.StartSpan(ctx, fmt.Sprintf("%s.%s", traceSpanRoot, name))
}
