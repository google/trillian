// Copyright 2016 Google Inc. All Rights Reserved.
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

package mysql

import (
	"bytes"
	"context"
	"crypto"
	"fmt"
	"strings"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/examples/ct/ctmapper/ctmapperpb"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/testdb"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/trees"
	"github.com/google/trillian/types"
	"github.com/kylelemons/godebug/pretty"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tcrypto "github.com/google/trillian/crypto"
	storageto "github.com/google/trillian/storage/testonly"
)

var fixedSigner = tcrypto.NewSigner(0, testonly.NewSignerWithFixedSig(nil, []byte("notempty")), crypto.SHA256)

func MustSignMapRoot(root *types.MapRootV1) *trillian.SignedMapRoot {
	r, err := fixedSigner.SignMapRoot(root)
	if err != nil {
		panic(fmt.Sprintf("SignMapRoot(): %v", err))
	}
	return r
}

func TestMySQLMapStorage_CheckDatabaseAccessible(t *testing.T) {
	testdb.SkipIfNoMySQL(t)

	cleanTestDB(DB)
	s := NewMapStorage(DB)
	if err := s.CheckDatabaseAccessible(context.Background()); err != nil {
		t.Errorf("CheckDatabaseAccessible() = %v, want = nil", err)
	}
}

func TestMapSnapshot(t *testing.T) {
	testdb.SkipIfNoMySQL(t)

	cleanTestDB(DB)
	ctx := context.Background()
	as := NewAdminStorage(DB)
	s := NewMapStorage(DB)
	frozenMap := createInitializedMapForTests(ctx, t, s, as)
	storage.UpdateTree(ctx, as, frozenMap.TreeId, func(tree *trillian.Tree) {
		tree.TreeState = trillian.TreeState_FROZEN
	})

	activeMap := createInitializedMapForTests(ctx, t, s, as)
	logID := mustCreateTree(ctx, t, as, storageto.LogTree).TreeId

	tests := []struct {
		desc    string
		tree    *trillian.Tree
		wantErr bool
	}{
		{
			desc:    "unknownSnapshot",
			tree:    mapTree(-1),
			wantErr: true,
		},
		{
			desc: "activeMapSnapshot",
			tree: activeMap,
		},
		{
			desc: "frozenSnapshot",
			tree: frozenMap,
		},
		{
			desc:    "logSnapshot",
			tree:    mapTree(logID),
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			tx, err := s.SnapshotForTree(ctx, test.tree)
			if err != nil {
				t.Fatalf("SnapshotForTree()=_,%v; want _, nil", err)
			}
			defer tx.Close()

			_, err = tx.LatestSignedMapRoot(ctx)
			if gotErr := (err != nil); gotErr != test.wantErr {
				t.Errorf("LatestSignedMapRoot()=_,%v; want _, err? %v", err, test.wantErr)
			}
			if err != nil {
				return
			}
			if err := tx.Commit(ctx); err != nil {
				t.Errorf("Commit()=_,%v; want _,nil", err)
			}
		})
	}
}

