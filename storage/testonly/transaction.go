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

	"github.com/google/trillian/storage"
)

// RunOnLogTX is a helper for mocking out the LogStorage.ReadWriteTransaction method.
func RunOnLogTX(tx storage.LogTreeTX) func(ctx context.Context, treeID int64, f storage.LogTXFunc) error {
	return func(ctx context.Context, _ int64, f storage.LogTXFunc) error {
		defer tx.Close()
		return f(ctx, tx)
	}
}

// RunOnMapTX is a helper for mocking out the MapStorage.ReadWriteTransaction method.
func RunOnMapTX(tx storage.MapTreeTX) func(ctx context.Context, treeID int64, f storage.MapTXFunc) error {
	return func(ctx context.Context, _ int64, f storage.MapTXFunc) error {
		defer tx.Close()
		return f(ctx, tx)
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
