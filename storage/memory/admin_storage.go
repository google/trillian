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

package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
)

// NewAdminStorage returns a storage.AdminStorage implementation backed by
// memoryTreeStorage.
func NewAdminStorage(ms storage.LogStorage) storage.AdminStorage {
	return &memoryAdminStorage{ms.(*memoryLogStorage).memoryTreeStorage}
}

// memoryAdminStorage implements storage.AdminStorage
type memoryAdminStorage struct {
	ms *memoryTreeStorage
}

func (s *memoryAdminStorage) Snapshot(ctx context.Context) (storage.ReadOnlyAdminTX, error) {
	return s.Begin(ctx)
}

func (s *memoryAdminStorage) Begin(ctx context.Context) (storage.AdminTX, error) {
	return &adminTX{ms: s.ms}, nil
}

func (s *memoryAdminStorage) CheckDatabaseAccessible(ctx context.Context) error {
	return nil
}

type adminTX struct {
	ms *memoryTreeStorage
	// mu guards reads/writes on closed, which happen only on
	// Commit/Rollback/IsClosed/Close methods.
	// We don't check closed on *all* methods (apart from the ones above),
	// as we trust tx to keep tabs on its state (and consequently fail to do
	// queries after closed).
	mu     sync.RWMutex
	closed bool
}

func (t *adminTX) Commit() error {
	// TODO(al): The admin implementation isn't transactional
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	return nil
}

func (t *adminTX) Rollback() error {
	// TODO(al): The admin implementation isn't transactional
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	return nil
}

func (t *adminTX) IsClosed() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.closed
}

func (t *adminTX) Close() error {
	// Acquire and release read lock manually, without defer, as if the txn
	// is not closed Rollback() will attempt to acquire the rw lock.
	t.mu.RLock()
	closed := t.closed
	t.mu.RUnlock()
	if !closed {
		err := t.Rollback()
		if err != nil {
			glog.Warningf("Rollback error on Close(): %v", err)
		}
		return err
	}
	return nil
}

func (t *adminTX) GetTree(ctx context.Context, treeID int64) (*trillian.Tree, error) {
	tree := t.ms.getTree(treeID)
	tree.RLock()
	defer tree.RUnlock()

	if tree == nil {
		return nil, fmt.Errorf("no such treeID %d", treeID)
	}
	return tree.meta, nil
}

func (t *adminTX) ListTreeIDs(ctx context.Context, includeDeleted bool) ([]int64, error) {
	t.ms.mu.RLock()
	defer t.ms.mu.RUnlock()

	var ret []int64
	for _, v := range t.ms.trees {
		ret = append(ret, v.meta.TreeId)
	}
	return ret, nil
}

func (t *adminTX) ListTrees(ctx context.Context, includeDeleted bool) ([]*trillian.Tree, error) {
	t.ms.mu.RLock()
	defer t.ms.mu.RUnlock()

	var ret []*trillian.Tree
	for _, v := range t.ms.trees {
		ret = append(ret, v.meta)
	}
	return ret, nil
}

func (t *adminTX) CreateTree(ctx context.Context, tr *trillian.Tree) (*trillian.Tree, error) {
	if err := storage.ValidateTreeForCreation(tr); err != nil {
		return nil, err
	}
	if err := validateStorageSettings(tr); err != nil {
		return nil, err
	}

	id, err := storage.NewTreeID()
	if err != nil {
		return nil, err
	}

	now := time.Now()

	meta := *tr
	meta.TreeId = id
	meta.CreateTime, err = ptypes.TimestampProto(now)
	if err != nil {
		return nil, err
	}
	meta.UpdateTime, err = ptypes.TimestampProto(now)
	if err != nil {
		return nil, err
	}

	t.ms.mu.Lock()
	defer t.ms.mu.Unlock()
	t.ms.trees[id] = newTree(meta)

	glog.Infof("trees: %v", t.ms.trees)

	return &meta, nil
}

func (t *adminTX) UpdateTree(ctx context.Context, treeID int64, updateFunc func(*trillian.Tree)) (*trillian.Tree, error) {
	mTree := t.ms.getTree(treeID)
	mTree.mu.Lock()
	defer mTree.mu.Unlock()

	tree := mTree.meta
	beforeUpdate := *tree
	updateFunc(tree)
	if err := storage.ValidateTreeForUpdate(&beforeUpdate, tree); err != nil {
		return nil, err
	}
	if err := validateStorageSettings(tree); err != nil {
		return nil, err
	}

	var err error
	tree.UpdateTime, err = ptypes.TimestampProto(time.Now())
	if err != nil {
		return nil, err
	}
	return tree, nil
}

func (t *adminTX) SoftDeleteTree(ctx context.Context, treeID int64) (*trillian.Tree, error) {
	return nil, fmt.Errorf("method not supported: SoftDeleteTree")
}

func (t *adminTX) HardDeleteTree(ctx context.Context, treeID int64) error {
	return fmt.Errorf("method not supported: HardDeleteTree")
}

func (t *adminTX) UndeleteTree(ctx context.Context, treeID int64) (*trillian.Tree, error) {
	return nil, fmt.Errorf("method not supported: UndeleteTree")
}

func validateStorageSettings(tree *trillian.Tree) error {
	if tree.StorageSettings != nil {
		return fmt.Errorf("storage_settings not supported, but got %v", tree.StorageSettings)
	}
	return nil
}