func TestMapReadWriteTransaction(t *testing.T) {
	testdb.SkipIfNoMySQL(t)

	cleanTestDB(DB)
	ctx := context.Background()
	as := NewAdminStorage(DB)
	s := NewMapStorage(DB)
	activeMap := createInitializedMapForTests(ctx, t, s, as)

	tests := []struct {
		desc        string
		tree        *trillian.Tree
		wantRev     int64
		wantTXRev   int64
		wantErr     bool
		wantRootErr string
	}{
		{
			desc:        "unknownBegin",
			tree:        mapTree(-1),
			wantRev:     0,
			wantTXRev:   -1,
			wantRootErr: "needs initialising",
		},
		{
			desc:      "activeMapBegin",
			tree:      activeMap,
			wantRev:   0,
			wantTXRev: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			err := s.ReadWriteTransaction(ctx, test.tree, func(ctx context.Context, tx storage.MapTreeTX) error {
				root, err := tx.LatestSignedMapRoot(ctx)
				if err != nil {
					if !strings.Contains(err.Error(), test.wantRootErr) {
						t.Errorf("LatestSignedMapRoot() returned err = %v", err)
					}
					return nil
				}
				if len(test.wantRootErr) != 0 {
					t.Fatalf("LatestSignedMapRoot() returned err = %v, want: nil", err)
				}
				var mapRoot types.MapRootV1
				if err := mapRoot.UnmarshalBinary(root.MapRoot); err != nil {
					t.Fatalf("UmarshalBinary(): %v", err)
				}
				gotRev, _ := tx.WriteRevision(ctx)
				if gotRev != test.wantTXRev {
					t.Errorf("WriteRevision() = %v, want = %v", gotRev, test.wantTXRev)
				}
				if got, want := int64(mapRoot.Revision), test.wantRev; got != want {
					t.Errorf("TreeRevision() = %v, want = %v", got, want)
				}
				return nil
			})
			if hasErr := err != nil; hasErr != test.wantErr {
				t.Fatalf("err = %q, wantErr = %v", err, test.wantErr)
			} else if hasErr {
				return
			}
		})
	}
}

func TestGetLastInRange(t *testing.T) {
	testdb.SkipIfNoMySQL(t)
	ctx := context.Background()

	nodeA := storage.NewNodeIDFromHash([]byte("index1111\x00"))
	nodeB := storage.NewNodeIDFromHash([]byte("index1111\x01"))
	nodeQuery := nodeA.MaskLeft(len("index1111") * 8)
	//nodeC := storage.NewNodeIDFromHash([]byte("index1111\x0F"))

	for _, tc := range []struct {
		desc     string
		node     *storage.NodeID
		write    [][]*trillian.MapLeaf
		want     *trillian.MapLeaf
		wantCode codes.Code
	}{
		//{desc: "0leaf", node: nodeQuery, wantCode: codes.NotFound},
		{desc: "1leaf", node: nodeQuery, want: &trillian.MapLeaf{Index: nodeA.Path, LeafValue: []byte("A")},
			write: [][]*trillian.MapLeaf{{
				&trillian.MapLeaf{Index: nodeA.Path, LeafValue: []byte("A")},
			}},
		},
		{desc: "2leaves", node: nodeQuery, want: &trillian.MapLeaf{Index: nodeB.Path, LeafValue: []byte("B")},
			write: [][]*trillian.MapLeaf{{
				&trillian.MapLeaf{Index: nodeA.Path, LeafValue: []byte("A")},
				&trillian.MapLeaf{Index: nodeB.Path, LeafValue: []byte("B")},
			}},
		},
		{desc: "multirev", node: nodeQuery, want: &trillian.MapLeaf{Index: nodeB.Path, LeafValue: []byte("B2")},
			write: [][]*trillian.MapLeaf{
				{
					&trillian.MapLeaf{Index: nodeA.Path, LeafValue: []byte("A1")},
					&trillian.MapLeaf{Index: nodeB.Path, LeafValue: []byte("B1")},
				},
				{
					&trillian.MapLeaf{Index: nodeA.Path, LeafValue: []byte("A2")},
					&trillian.MapLeaf{Index: nodeB.Path, LeafValue: []byte("B2")},
				},
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			cleanTestDB(DB)
			s := NewMapStorage(DB)
			as := NewAdminStorage(DB)
			tree := createInitializedMapForTests(ctx, t, s, as)
			for rev, leaves := range tc.write {
				if err := s.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.MapTreeTX) error {
					for _, leaf := range leaves {
						if err := tx.Set(ctx, leaf.Index, leaf); err != nil {
							t.Fatalf("Set(%s): %v", leaf.LeafValue, err)
						}
					}
					smr := &types.MapRootV1{TimestampNanos: uint64(rev + 1), Revision: uint64(rev + 1)}
					signer, err := trees.Signer(ctx, tree)
					if err != nil {
						t.Fatalf("trees.Signer(): %v", err)
					}
					root, err := signer.SignMapRoot(smr)
					if err != nil {
						t.Fatalf("SignMapRoot(): %v", err)
					}
					if err := tx.StoreSignedMapRoot(ctx, root); err != nil {
						t.Fatalf("StoreSignedMapRoot(%v): %v", smr, err)
					}
					return nil
				}); err != nil {
					t.Fatalf("ReadWriteTransaction(rev: %v): %v", rev, err)
				}
			}
			rev := int64(len(tc.write)) + 1
			if err := s.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.MapTreeTX) error {
				leaf, err := tx.GetLastInRange(ctx, rev+1, tc.node)
				if got := status.Code(err); got != tc.wantCode {
					t.Fatalf("GetLastInRange(): %v, want %v", err, tc.wantCode)
				}
				if !proto.Equal(leaf, tc.want) {
					t.Errorf("GetLastInRange(): %v, want %v", leaf, tc.want)
				}
				return nil
			}); err != nil {
				t.Errorf("ReadWriteTransaction(): %v", err)
			}
		})
	}
}

