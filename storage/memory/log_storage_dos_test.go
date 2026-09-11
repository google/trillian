// Copyright 2026 Google LLC. All Rights Reserved.
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
	"testing"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/testonly"
	"github.com/google/trillian/types"
)

// TestGetLeavesByRangeDoS reproduces, directly against the storage layer,
// the bug that used to let an attacker-controlled Count reach an unclamped
// make([]*trillian.LogLeaf, 0, count) allocation and crash the process
// (runtime error: makeslice: cap out of range). storage/mysql,
// storage/postgresql and storage/crdb had the identical shape: the
// TreeSize-based clamp only applied to TreeType_LOG, leaving
// TreeType_PREORDERED_LOG completely unbounded. The fix adds an
// unconditional maxGetLeavesByRangeCount cap that applies regardless of
// tree type.
func TestGetLeavesByRangeDoS(t *testing.T) {
	ctx := context.Background()
	ts := NewTreeStorage()
	as := NewAdminStorage(ts)

	tree, err := storage.CreateTree(ctx, as, testonly.PreorderedLogTree)
	if err != nil {
		t.Fatalf("CreateTree: %v", err)
	}

	ls := NewLogStorage(ts, nil)

	// A real deployment only ever hits this code path once the log has
	// published entries; simulate that by seeding a root directly.
	root, err := (&types.LogRootV1{
		RootHash:       make([]byte, 32),
		TimestampNanos: uint64(time.Now().UnixNano()),
	}).MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	err = ls.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.LogTreeTX) error {
		return tx.StoreSignedLogRoot(ctx, &trillian.SignedLogRoot{LogRoot: root})
	})
	if err != nil {
		t.Fatalf("seed StoreSignedLogRoot: %v", err)
	}

	tx, err := ls.SnapshotForTree(ctx, tree)
	if err != nil {
		t.Fatalf("SnapshotForTree: %v", err)
	}
	defer tx.Close()

	// Before the fix, this call panicked with "runtime error: makeslice: cap
	// out of range" instead of returning an error -- and nothing in the
	// gRPC handler chain recovers panics, so it would have crashed the
	// whole server process. If that regresses, fail loudly instead of
	// letting the test binary itself crash uninformatively.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("VULNERABLE: GetLeavesByRange(count=MaxInt64) panicked instead of returning an error: %v", r)
			}
		}()
		leaves, err := tx.GetLeavesByRange(ctx, 0, 9223372036854775807) // math.MaxInt64
		if err != nil {
			t.Fatalf("GetLeavesByRange returned an error instead of a clamped, safe result: %v", err)
		}
		if len(leaves) != 0 {
			// Empty tree, so no actual leaves are stored; the point of this
			// assertion is that we got here at all without allocating a
			// multi-exabyte slice first.
			t.Fatalf("got %d leaves, want 0 (empty tree)", len(leaves))
		}
	}()
}
