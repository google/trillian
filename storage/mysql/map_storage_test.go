// Copyright 2016 Google LLC. All Rights Reserved.
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
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian"
	"github.com/google/trillian/integration/storagetest"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/testdb"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/types"

	tcrypto "github.com/google/trillian/crypto"
	storageto "github.com/google/trillian/storage/testonly"
	stree "github.com/google/trillian/storage/tree"
)

var fixedSigner = tcrypto.NewSigner(0, testonly.NewSignerWithFixedSig(nil, []byte("notempty")), crypto.SHA256)

func TestMapIntegration(t *testing.T) {
	ctx := context.Background()
	testdb.SkipIfNoMySQL(t)
	db, cleanup := openTestDBOrDie()
	defer db.Close()
	defer cleanup(ctx)

	storageFactory := func(context.Context, *testing.T) (storage.MapStorage, storage.AdminStorage) {
		cleanTestDB(db)
		return NewMapStorage(db), NewAdminStorage(db)
	}

	storagetest.RunMapStorageTests(t, storageFactory)
}

func MustSignMapRoot(t *testing.T, root *types.MapRootV1) *trillian.SignedMapRoot {
	t.Helper()
	r, err := fixedSigner.SignMapRoot(root)
	if err != nil {
		t.Fatalf("SignMapRoot(): %v", err)
	}
	return r
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
			wantErr:     true,
			wantRootErr: "tree must be pulled from storage",
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

func TestMapRootUpdate(t *testing.T) {
	testdb.SkipIfNoMySQL(t)

	cleanTestDB(DB)
	ctx := context.Background()
	as := NewAdminStorage(DB)
	s := NewMapStorage(DB)
	tree := createInitializedMapForTests(ctx, t, s, as)

	populatedMetadata := []byte("d47a")

	for _, tc := range []struct {
		desc         string
		root         *trillian.SignedMapRoot
		wantMetadata []byte
	}{
		{
			desc: "Initial root",
			root: MustSignMapRoot(t, &types.MapRootV1{
				TimestampNanos: 98765,
				Revision:       5,
				RootHash:       []byte(dummyHash),
			}),
		},
		{
			desc: "Root update",
			root: MustSignMapRoot(t, &types.MapRootV1{
				TimestampNanos: 98766,
				Revision:       6,
				RootHash:       []byte(dummyHash),
			}),
		},
		{
			desc: "Root with default (empty) MapperMetadata",
			root: MustSignMapRoot(t, &types.MapRootV1{
				TimestampNanos: 98768,
				Revision:       7,
				RootHash:       []byte(dummyHash),
			}),
		},
		{
			desc: "Root with non-default (populated) MapperMetadata",
			root: MustSignMapRoot(t, &types.MapRootV1{
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
					t.Errorf("%v: LatestSignedMapRoot() diff(-got, +want) \n%v", tc.desc, cmp.Diff(got, want))
				}
				return nil
			})
		}()
	}
}

var (
	keyHash = []byte([]byte("A Key Hash"))
	mapLeaf = &trillian.MapLeaf{
		Index:     keyHash,
		LeafHash:  []byte("A Hash"),
		LeafValue: []byte("A Value"),
		ExtraData: []byte("Some Extra Data"),
	}
)

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
		{index: nil}, // empty list.
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

func TestMapMultiRevisionFetchBigBatch(t *testing.T) {
	t.Skip("Known Issue: https://github.com/google/trillian/issues/1845")
	testdb.SkipIfNoMySQL(t)

	cleanTestDB(DB)
	ctx := context.Background()
	as := NewAdminStorage(DB)
	s := NewMapStorage(DB)
	tree := createInitializedMapForTests(ctx, t, s, as)

	nodes1 := createMapNodes(1002)
	nodeIDs := make([]stree.NodeID, len(nodes1))
	for i := range nodes1 {
		nodeIDs[i] = nodes1[i].NodeID
	}

	runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
		mapTX := tx.(*mapTreeTX)
		mapTX.treeTX.writeRevision = 1
		// Need to read nodes before attempting to write.
		if _, err := tx.GetMerkleNodes(ctx, 0, nodeIDs); err != nil {
			t.Fatalf("Failed to read nodes: %s", err)
		}
		if err := tx.SetMerkleNodes(ctx, nodes1); err != nil {
			t.Fatalf("Failed to store nodes: %s", err)
		}
		return nil
	})

	nodes2 := make([]stree.Node, len(nodes1))
	delta := make([]stree.Node, len(nodes1)/2)
	for i := range nodes1 {
		nodes2[i] = nodes1[i]
		if i%2 == 0 && i/2 < cap(delta) {
			h := sha256.Sum256([]byte{byte(42 + i*i)})
			nodes2[i].Hash = h[:]
			delta[i/2] = nodes2[i]
		}
	}

	runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
		mapTX := tx.(*mapTreeTX)
		mapTX.treeTX.writeRevision = 2
		// Need to read nodes before attempting to write.
		if _, err := tx.GetMerkleNodes(ctx, 1, nodeIDs); err != nil {
			t.Fatalf("Failed to read nodes: %s", err)
		}
		if err := tx.SetMerkleNodes(ctx, delta); err != nil {
			t.Fatalf("Failed to store nodes: %s", err)
		}
		return nil
	})

	runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
		readNodes, err := tx.GetMerkleNodes(ctx, 1, nodeIDs)
		if err != nil {
			t.Fatalf("Failed to retrieve nodes: %s", err)
		}
		if err := nodesAreEqual(readNodes, nodes1); err != nil {
			t.Fatalf("Read back different nodes from the ones stored: %s", err)
		}
		return nil
	})

	runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
		readNodes, err := tx.GetMerkleNodes(ctx, 2, nodeIDs)
		if err != nil {
			t.Fatalf("Failed to retrieve nodes: %s", err)
		}
		if err := nodesAreEqual(readNodes, nodes2); err != nil {
			t.Fatalf("Read back different nodes from the ones stored: %s", err)
		}
		return nil
	})
}