func TestMapRootUpdate(t *testing.T) {
	testdb.SkipIfNoMySQL(t)

	cleanTestDB(DB)
	ctx := context.Background()
	as := NewAdminStorage(DB)
	s := NewMapStorage(DB)
	tree := createInitializedMapForTests(ctx, t, s, as)

	populatedMetadata := testonly.MustMarshalAnyNoT(&ctmapperpb.MapperMetadata{HighestFullyCompletedSeq: 1})

	for _, tc := range []struct {
		desc         string
		root         *trillian.SignedMapRoot
		wantMetadata []byte
	}{
		{
			desc: "Initial root",
			root: MustSignMapRoot(&types.MapRootV1{
				TimestampNanos: 98765,
				Revision:       5,
				RootHash:       []byte(dummyHash),
			}),
		},
		{
			desc: "Root update",
			root: MustSignMapRoot(&types.MapRootV1{
				TimestampNanos: 98766,
				Revision:       6,
				RootHash:       []byte(dummyHash),
			}),
		},
		{
			desc: "Root with default (empty) MapperMetadata",
			root: MustSignMapRoot(&types.MapRootV1{
				TimestampNanos: 98768,
				Revision:       7,
				RootHash:       []byte(dummyHash),
			}),
		},
		{
			desc: "Root with non-default (populated) MapperMetadata",
			root: MustSignMapRoot(&types.MapRootV1{
				TimestampNanos: 98769,
				Revision:       8,
				RootHash:       []byte(dummyHash),
				Metadata:       populatedMetadata,
			}),
			wantMetadata: populatedMetadata,
		},
	} {
		func() {
			runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
				if err := tx.StoreSignedMapRoot(ctx, tc.root); err != nil {
					t.Fatalf("%v: Failed to store signed map root: %v", tc.desc, err)
				}
				return nil
			})
		}()

		func() {
			runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
				smr, err := tx.LatestSignedMapRoot(ctx)
				if err != nil {
					t.Fatalf("%v: Failed to read back new map root: %v", tc.desc, err)
				}

				var root types.MapRootV1
				if err := root.UnmarshalBinary(smr.MapRoot); err != nil {
					t.Fatalf("%v: UnmarshalBinary(): %v", tc.desc, err)
				}

				if got, want := root.Metadata, tc.wantMetadata; !bytes.Equal(got, want) {
					t.Errorf("%v: LatestSignedMapRoot() diff(-got, +want) \n%v", tc.desc, pretty.Compare(got, want))
				}
				return nil
			})
		}()
	}
}

var keyHash = []byte([]byte("A Key Hash"))
var mapLeaf = &trillian.MapLeaf{
	Index:     keyHash,
	LeafHash:  []byte("A Hash"),
	LeafValue: []byte("A Value"),
	ExtraData: []byte("Some Extra Data"),
}

