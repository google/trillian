// Copyright 2020 Google LLC. All Rights Reserved.
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

package storagetest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian"
	_ "github.com/google/trillian/merkle/rfc6962" // Register the hasher.
	"github.com/google/trillian/storage"
	"github.com/google/trillian/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/testing/protocmp"

	storageto "github.com/google/trillian/storage/testonly"
)

// LogStorageFactory creates LogStorage and AdminStorage for a test to use.
type LogStorageFactory = func(ctx context.Context, t *testing.T) (storage.LogStorage, storage.AdminStorage)

// LogStorageTest executes a test using the given storage implementations.
type LogStorageTest = func(ctx context.Context, t *testing.T, s storage.LogStorage, as storage.AdminStorage)

// RunLogStorageTests runs all the log storage tests against the provided log storage implementation.
func RunLogStorageTests(t *testing.T, storageFactory LogStorageFactory) {
	ctx := context.Background()
	for name, f := range logTestFunctions(t, &logTests{}) {
		s, as := storageFactory(ctx, t)
		t.Run(name, func(t *testing.T) { f(ctx, t, s, as) })
	}
}

func logTestFunctions(t *testing.T, x interface{}) map[string]LogStorageTest {
	tests := make(map[string]LogStorageTest)
	xv := reflect.ValueOf(x)
	for _, name := range testFunctions(x) {
		m := xv.MethodByName(name)
		if !m.IsValid() {
			t.Fatalf("storagetest: function %v is not valid", name)
		}
		i := m.Interface()
		f, ok := i.(LogStorageTest)
		if !ok {
			// Method exists but has the wrong type signature.
			t.Fatalf("storagetest: function %v has unexpected signature %T, %v", name, m.Interface(), m)
		}
		nickname := strings.TrimPrefix(name, "Test")
		tests[nickname] = f
	}
	return tests
}

// logTests is a suite of tests to run against the storage.LogTest interface.
type logTests struct{}

func (*logTests) TestCheckDatabaseAccessible(ctx context.Context, t *testing.T, s storage.LogStorage, as storage.AdminStorage) {
	if err := s.CheckDatabaseAccessible(ctx); err != nil {
		t.Errorf("CheckDatabaseAccessible() = %v, want = nil", err)
	}
}

func (*logTests) TestSnapshot(ctx context.Context, t *testing.T, s storage.LogStorage, as storage.AdminStorage) {
	frozenLog := mustCreateTree(ctx, t, as, storageto.LogTree)
	mustSignAndStoreLogRoot(ctx, t, s, frozenLog, &types.LogRootV1{})
	if _, err := storage.UpdateTree(ctx, as, frozenLog.TreeId, func(tree *trillian.Tree) {
		tree.TreeState = trillian.TreeState_FROZEN
	}); err != nil {
		t.Fatalf("Error updating frozen tree: %v", err)
	}

	activeLog := mustCreateTree(ctx, t, as, storageto.LogTree)
	mustSignAndStoreLogRoot(ctx, t, s, activeLog, &types.LogRootV1{})
	mapTreeID := mustCreateTree(ctx, t, as, storageto.MapTree).TreeId

	tests := []struct {
		desc    string
		tree    *trillian.Tree
		wantErr bool
	}{
		{
			desc:    "unknownSnapshot",
			tree:    logTree(-1),
			wantErr: true,
		},
		{
			desc: "activeLogSnapshot",
			tree: activeLog,
		},
		{
			desc: "frozenSnapshot",
			tree: frozenLog,
		},
		{
			desc:    "mapSnapshot",
			tree:    logTree(mapTreeID),
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			tx, err := s.SnapshotForTree(ctx, test.tree)

			if err == storage.ErrTreeNeedsInit {
				defer tx.Close()
			}

			if hasErr := err != nil; hasErr != test.wantErr {
				t.Fatalf("err: %v, wantErr = %v", err, test.wantErr)
			} else if hasErr {
				return
			}
			defer tx.Close()

			_, err = tx.LatestSignedLogRoot(ctx)
			if err != nil {
				t.Errorf("LatestSignedLogRoot() returned err: %v", err)
			}
			if err := tx.Commit(ctx); err != nil {
				t.Errorf("Commit() returned err: %v", err)
			}
		})
	}
}

