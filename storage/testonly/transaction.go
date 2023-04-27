// Copyright 2018 Google LLC. All Rights Reserved.
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
	"context"
	"errors"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

// RunOnLogTX is a helper for mocking out the LogStorage.ReadWriteTransaction method.
func RunOnLogTX(tx storage.LogTreeTX) func(ctx context.Context, treeID int64, f storage.LogTXFunc) error {
	return func(ctx context.Context, _ int64, f storage.LogTXFunc) error {
		defer func() {
			if err := tx.Close(); err != nil {
				klog.Errorf("tx.Close(): %v", err)
			}
		}()
		if err := f(ctx, tx); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
}

// RunOnAdminTX is a helper for mocking out the AdminStorage.ReadWriteTransaction method.
func RunOnAdminTX(tx storage.AdminTX) func(ctx context.Context, f storage.AdminTXFunc) error {
	return func(ctx context.Context, f storage.AdminTXFunc) error {
		defer func() {
			if err := tx.Close(); err != nil {
				klog.Errorf("tx.Close(): %v", err)
			}
		}()
		if err := f(ctx, tx); err != nil {
			return err
		}
		return tx.Commit()
	}
}

// ErrNotImplemented is returned by unimplemented methods on the storage fakes.
var ErrNotImplemented = errors.New("not implemented")

// FakeLogStorage is a LogStorage implementation which is used for testing.
type FakeLogStorage struct {
	TX         storage.LogTreeTX
	ReadOnlyTX storage.ReadOnlyLogTreeTX

	TXErr                 error
	QueueLeavesErr        error
	AddSequencedLeavesErr error
}

// GetActiveLogIDs implements LogStorage.GetActiveLogIDs.
func (f *FakeLogStorage) GetActiveLogIDs(ctx context.Context) ([]int64, error) {
	return nil, ErrNotImplemented
}

// SnapshotForTree implements LogStorage.SnapshotForTree
func (f *FakeLogStorage) SnapshotForTree(ctx context.Context, _ *trillian.Tree) (storage.ReadOnlyLogTreeTX, error) {
	return f.ReadOnlyTX, f.TXErr
}

// ReadWriteTransaction implements LogStorage.ReadWriteTransaction
func (f *FakeLogStorage) ReadWriteTransaction(ctx context.Context, tree *trillian.Tree, fn storage.LogTXFunc) error {
	if f.TXErr != nil {
		return f.TXErr
	}
	return RunOnLogTX(f.TX)(ctx, tree.TreeId, fn)
}

// QueueLeaves implements LogStorage.QueueLeaves.
func (f *FakeLogStorage) QueueLeaves(ctx context.Context, tree *trillian.Tree, leaves []*trillian.LogLeaf, queueTimestamp time.Time) ([]*trillian.QueuedLogLeaf, error) {
	if f.QueueLeavesErr != nil {
		return nil, f.QueueLeavesErr
	}
	return make([]*trillian.QueuedLogLeaf, len(leaves)), nil
}

// AddSequencedLeaves implements LogStorage.AddSequencedLeaves.
func (f *FakeLogStorage) AddSequencedLeaves(ctx context.Context, tree *trillian.Tree, leaves []*trillian.LogLeaf, timestamp time.Time) ([]*trillian.QueuedLogLeaf, error) {
	if f.AddSequencedLeavesErr != nil {
		return nil, f.AddSequencedLeavesErr
	}
	res := make([]*trillian.QueuedLogLeaf, len(leaves))
	for i := range res {
		res[i] = &trillian.QueuedLogLeaf{Status: status.New(codes.OK, "OK").Proto()}
	}
	return res, nil
}

// CheckDatabaseAccessible implements LogStorage.CheckDatabaseAccessible
func (f *FakeLogStorage) CheckDatabaseAccessible(ctx context.Context) error {
	return nil
}

// FakeAdminStorage is a AdminStorage implementation which is used for testing.
type FakeAdminStorage struct {
	TX          []storage.AdminTX
	ReadOnlyTX  []storage.ReadOnlyAdminTX
	TXErr       []error
	SnapshotErr []error
}

// Begin implements AdminStorage.Begin
func (f *FakeAdminStorage) Begin(ctx context.Context) (storage.AdminTX, error) {
	return nil, ErrNotImplemented
}

// Snapshot implements AdminStorage.Snapshot
func (f *FakeAdminStorage) Snapshot(ctx context.Context) (storage.ReadOnlyAdminTX, error) {
	if len(f.SnapshotErr) > 0 {
		e := f.SnapshotErr[0]
		f.SnapshotErr = f.SnapshotErr[1:]
		return nil, e
	}
	r := f.ReadOnlyTX[0]
	f.ReadOnlyTX = f.ReadOnlyTX[1:]
	return r, nil
}

// ReadWriteTransaction implements AdminStorage.ReadWriteTransaction
func (f *FakeAdminStorage) ReadWriteTransaction(ctx context.Context, fn storage.AdminTXFunc) error {
	if len(f.TXErr) > 0 {
		e := f.TXErr[0]
		f.TXErr = f.TXErr[1:]
		return e
	}
	t := f.TX[0]
	f.TX = f.TX[1:]
	return RunOnAdminTX(t)(ctx, fn)
}

// CheckDatabaseAccessible implements AdminStorage.CheckDatabaseAccessible
func (f *FakeAdminStorage) CheckDatabaseAccessible(ctx context.Context) error {
	return nil
}