func TestMapSetGetRoundTrip(t *testing.T) {
	testdb.SkipIfNoMySQL(t)

	cleanTestDB(DB)
	ctx := context.Background()
	as := NewAdminStorage(DB)
	s := NewMapStorage(DB)
	tree := createInitializedMapForTests(ctx, t, s, as)

	readRev := int64(1)
	{
		runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
			if err := tx.Set(ctx, keyHash, mapLeaf); err != nil {
				t.Fatalf("Failed to set %v to %v: %v", keyHash, mapLeaf, err)
			}
			return nil
		})
	}

	{
		runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
			readValues, err := tx.Get(ctx, readRev, [][]byte{keyHash})
			if err != nil {
				t.Fatalf("Failed to get %v:  %v", keyHash, err)
			}
			if got, want := len(readValues), 1; got != want {
				t.Fatalf("Got %d values, expected %d", got, want)
			}
			if got, want := readValues[0], mapLeaf; !proto.Equal(got, want) {
				t.Fatalf("Read back %v, but expected %v", got, want)
			}
			return nil
		})
	}
}

func TestMapSetSameKeyInSameRevisionFails(t *testing.T) {
	testdb.SkipIfNoMySQL(t)

	cleanTestDB(DB)
	ctx := context.Background()
	as := NewAdminStorage(DB)
	s := NewMapStorage(DB)
	tree := createInitializedMapForTests(ctx, t, s, as)

	{
		runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
			if err := tx.Set(ctx, keyHash, mapLeaf); err != nil {
				t.Fatalf("Failed to set %v to %v: %v", keyHash, mapLeaf, err)
			}
			return nil
		})
	}

	{
		runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
			if err := tx.Set(ctx, keyHash, mapLeaf); err == nil {
				t.Fatalf("Unexpectedly succeeded in setting %v to %v", keyHash, mapLeaf)
			}
			return nil
		})
	}
}

func TestMapGet0Results(t *testing.T) {
	testdb.SkipIfNoMySQL(t)

	cleanTestDB(DB)
	ctx := context.Background()
	as := NewAdminStorage(DB)
	s := NewMapStorage(DB)
	tree := createInitializedMapForTests(ctx, t, s, as)

	for _, tc := range []struct {
		index [][]byte
	}{
		{index: nil}, //empty list.
		{index: [][]byte{[]byte("This doesn't exist.")}},
	} {
		t.Run(fmt.Sprintf("tx.Get(%s)", tc.index), func(t *testing.T) {
			runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
				readValues, err := tx.Get(ctx, 1, tc.index)
				if err != nil {
					t.Fatal(err)
				}
				if got, want := len(readValues), 0; got != want {
					t.Fatalf("len = %d, want %d", got, want)
				}
				return nil
			})
		})
	}
}

func TestMapSetGetMultipleRevisions(t *testing.T) {
	testdb.SkipIfNoMySQL(t)

	// Write two roots for a map and make sure the one with the newest timestamp supersedes
	cleanTestDB(DB)
	ctx := context.Background()
	as := NewAdminStorage(DB)
	s := NewMapStorage(DB)
	tree := createInitializedMapForTests(ctx, t, s, as)

	tests := []struct {
		rev  int64
		leaf *trillian.MapLeaf
	}{
		{0, &trillian.MapLeaf{Index: keyHash, LeafHash: []byte{0}, LeafValue: []byte{0}, ExtraData: []byte{0}}},
		{1, &trillian.MapLeaf{Index: keyHash, LeafHash: []byte{1}, LeafValue: []byte{1}, ExtraData: []byte{1}}},
		{2, &trillian.MapLeaf{Index: keyHash, LeafHash: []byte{2}, LeafValue: []byte{2}, ExtraData: []byte{2}}},
		{3, &trillian.MapLeaf{Index: keyHash, LeafHash: []byte{3}, LeafValue: []byte{3}, ExtraData: []byte{3}}},
	}

	for _, tc := range tests {
		func() {
			// Write the current test case.
			runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
				mapTX := tx.(*mapTreeTX)
				mapTX.treeTX.writeRevision = tc.rev
				if err := tx.Set(ctx, keyHash, tc.leaf); err != nil {
					t.Fatalf("Failed to set %v to %v: %v", keyHash, tc.leaf, err)
				}
				return nil
			})

			// Read at a point in time in the future. Expect to get the latest value.
			// Read at each point in the past. Expect to get that exact point in history.
			for i := int64(0); i < int64(len(tests)); i++ {
				func() {
					expectRev := i
					if expectRev > tc.rev {
						expectRev = tc.rev // For future revisions, expect the current value.
					}

					runMapTX(ctx, s, tree, t, func(ctx context.Context, tx2 storage.MapTreeTX) error {
						readValues, err := tx2.Get(ctx, i, [][]byte{keyHash})
						if err != nil {
							t.Fatalf("At i %d failed to get %v:  %v", i, keyHash, err)
						}
						if got, want := len(readValues), 1; got != want {
							t.Fatalf("At i %d got %d values, expected %d", i, got, want)
						}
						if got, want := readValues[0], tests[expectRev].leaf; !proto.Equal(got, want) {
							t.Fatalf("At i %d read back %v, but expected %v", i, got, want)
						}
						return nil
					})
				}()
			}
		}()
	}
}