func (*logTests) TestReadWriteTransaction(ctx context.Context, t *testing.T, s storage.LogStorage, as storage.AdminStorage) {
	activeLog := mustCreateTree(ctx, t, as, storageto.LogTree)
	mustSignAndStoreLogRoot(ctx, t, s, activeLog, &types.LogRootV1{RootHash: []byte{0}})

	tests := []struct {
		desc          string
		tree          *trillian.Tree
		wantNeedsInit bool
		wantErr       bool
		wantLogRoot   []byte
		wantTXRev     int64
	}{
		{
			desc:          "uninitializedBegin",
			tree:          logTree(-1),
			wantNeedsInit: true,
			wantTXRev:     0,
		},
		{
			desc: "activeLogBegin",
			tree: activeLog,
			wantLogRoot: func() []byte {
				b, err := (&types.LogRootV1{RootHash: []byte{0}}).MarshalBinary()
				if err != nil {
					panic(err)
				}
				return b
			}(),
			wantTXRev: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			err := s.ReadWriteTransaction(ctx, test.tree, func(ctx context.Context, tx storage.LogTreeTX) error {
				root, err := tx.LatestSignedLogRoot(ctx)
				if err != nil && !(err == storage.ErrTreeNeedsInit && test.wantNeedsInit) {
					t.Fatalf("%v: LatestSignedLogRoot() returned err = %v", test.desc, err)
				}
				gotRev, _ := tx.WriteRevision(ctx)
				if gotRev != test.wantTXRev {
					t.Errorf("%v: WriteRevision() = %v, want = %v", test.desc, gotRev, test.wantTXRev)
				}
				if got, want := root.GetLogRoot(), test.wantLogRoot; !bytes.Equal(got, want) {
					var logRoot types.LogRootV1
					if err := logRoot.UnmarshalBinary(got); err != nil {
						t.Error(err)
					}
					t.Errorf("%v: LogRoot: \n%x, want \n%x \nUnpacked: %#v", test.desc, got, want, logRoot)
				}
				return nil
			})
			if hasErr := err != nil; hasErr != test.wantErr {
				t.Fatalf("%v: err = %q, wantErr = %v", test.desc, err, test.wantErr)
			} else if hasErr {
				return
			}
		})
	}
}

func logTree(logID int64) *trillian.Tree {
	return &trillian.Tree{
		TreeId:       logID,
		TreeType:     trillian.TreeType_LOG,
		HashStrategy: trillian.HashStrategy_RFC6962_SHA256,
	}
}

// AddSequencedLeaves tests. ---------------------------------------------------

type addSequencedLeavesTest struct {
	t    *testing.T
	s    storage.LogStorage
	tree *trillian.Tree
}

func initAddSequencedLeavesTest(ctx context.Context, t *testing.T, s storage.LogStorage, as storage.AdminStorage) addSequencedLeavesTest {
	tree := mustCreateTree(ctx, t, as, storageto.PreorderedLogTree)
	mustSignAndStoreLogRoot(ctx, t, s, tree, &types.LogRootV1{})
	return addSequencedLeavesTest{t, s, tree}
}

func (t *addSequencedLeavesTest) addSequencedLeaves(leaves []*trillian.LogLeaf) {
	ctx := context.TODO()
	// Time we will queue all leaves at.
	fakeQueueTime := time.Date(2016, 11, 10, 15, 16, 27, 0, time.UTC)

	queued, err := t.s.AddSequencedLeaves(ctx, t.tree, leaves, fakeQueueTime)
	if err != nil {
		t.t.Fatalf("Failed to add sequenced leaves: %v", err)
	}
	if got, want := len(queued), len(leaves); got != want {
		t.t.Errorf("AddSequencedLeaves(): %v queued leaves, want %v", got, want)
	}
	// TODO(pavelkalinnikov): Verify returned status for each leaf.
}