func TestMapSetGetBigBatch(t *testing.T) {
	testdb.SkipIfNoMySQL(t)

	cleanTestDB(DB)
	ctx := context.Background()
	as := NewAdminStorage(DB)
	s := NewMapStorage(DB)
	tree := createInitializedMapForTests(ctx, t, s, as)

	leaves := make([]*trillian.MapLeaf, 1001)
	keys := make([][]byte, cap(leaves))
	for i := 0; i < cap(leaves); i++ {
		bs := []byte(fmt.Sprintf("leaf-%08d", i))
		keys[i] = bs
		leaves[i] = &trillian.MapLeaf{Index: bs, LeafHash: bs, LeafValue: bs, ExtraData: bs}
	}

	runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
		mapTX := tx.(*mapTreeTX)
		mapTX.treeTX.writeRevision = 1
		for _, l := range leaves {
			if err := tx.Set(ctx, l.Index, l); err != nil {
				t.Fatalf("Failed to set %v to %v: %v", l.Index, l, err)
			}
		}
		return nil
	})

	runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
		mapTX := tx.(*mapTreeTX)
		mapTX.treeTX.writeRevision = 2
		for i, l := range leaves {
			if i%2 == 0 {
				u := &trillian.MapLeaf{Index: l.Index, LeafHash: []byte{}, LeafValue: []byte{}, ExtraData: []byte{}}
				if err := tx.Set(ctx, l.Index, u); err != nil {
					t.Fatalf("Failed to set %v to %v: %v", l.Index, l, err)
				}
			}
		}
		return nil
	})

	runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
		ls, err := tx.Get(ctx, 1, keys)
		if err != nil {
			return err
		}
		for i := 0; i < len(leaves); i++ {
			if got, want := ls[i], leaves[i]; !proto.Equal(got, want) {
				t.Fatalf("Read != written leaf: got %v, want %v", got, want)
			}
		}
		return nil
	})

	runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
		ls, err := tx.Get(ctx, 2, keys)
		if err != nil {
			return err
		}
		for i := 0; i < len(leaves); i++ {
			var want *trillian.MapLeaf
			if i%2 == 0 {
				want = &trillian.MapLeaf{Index: leaves[i].Index, LeafHash: []byte{}, LeafValue: []byte{}, ExtraData: []byte{}}
			} else {
				want = leaves[i]
			}
			if got := ls[i]; !proto.Equal(got, want) {
				t.Fatalf("Read != written leaf: got %v, want %v", got, want)
			}
		}
		return nil
	})
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
	root := MustSignMapRoot(t, &types.MapRootV1{
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

func TestDuplicateSignedMapRoot(t *testing.T) {
	testdb.SkipIfNoMySQL(t)

	cleanTestDB(DB)
	ctx := context.Background()
	as := NewAdminStorage(DB)
	s := NewMapStorage(DB)
	tree := createInitializedMapForTests(ctx, t, s, as)

	runMapTX(ctx, s, tree, t, func(ctx context.Context, tx storage.MapTreeTX) error {
		root := MustSignMapRoot(t, &types.MapRootV1{
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

func createMapNodes(count int) []stree.Node {
	r := make([]stree.Node, count)
	bs := make([]byte, 32) // Default value of 0 index in 2^256 keyspace.

	const bottomStrataIdx = 9

	for i := range r {
		bt := make([]byte, len(bs))
		copy(bt, bs)
		r[i].NodeID = stree.NewNodeIDFromHash(bt)
		h := sha256.Sum256(bs)
		r[i].Hash = h[:]
		glog.V(3).Infof("Node to store: %v\n", r[i].NodeID)

		needsIncrement := true
		for i := bottomStrataIdx; needsIncrement && i >= 0; i-- {
			bs[i]++
			needsIncrement = bs[i] == 0 // Keep incrementing if we overflowed this byte.
		}
		if needsIncrement {
			panic(fmt.Sprintf("ran out of subtrees to create %d nodes", count))
		}
	}
	return r
}