func TestGetSignedMapRootNotExist(t *testing.T) {
	testdb.SkipIfNoMySQL(t)

	ctx := context.Background()
	cleanTestDB(DB)
	as := NewAdminStorage(DB)
	tree := mustCreateTree(ctx, t, as, storageto.MapTree) // Uninitialized: no revision 0 MapRoot exists.
	s := NewMapStorage(DB)

	err := s.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.MapTreeTX) error {
		_, err := tx.GetSignedMapRoot(ctx, 0)
		if got, want := err, storage.ErrTreeNeedsInit; got != want {
			t.Fatalf("GetSignedMapRoot: %v, want %v", got, want)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ReadWriteTransaction: %v", err)
	}
}

func TestLatestSignedMapRootNoneWritten(t *testing.T) {
	// TODO(phad): I'm considering removing this test, because for an Map that has been
	// initialized there should always be the revision 0 SMR written to the DB, and
	// without initialization the error path is identical to that tested in the func
	// TestGetSignedMapRootNotExist above.
	t.Skip("TODO: remove this as it can no longer occur.")

	cleanTestDB(DB)
	ctx := context.Background()
	as := NewAdminStorage(DB)
	s := NewMapStorage(DB)
	tree := createInitializedMapForTests(ctx, t, s, as)

	runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
		root, err := tx.LatestSignedMapRoot(ctx)
		if err != nil {
			t.Fatalf("Failed to read an empty map root: %v", err)
		}
		if len(root.MapRoot) != 0 || root.Signature != nil {
			t.Fatalf("Read a root with contents when it should be empty: %v", root)
		}
		return nil
	})
}

func TestGetSignedMapRoot(t *testing.T) {
	testdb.SkipIfNoMySQL(t)

	cleanTestDB(DB)
	ctx := context.Background()
	as := NewAdminStorage(DB)
	s := NewMapStorage(DB)
	tree := createInitializedMapForTests(ctx, t, s, as)

	revision := int64(5)
	root := MustSignMapRoot(&types.MapRootV1{
		TimestampNanos: 98765,
		Revision:       uint64(revision),
		RootHash:       []byte(dummyHash),
	})
	runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
		if err := tx.StoreSignedMapRoot(ctx, root); err != nil {
			t.Fatalf("Failed to store signed root: %v", err)
		}
		return nil
	})

	{
		runMapTX(ctx, s, tree, t, func(ctx context.Context, tx2 storage.MapTreeTX) error {
			root2, err := tx2.GetSignedMapRoot(ctx, revision)
			if err != nil {
				t.Fatalf("Failed to get back new map root: %v", err)
			}
			if !proto.Equal(root, root2) {
				t.Fatalf("Getting root round trip failed: <%#v> and: <%#v>", root, root2)
			}
			return nil
		})
	}
}