func (t *addSequencedLeavesTest) verifySequencedLeaves(start, count int64, exp []*trillian.LogLeaf) {
	var stored []*trillian.LogLeaf
	runLogTX(t.s, t.tree, t.t, func(ctx context.Context, tx storage.LogTreeTX) error {
		var err error
		t.t.Logf("GetLeavesByRange(%v, %v)", start, count)
		stored, err = tx.GetLeavesByRange(ctx, start, count)
		if err != nil {
			t.t.Fatalf("Failed to read sequenced leaves: %v", err)
		}
		return nil
	})
	if got, want := len(stored), len(exp); got != want {
		t.t.Fatalf("Unexpected number of leaves: got %d, want %d", got, want)
	}

	for i, leaf := range stored {
		if got, want := leaf.LeafIndex, exp[i].LeafIndex; got != want {
			t.t.Fatalf("Leaf #%d: LeafIndex=%v, want %v", i, got, want)
		}
		if got, want := leaf.LeafIdentityHash, exp[i].LeafIdentityHash; !bytes.Equal(got, want) {
			t.t.Fatalf("Leaf #%d: LeafIdentityHash=%v, want %v", i, got, want)
		}
	}
}

func (*logTests) TestAddSequencedLeavesUnordered(ctx context.Context, t *testing.T, s storage.LogStorage, as storage.AdminStorage) {
	const chunk = 5
	const count = chunk * 5
	const extraCount = 16
	leaves := createTestLeaves(count, 0)

	aslt := initAddSequencedLeavesTest(ctx, t, s, as)
	for _, idx := range []int{1, 0, 4, 2} {
		aslt.addSequencedLeaves(leaves[chunk*idx : chunk*(idx+1)])
	}
	aslt.verifySequencedLeaves(0, count+extraCount, leaves[:chunk*3])
	aslt.verifySequencedLeaves(chunk*4, chunk+extraCount, leaves[chunk*4:count])
	aslt.addSequencedLeaves(leaves[chunk*3 : chunk*4])
	aslt.verifySequencedLeaves(0, count+extraCount, leaves)
}

func (*logTests) TestAddSequencedLeavesWithDuplicates(ctx context.Context, t *testing.T, s storage.LogStorage, as storage.AdminStorage) {
	leaves := createTestLeaves(6, 0)

	aslt := initAddSequencedLeavesTest(ctx, t, s, as)
	aslt.addSequencedLeaves(leaves[:3])
	aslt.verifySequencedLeaves(0, 3, leaves[:3])
	aslt.addSequencedLeaves(leaves[2:]) // Full dup.
	aslt.verifySequencedLeaves(0, 6, leaves)

	dupLeaves := createTestLeaves(4, 6)
	dupLeaves[0].LeafIdentityHash = leaves[0].LeafIdentityHash // Hash dup.
	dupLeaves[2].LeafIndex = 2                                 // Index dup.
	leafHash := sha256.Sum256([]byte("foobar"))
	dupLeaves[2].LeafIdentityHash = leafHash[:] // TODO: Remove when spannertest has transaction support.
	aslt.addSequencedLeaves(dupLeaves)
	aslt.verifySequencedLeaves(6, 4, nil)
	aslt.verifySequencedLeaves(7, 4, dupLeaves[1:2])
	aslt.verifySequencedLeaves(8, 4, nil)
	aslt.verifySequencedLeaves(9, 4, dupLeaves[3:4])

	dupLeaves = createTestLeaves(4, 6)
	aslt.addSequencedLeaves(dupLeaves)
	aslt.verifySequencedLeaves(6, 4, dupLeaves)
}

