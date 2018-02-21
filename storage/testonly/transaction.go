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

package testonly

import (
	"context"
	"errors"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RunOnLogTX is a helper for mocking out the LogStorage.ReadWriteTransaction method.
func RunOnLogTX(tx storage.LogTreeTX) func(ctx context.Context, treeID int64, f storage.LogTXFunc) error {
	return func(ctx context.Context, _ int64, f storage.LogTXFunc) error {
		defer tx.Close()
		if err := f(ctx, tx); err != nil {
			return err
		}
		return tx.Commit()
	}
}

// RunOnMapTX is a helper for mocking out the MapStorage.ReadWriteTransaction method.
func RunOnMapTX(tx storage.MapTreeTX) func(ctx context.Context, treeID int64, f storage.MapTXFunc) error {
	return func(ctx context.Context, _ int64, f storage.MapTXFunc) error {
		defer tx.Close()
		if err := f(ctx, tx); err != nil {
			return err
		}
		return tx.Commit()
	}
}

// RunOnAdminTX is a helper for mocking out the AdminStorage.ReadWriteTransaction method.
func RunOnAdminTX(tx storage.AdminTX) func(ctx context.Context, f storage.AdminTXFunc) error {
	return func(ctx context.Context, f storage.AdminTXFunc) error {
		defer tx.Close()
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

// Snapshot implements LogStorage.Snapshot
func (f *FakeLogStorage) Snapshot(ctx context.Context) (storage.ReadOnlyLogTX, error) {
	return nil, ErrNotImplemented
}

// BeginForTree implements LogStorage.BeginForTree
func (f *FakeLogStorage) BeginForTree(ctx context.Context, id int64) (storage.LogTreeTX, error) {
	return nil, ErrNotImplemented
}

// SnapshotForTree implements LogStorage.SnapshotForTree
func (f *FakeLogStorage) SnapshotForTree(ctx context.Context, id int64) (storage.ReadOnlyLogTreeTX, error) {
	return f.ReadOnlyTX, f.TXErr
}

// ReadWriteTransaction implements LogStorage.ReadWriteTransaction
func (f *FakeLogStorage) ReadWriteTransaction(ctx context.Context, id int64, fn storage.LogTXFunc) error {
	if f.TXErr != nil {
		return f.TXErr
	}
	return RunOnLogTX(f.TX)(ctx, id, fn)
}

// QueueLeaves implements LogStorage.QueueLeaves.
func (f *FakeLogStorage) QueueLeaves(ctx context.Context, logID int64, leaves []*trillian.LogLeaf, queueTimestamp time.Time) ([]*trillian.QueuedLogLeaf, error) {
	if f.QueueLeavesErr != nil {
		return nil, f.QueueLeavesErr
	}
	return make([]*trillian.QueuedLogLeaf, len(leaves)), nil
}

// AddSequencedLeaves implements LogStorage.AddSequencedLeaves.
func (f *FakeLogStorage) AddSequencedLeaves(ctx context.Context, logID int64, leaves []*trillian.LogLeaf) ([]*trillian.QueuedLogLeaf, error) {
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

// FakeMapStorage is a MapStorage implementation which is used for testing.
type FakeMapStorage struct {
	TX          storage.MapTreeTX
	ReadOnlyTX  storage.ReadOnlyMapTreeTX
	SnapshotErr error
}

// Snapshot implements MapStorage.Snapshot
func (f *FakeMapStorage) Snapshot(ctx context.Context) (storage.ReadOnlyMapTX, error) {
	return nil, ErrNotImplemented
}

// BeginForTree implements MapStorage.BeginForTree
func (f *FakeMapStorage) BeginForTree(ctx context.Context, id int64) (storage.MapTreeTX, error) {
	return nil, ErrNotImplemented
}

// SnapshotForTree implements MapStorage.SnapshotForTree
func (f *FakeMapStorage) SnapshotForTree(ctx context.Context, id int64) (storage.ReadOnlyMapTreeTX, error) {
	return f.ReadOnlyTX, f.SnapshotErr
}

// ReadWriteTransaction implements MapStorage.ReadWriteTransaction
func (f *FakeMapStorage) ReadWriteTransaction(ctx context.Context, id int64, fn storage.MapTXFunc) error {
	return RunOnMapTX(f.TX)(ctx, id, fn)
}

// CheckDatabaseAccessible implements MapStorage.CheckDatabaseAccessible
func (f *FakeMapStorage) CheckDatabaseAccessible(ctx context.Context) error {
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