func TestLatestSignedMapRoot(t *testing.T) {
	testdb.SkipIfNoMySQL(t)

	cleanTestDB(DB)
	ctx := context.Background()
	as := NewAdminStorage(DB)
	s := NewMapStorage(DB)
	tree := createInitializedMapForTests(ctx, t, s, as)

	root := MustSignMapRoot(&types.MapRootV1{
		TimestampNanos: 98765,
		Revision:       5,
		RootHash:       []byte(dummyHash),
	})
	runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
		if err := tx.StoreSignedMapRoot(ctx, root); err != nil {
			t.Fatalf("Failed to store signed root: %v", err)
		}
		return nil
	})

	{
		runMapTX(ctx, s, tree, t, func(ctx context.Context, tx2 storage.MapTreeTX) error {
			root2, err := tx2.LatestSignedMapRoot(ctx)
			if err != nil {
				t.Fatalf("Failed to read back new map root: %v", err)
			}
			if !proto.Equal(root, root2) {
				t.Fatalf("Root round trip failed: <%#v> and: <%#v>", root, root2)
			}
			return nil
		})
	}
}

func TestDuplicateSignedMapRoot(t *testing.T) {
	testdb.SkipIfNoMySQL(t)

	cleanTestDB(DB)
	ctx := context.Background()
	as := NewAdminStorage(DB)
	s := NewMapStorage(DB)
	tree := createInitializedMapForTests(ctx, t, s, as)

	runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
		root := MustSignMapRoot(&types.MapRootV1{
			TimestampNanos: 98765,
			Revision:       5,
			RootHash:       []byte(dummyHash),
		})
		if err := tx.StoreSignedMapRoot(ctx, root); err != nil {
			t.Fatalf("Failed to store signed map root: %v", err)
		}
		// Shouldn't be able to do it again
		if err := tx.StoreSignedMapRoot(ctx, root); err == nil {
			t.Fatal("Allowed duplicate signed map root")
		}
		return nil
	})
}

func TestReadOnlyMapTX_Rollback(t *testing.T) {
	testdb.SkipIfNoMySQL(t)
	ctx := context.Background()

	cleanTestDB(DB)
	as := NewAdminStorage(DB)
	s := NewMapStorage(DB)
	activeMap := createInitializedMapForTests(ctx, t, s, as)
	tx, err := s.SnapshotForTree(ctx, activeMap)
	if err != nil {
		t.Fatalf("Snapshot() = (_, %v), want = (_, nil)", err)
	}
	defer tx.Close()
	// It's a bit hard to have a more meaningful test. This should suffice.
	if err := tx.Rollback(); err != nil {
		t.Errorf("Rollback() = (_, %v), want = (_, nil)", err)
	}
}

func runMapTX(ctx context.Context, s storage.MapStorage, tree *trillian.Tree, t *testing.T, f storage.MapTXFunc) {
	if err := s.ReadWriteTransaction(ctx, tree, f); err != nil {
		t.Fatalf("Failed to begin map tx: %v", err)
	}
}

func createInitializedMapForTests(ctx context.Context, t *testing.T, s storage.MapStorage, as storage.AdminStorage) *trillian.Tree {
	t.Helper()
	tree := mustCreateTree(ctx, t, as, storageto.MapTree)

	signer := tcrypto.NewSigner(tree.TreeId, testonly.NewSignerWithFixedSig(nil, []byte("sig")), crypto.SHA256)
	err := s.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.MapTreeTX) error {
		initialRoot, _ := signer.SignMapRoot(&types.MapRootV1{
			RootHash: []byte("rootHash"),
			Revision: 0,
		})

		if err := tx.StoreSignedMapRoot(ctx, initialRoot); err != nil {
			t.Fatalf("Failed to StoreSignedMapRoot: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ReadWriteTransaction() = %v", err)
	}

	return tree
}

func mapTree(mapID int64) *trillian.Tree {
	return &trillian.Tree{
		TreeId:       mapID,
		TreeType:     trillian.TreeType_MAP,
		HashStrategy: trillian.HashStrategy_TEST_MAP_HASHER,
	}
}