// Time we'll request for guard cutoff in tests that don't test this (should include all above)
var fakeDequeueCutoffTime = time.Date(2016, 11, 10, 15, 16, 30, 0, time.UTC)

func (*logTests) TestDequeueLeavesNoneQueued(ctx context.Context, t *testing.T, s storage.LogStorage, as storage.AdminStorage) {
	tree := mustCreateTree(ctx, t, as, storageto.LogTree)

	runLogTX(s, tree, t, func(ctx context.Context, tx storage.LogTreeTX) error {
		leaves, err := tx.DequeueLeaves(ctx, 999, fakeDequeueCutoffTime)
		if err != nil {
			t.Fatalf("Didn't expect an error on dequeue with no work to be done: %v", err)
		}
		if len(leaves) > 0 {
			t.Fatalf("Expected nothing to be dequeued but we got %d leaves", len(leaves))
		}
		return nil
	})
}

// GetLeavesByRange tests. -----------------------------------------------------

type getLeavesByRangeTest struct {
	start, count int64
	want         []int64
	wantErr      bool
}

func testGetLeavesByRangeImpl(ctx context.Context, t *testing.T, s storage.LogStorage, as storage.AdminStorage, create *trillian.Tree, tests []getLeavesByRangeTest) {
	tree := mustCreateTree(ctx, t, as, create)

	// Note: GetLeavesByRange loads the root internally to get the tree size.
	mustSignAndStoreLogRoot(ctx, t, s, tree, &types.LogRootV1{TreeSize: 14})

	// Create leaves [0]..[19] but drop leaf [5] and set the tree size to 14.
	for i := int64(0); i < 20; i++ {
		if i == 5 {
			continue
		}
		data := []byte{byte(i)}
		someExtraData := []byte("Some extra data")
		identityHash := sha256.Sum256(data)
		createFakeLeaf(ctx, s, tree, identityHash[:], identityHash[:], data, someExtraData, i, t)
	}

	for _, test := range tests {
		runLogTX(s, tree, t, func(ctx context.Context, tx storage.LogTreeTX) error {
			leaves, err := tx.GetLeavesByRange(ctx, test.start, test.count)
			if err != nil {
				if !test.wantErr {
					t.Errorf("GetLeavesByRange(%d, +%d)=_,%v; want _,nil", test.start, test.count, err)
				}
				return nil
			}
			if test.wantErr {
				t.Errorf("GetLeavesByRange(%d, +%d)=_,nil; want _,non-nil", test.start, test.count)
			}
			got := make([]int64, len(leaves))
			for i, leaf := range leaves {
				got[i] = leaf.LeafIndex
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("GetLeavesByRange(%d, +%d)=%+v; want %+v", test.start, test.count, got, test.want)
			}
			return nil
		})
	}
}

func (*logTests) TestGetLeavesByRangeFromLog(ctx context.Context, t *testing.T, s storage.LogStorage, as storage.AdminStorage) {
	tests := []getLeavesByRangeTest{
		{start: 0, count: 1, want: []int64{0}},
		{start: 0, count: 2, want: []int64{0, 1}},
		{start: 1, count: 3, want: []int64{1, 2, 3}},
		{start: 10, count: 7, want: []int64{10, 11, 12, 13}},
		{start: 13, count: 1, want: []int64{13}},
		{start: 14, count: 4, wantErr: true},   // Starts right after tree size.
		{start: 19, count: 2, wantErr: true},   // Starts further away.
		{start: 3, count: 5, wantErr: true},    // Hits non-contiguous leaves.
		{start: 5, count: 5, wantErr: true},    // Starts from a missing leaf.
		{start: 1, count: 0, wantErr: true},    // Empty range.
		{start: -1, count: 1, wantErr: true},   // Negative start.
		{start: 1, count: -1, wantErr: true},   // Negative count.
		{start: 100, count: 30, wantErr: true}, // Starts after all stored leaves.
	}
	testGetLeavesByRangeImpl(ctx, t, s, as, storageto.LogTree, tests)
}

func (*logTests) TestGetLeavesByRangeFromPreorderedLog(ctx context.Context, t *testing.T, s storage.LogStorage, as storage.AdminStorage) {
	tests := []getLeavesByRangeTest{
		{start: 0, count: 1, want: []int64{0}},
		{start: 0, count: 2, want: []int64{0, 1}},
		{start: 1, count: 3, want: []int64{1, 2, 3}},
		{start: 10, count: 7, want: []int64{10, 11, 12, 13, 14, 15, 16}},
		{start: 13, count: 1, want: []int64{13}},
		// Starts right after tree size.
		{start: 14, count: 4, want: []int64{14, 15, 16, 17}},
		{start: 19, count: 2, want: []int64{19}}, // Starts further away.
		{start: 3, count: 5, wantErr: true},      // Hits non-contiguous leaves.
		{start: 5, count: 5, wantErr: true},      // Starts from a missing leaf.
		{start: 1, count: 0, wantErr: true},      // Empty range.
		{start: -1, count: 1, wantErr: true},     // Negative start.
		{start: 1, count: -1, wantErr: true},     // Negative count.
		{start: 100, count: 30, want: []int64{}}, // Starts after all stored leaves.
	}
	testGetLeavesByRangeImpl(ctx, t, s, as, storageto.PreorderedLogTree, tests)
}

// Time we will queue all leaves at
var fakeQueueTime = time.Date(2016, 11, 10, 15, 16, 27, 0, time.UTC)

func createFakeLeaf(ctx context.Context, s storage.LogStorage, tree *trillian.Tree, rawHash, hash, data, extraData []byte, seq int64, t *testing.T) *trillian.LogLeaf {
	t.Helper()
	leaf := &trillian.LogLeaf{
		MerkleLeafHash:   hash,
		LeafValue:        data,
		ExtraData:        extraData,
		LeafIndex:        seq,
		LeafIdentityHash: rawHash,
	}
	q, err := s.AddSequencedLeaves(ctx, tree, []*trillian.LogLeaf{leaf}, fakeQueueTime)
	if err != nil {
		t.Fatalf("Failed to create test leaves: %v", err)
	}

	return q[0].Leaf
}

func (*logTests) TestDequeueLeaves(ctx context.Context, t *testing.T, s storage.LogStorage, as storage.AdminStorage) {
	const leavesToInsert = 5
	tree := mustCreateTree(ctx, t, as, storageto.LogTree)
	mustSignAndStoreLogRoot(ctx, t, s, tree, &types.LogRootV1{})

	leaves := createTestLeaves(leavesToInsert, 20)
	if _, err := s.QueueLeaves(ctx, tree, leaves, fakeDequeueCutoffTime); err != nil {
		t.Fatalf("Failed to queue leaves: %v", err)
	}

	// Now try to dequeue them
	// Some dequeue implementations probabalistically dequeue and require retrying until timeout.
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second) // Retry until timeout
	defer cancel()
	leaves2 := dequeueAndSequence(cctx, t, s, tree, fakeDequeueCutoffTime, leavesToInsert, 0)
	if got, want := len(leaves2), leavesToInsert; got != want {
		t.Fatalf("Got %d leaves want %d", got, want)
	}

	// If we dequeue again then we should now get nothing
	if err := s.ReadWriteTransaction(ctx, tree,
		func(ctx context.Context, tx storage.LogTreeTX) error {
			leaves, err := tx.DequeueLeaves(ctx, 99, fakeDequeueCutoffTime)
			if err != nil {
				t.Fatalf("Failed to dequeue leaves (second time): %v", err)
			}
			if got, want := len(leaves), 0; got != want {
				t.Fatalf("Got %d leaves want %d", got, want)
			}
			return nil
		},
	); err != nil {
		t.Fatalf("Could not dequeue the expected number of leaves: %v", err)
	}
}

// dequeueAndSequence repeatedly dequeues in a single transaction until limit is reached or a timeout occurs.
// Then, it sequences the leaves with UpdateSequencedLeaves.
func dequeueAndSequence(ctx context.Context, t *testing.T, ls storage.LogStorage, tree *trillian.Tree, ts time.Time, limit int, startIndex int64) []*trillian.LogLeaf {
	t.Helper()
	// We'll retry a few times if we get nothing back since we're now dependent
	// on the underlying queue delivering unsequenced entries.
	var ret []*trillian.LogLeaf
	err := ls.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.LogTreeTX) error {
		ret = make([]*trillian.LogLeaf, 0, limit)
		i := 0
		start := time.Now()
		for rem := limit; rem > 0; {
			i++
			got, err := tx.DequeueLeaves(ctx, rem, ts)
			if err != nil {
				return err
			}
			ret = append(ret, got...)
			rem -= len(got)
		}
		t.Logf("DequeueLeaves took %v tries and %v to dequeue %d leaves", i, time.Since(start), len(ret))
		ensureAllLeavesDistinct(t, ret)
		ensureLeavesHaveQueueTimestamp(t, ret, ts)
		iTimestamp := ptypes.TimestampNow()
		for i, l := range ret {
			l.IntegrateTimestamp = iTimestamp
			l.LeafIndex = int64(i) + startIndex
		}
		if err := tx.UpdateSequencedLeaves(ctx, ret); err != nil {
			return fmt.Errorf("UpdateSequencedLeaves(): %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("dequeueLeaves(limit: %v): %v", limit, err)
	}
	return ret
}

func ensureAllLeavesDistinct(t *testing.T, leaves []*trillian.LogLeaf) {
	t.Helper()
	set := make(map[string]bool)
	for _, l := range leaves {
		k := string(l.LeafIdentityHash)
		if _, ok := set[k]; ok {
			t.Fatalf("Unexpectedly got a duplicate leaf hash: %x", l.LeafIdentityHash)
		}
		set[k] = true
	}
}

func ensureLeavesHaveQueueTimestamp(t *testing.T, leaves []*trillian.LogLeaf, want time.Time) {
	t.Helper()
	for _, leaf := range leaves {
		gotQTimestamp, err := ptypes.Timestamp(leaf.QueueTimestamp)
		if err != nil {
			t.Fatalf("Got invalid queue timestamp: %v", err)
		}
		if got, want := gotQTimestamp.UnixNano(), want.UnixNano(); got != want {
			t.Errorf("Got leaf with QueueTimestampNanos = %v, want %v: %v", got, want, leaf)
		}
	}
}

func (*logTests) TestDequeueLeavesTwoBatches(ctx context.Context, t *testing.T, s storage.LogStorage, as storage.AdminStorage) {
	fakeDequeueCutoffTime := time.Date(2016, 11, 10, 15, 16, 30, 0, time.UTC)
	const leavesToInsert = 5
	tree := mustCreateTree(ctx, t, as, storageto.LogTree)
	mustSignAndStoreLogRoot(ctx, t, s, tree, &types.LogRootV1{})

	leavesToDequeue1 := 3
	leavesToDequeue2 := 2

	leaves := createTestLeaves(leavesToInsert, 20)
	if _, err := s.QueueLeaves(ctx, tree, leaves, fakeDequeueCutoffTime); err != nil {
		t.Fatalf("Failed to queue leaves: %v", err)
	}

	cctx, cancel := context.WithTimeout(ctx, 5*time.Second) // Retry until timeout
	defer cancel()
	leaves2 := dequeueAndSequence(cctx, t, s, tree, fakeDequeueCutoffTime, leavesToDequeue1, 0)
	if len(leaves2) != leavesToDequeue1 {
		t.Fatalf("Dequeued %d leaves but expected to get %d", len(leaves2), leavesToDequeue1)
	}

	mustSignAndStoreLogRoot(ctx, t, s, tree, &types.LogRootV1{
		Revision:       1,
		TreeSize:       uint64(leavesToDequeue1),
		TimestampNanos: 1,
	})
	leaves3 := dequeueAndSequence(cctx, t, s, tree, fakeDequeueCutoffTime, leavesToDequeue2, int64(leavesToDequeue1))
	if len(leaves3) != leavesToDequeue2 {
		t.Fatalf("Dequeued %d leaves but expected to get %d", len(leaves2), leavesToDequeue2)
	}

	// Plus the union of the leaf batches should all have distinct hashes
	ensureAllLeavesDistinct(t, append(leaves2, leaves3...))

	// If we dequeue again then we should now get nothing
	runLogTX(s, tree, t, func(ctx context.Context, tx4 storage.LogTreeTX) error {
		leaves5, err := tx4.DequeueLeaves(ctx, 99, fakeDequeueCutoffTime)
		if err != nil {
			t.Fatalf("Failed to dequeue leaves (second time): %v", err)
		}
		if len(leaves5) != 0 {
			t.Fatalf("Dequeued %d leaves but expected to get none", len(leaves5))
		}
		return nil
	})
}

func (*logTests) TestAddSequencedLeavesAndDequeueLeaves(ctx context.Context, t *testing.T, s storage.LogStorage, as storage.AdminStorage) {
	tree := mustCreateTree(ctx, t, as, storageto.PreorderedLogTree)
	mustSignAndStoreLogRoot(ctx, t, s, tree, &types.LogRootV1{})

	leaves := createTestLeaves(3, 0)
	leaves[0].ExtraData = nil

	now := time.Now()
	res, err := s.AddSequencedLeaves(ctx, tree, leaves, now)
	if err != nil {
		t.Errorf("AddSequencedLeaves(_, %v, %+v, %v): %v", tree.TreeId, leaves, now, err)
	}

	if got, want := len(res), len(leaves); got != want {
		t.Fatalf("AddSequencedLeaves returned %v results, want %v", got, want)
	}
	for i, r := range res {
		if got, want := status.FromProto(r.Status).Code(), codes.OK; got != want {
			t.Errorf("res[%d]=%v, want %v", i, got, want)
		}
	}

	qts, _ := ptypes.TimestampProto(now)
	its, _ := ptypes.TimestampProto(time.Unix(0, 0))

	partial := make([]*trillian.LogLeaf, 0, len(leaves))
	for _, leaf := range leaves {
		partial = append(partial, &trillian.LogLeaf{
			LeafIdentityHash:   leaf.LeafIdentityHash,
			MerkleLeafHash:     leaf.MerkleLeafHash,
			LeafValue:          leaf.LeafValue,
			ExtraData:          leaf.ExtraData,
			LeafIndex:          leaf.LeafIndex,
			QueueTimestamp:     qts,
			IntegrateTimestamp: its,
		})
	}

	// Check that the first sequenced entry is returned.
	dequeued, err := dequeueLeavesInTx(ctx, s, tree, now, 1)
	if err != nil {
		t.Fatalf("dequeueLeaves(): %v", err)
	}
	if diff := cmp.Diff(dequeued, partial[:1], protocmp.Transform()); diff != "" {
		t.Errorf("dequeueLeaves() diff: %v", diff)
	}

	// Fake a signing run that signs 1 entry.
	mustSignAndStoreLogRoot(ctx, t, s, tree, &types.LogRootV1{
		TimestampNanos: uint64(time.Now().UnixNano()),
		TreeSize:       1,
		RootHash:       []byte("roothash"),
		Revision:       1,
	})

	// Check that the 2nd and 3rd sequenced entries are returned.
	dequeued, err = dequeueLeavesInTx(ctx, s, tree, now, 10)
	if err != nil {
		t.Fatalf("dequeueLeaves(): %v", err)
	}
	if diff := cmp.Diff(dequeued, partial[1:3], protocmp.Transform()); diff != "" {
		t.Errorf("dequeueLeaves() diff: %v", diff)
	}
}
