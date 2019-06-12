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

package maptest

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/golang/glog"
	"github.com/kylelemons/godebug/pretty"

	"github.com/golang/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/trillian"
	"github.com/google/trillian/client"
	"github.com/google/trillian/examples/ct/ctmapper/ctmapperpb"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/types"

	stestonly "github.com/google/trillian/storage/testonly"
)

// NamedTestFn is a binding between a readable test name (used for a Go subtest) and a function
// that performs the test, given a Trillian Admin and Map client.
type NamedTestFn struct {
	Name string
	Fn   func(context.Context, *testing.T, trillian.TrillianAdminClient, trillian.TrillianMapClient)
}

// TestTable is a collection of NamedTestFns.
type TestTable []NamedTestFn

// AllTests is the TestTable containing all the trillian Map integration tests.
// Be sure to extend this when additional tests are added.
// This is done so that tests can be run in different environments in a portable way.
var AllTests = TestTable{
	{"MapRevisionZero", RunMapRevisionZero},
	{"MapRevisionInvalid", RunMapRevisionInvalid},
	{"SetLeavesRevision", RunSetLeavesRevision},
	{"LeafHistory", RunLeafHistory},
	{"Inclusion", RunInclusion},
	{"InclusionBatch", RunInclusionBatch},
	{"RunGetLeafByRevisionNoProof", RunGetLeafByRevisionNoProof},
}

var (
	h2b    = testonly.MustHexDecode
	index0 = h2b("0000000000000000000000000000000000000000000000000000000000000000")
	index1 = h2b("0000000000000000000000000000000000000000000000000000000000000001")
	index2 = h2b("0000000000000000000000000000000000000000000000000000000000000002")
	index3 = h2b("0000000000000000000000000000000000000000000000000000000000000003")
	indexF = h2b("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF")
)

// createBatchLeaves produces n unique map leaves.
func createBatchLeaves(batch, n int) []*trillian.MapLeaf {
	leaves := make([]*trillian.MapLeaf, 0, n)
	for i := 0; i < n; i++ {
		leaves = append(leaves, &trillian.MapLeaf{
			Index:     testonly.TransparentHash(fmt.Sprintf("batch-%d-key-%d", batch, i)),
			LeafValue: []byte(fmt.Sprintf("batch-%d-value-%d", batch, i)),
		})
	}
	return leaves
}

func isEmptyMap(ctx context.Context, tmap trillian.TrillianMapClient, tree *trillian.Tree) error {
	r, err := tmap.GetSignedMapRoot(ctx, &trillian.GetSignedMapRootRequest{MapId: tree.TreeId})
	if err != nil {
		return fmt.Errorf("failed to get empty map head: %v", err)
	}

	var mapRoot types.MapRootV1
	if err := mapRoot.UnmarshalBinary(r.GetMapRoot().GetMapRoot()); err != nil {
		return err
	}

	if got, want := mapRoot.Revision, uint64(0); got != want {
		return fmt.Errorf("got SMR with revision %d, want %d", got, want)
	}
	return nil
}

func verifyGetSignedMapRootResponse(mapVerifier *client.MapVerifier, mapRoot *trillian.SignedMapRoot, wantRevision int64) error {
	root, err := mapVerifier.VerifySignedMapRoot(mapRoot)
	if err != nil {
		return err
	}
	if got, want := int64(root.Revision), wantRevision; got != want {
		return fmt.Errorf("got SMR with revision %d, want %d", got, want)
	}
	return nil
}

func verifyGetMapLeavesResponse(mapVerifier *client.MapVerifier, getResp *trillian.GetMapLeavesResponse, indexes [][]byte,
	wantRevision int64) error {
	if got, want := len(getResp.GetMapLeafInclusion()), len(indexes); got != want {
		return fmt.Errorf("got %d values, want %d", got, want)
	}
	if err := verifyGetSignedMapRootResponse(mapVerifier, getResp.GetMapRoot(), wantRevision); err != nil {
		return err
	}
	for _, incl := range getResp.GetMapLeafInclusion() {
		value := incl.GetLeaf().GetLeafValue()
		index := incl.GetLeaf().GetIndex()
		leafHash := incl.GetLeaf().GetLeafHash()

		wantLeafHash := mapVerifier.Hasher.HashLeaf(mapVerifier.MapID, index, value)
		if !bytes.Equal(leafHash, wantLeafHash) {
			if len(value) == 0 {
				// The leaf value is empty; if this is because it has never been set then its
				// hash value is nominally HashEmpty(index, 0), which is represented as nil
				// on the API.
				if len(leafHash) != 0 {
					return fmt.Errorf("leaf.LeafHash for %s = %x, want %x or nil", value, leafHash, wantLeafHash)
				}
			} else {
				return fmt.Errorf("leaf.LeafHash for %s = %x, want %x", value, leafHash, wantLeafHash)
			}
		}
		if err := mapVerifier.VerifyMapLeafInclusion(getResp.GetMapRoot(), incl); err != nil {
			return fmt.Errorf("VerifyMapLeafInclusion(%x): %v", index, err)
		}
	}
	return nil
}

// newTreeWithHasher is a test setup helper for creating new trees with a given hasher.
func newTreeWithHasher(ctx context.Context, tadmin trillian.TrillianAdminClient, tmap trillian.TrillianMapClient, hashStrategy trillian.HashStrategy) (*trillian.Tree, error) {
	treeParams := stestonly.MapTree
	treeParams.HashStrategy = hashStrategy
	tree, err := tadmin.CreateTree(ctx, &trillian.CreateTreeRequest{Tree: treeParams})
	if err != nil {
		return nil, err
	}

	if err := client.InitMap(ctx, tree, tmap); err != nil {
		return nil, err
	}

	return tree, nil
}

type hashStrategyAndRoot struct {
	hashStrategy trillian.HashStrategy
	wantRoot     []byte
}

// RunMapRevisionZero performs checks on Trillian Map behavior for new, empty maps.
func RunMapRevisionZero(ctx context.Context, t *testing.T, tadmin trillian.TrillianAdminClient, tmap trillian.TrillianMapClient) {
	for _, tc := range []struct {
		desc         string
		hashStrategy []hashStrategyAndRoot
		wantRev      int64
	}{
		{
			desc: "empty map has SMR at rev 0 but not rev 1",
			hashStrategy: []hashStrategyAndRoot{
				{trillian.HashStrategy_TEST_MAP_HASHER, testonly.MustDecodeBase64("xmifEIEqCYCXbZUz2Dh1KCFmFZVn7DUVVxbBQTr1PWo=")},
				{trillian.HashStrategy_CONIKS_SHA512_256, nil /* TODO: need to fix the treeID to have a known answer */},
				{trillian.HashStrategy_CONIKS_SHA256, nil /* TODO: need to fix the treeID to have a known answer */},
			},
			wantRev: 0,
		},
	} {
		for _, hsr := range tc.hashStrategy {
			t.Run(fmt.Sprintf("%v/%v", tc.desc, hsr.hashStrategy), func(t *testing.T) {
				tree, err := newTreeWithHasher(ctx, tadmin, tmap, hsr.hashStrategy)
				if err != nil {
					t.Fatalf("newTreeWithHasher(%v): %v", hsr.hashStrategy, err)
				}
				mapVerifier, err := client.NewMapVerifierFromTree(tree)
				if err != nil {
					t.Fatalf("NewMapVerifierFromTree(): %v", err)
				}

				getSmrResp, err := tmap.GetSignedMapRoot(ctx, &trillian.GetSignedMapRootRequest{MapId: tree.TreeId})
				if err != nil {
					t.Fatalf("GetSignedMapRoot(): %v", err)
				}
				if err := verifyGetSignedMapRootResponse(mapVerifier, getSmrResp.GetMapRoot(), tc.wantRev); err != nil {
					t.Errorf("verifyGetSignedMapRootResponse(rev %v): %v", tc.wantRev, err)
				}

				getSmrByRevResp, err := tmap.GetSignedMapRootByRevision(ctx, &trillian.GetSignedMapRootByRevisionRequest{
					MapId:    tree.TreeId,
					Revision: 0,
				})
				if err != nil {
					t.Errorf("GetSignedMapRootByRevision(): %v", err)
				}
				if err := verifyGetSignedMapRootResponse(mapVerifier, getSmrByRevResp.GetMapRoot(), tc.wantRev); err != nil {
					t.Errorf("verifyGetSignedMapRootResponse(rev %v): %v", tc.wantRev, err)
				}

				got, want := getSmrByRevResp.GetMapRoot(), getSmrResp.GetMapRoot()
				if diff := pretty.Compare(got, want); diff != "" {
					t.Errorf("GetSignedMapRootByRevision() != GetSignedMapRoot(); diff (-got +want):\n%v", diff)
				}

				if _, err = tmap.GetSignedMapRootByRevision(ctx, &trillian.GetSignedMapRootByRevisionRequest{
					MapId:    tree.TreeId,
					Revision: 1,
				}); err == nil {
					t.Errorf("GetSignedMapRootByRevision(rev: 1) err? false want? true")
				}
				// TODO(phad): ideally we'd inspect err's type and check it contains a NOT_FOUND Code (5), but I don't want
				// a dependency on gRPC here.
			})
		}
	}
}

// RunMapRevisionInvalid performs checks on Map APIs where revision takes illegal values.
func RunMapRevisionInvalid(ctx context.Context, t *testing.T, tadmin trillian.TrillianAdminClient, tmap trillian.TrillianMapClient) {
	for _, tc := range []struct {
		desc         string
		HashStrategy []trillian.HashStrategy
		set          [][]*trillian.MapLeaf
		get          []struct {
			index    []byte
			revision int64
			wantErr  bool
		}
	}{
		{
			desc:         "single leaf update",
			HashStrategy: []trillian.HashStrategy{trillian.HashStrategy_TEST_MAP_HASHER, trillian.HashStrategy_CONIKS_SHA512_256, trillian.HashStrategy_CONIKS_SHA256},
			set: [][]*trillian.MapLeaf{
				{}, // Advance revision without changing anything.
				{{Index: index1, LeafValue: []byte("A")}},
			},
			get: []struct {
				index    []byte
				revision int64
				wantErr  bool
			}{
				{index: index1, revision: -1, wantErr: true},
				{index: index1, revision: 0, wantErr: false},
			},
		},
	} {
		for _, hashStrategy := range tc.HashStrategy {
			t.Run(fmt.Sprintf("%v/%v", tc.desc, hashStrategy), func(t *testing.T) {
				tree, err := newTreeWithHasher(ctx, tadmin, tmap, hashStrategy)
				if err != nil {
					t.Fatalf("newTreeWithHasher(%v): %v", hashStrategy, err)
				}
				for _, batch := range tc.set {
					if _, err := tmap.SetLeaves(ctx, &trillian.SetMapLeavesRequest{
						MapId:  tree.TreeId,
						Leaves: batch,
					}); err != nil {
						t.Fatalf("SetLeaves(): %v", err)
					}
				}

				for _, batch := range tc.get {
					_, err := tmap.GetLeavesByRevision(ctx, &trillian.GetMapLeavesByRevisionRequest{
						MapId:    tree.TreeId,
						Index:    [][]byte{batch.index},
						Revision: batch.revision,
					})
					if gotErr := err != nil; gotErr != batch.wantErr {
						t.Errorf("GetLeavesByRevision(rev: %d)=_, err? %t want? %t (err=%v)", batch.revision, gotErr, batch.wantErr, err)
					}
				}
			})
		}
	}
}

// RunSetLeavesRevision checks Map SetLeaves API with revision parameter used.
// TODO(pavelkalinnikov): Merge RunMapRevisionInvalid into this test.
func RunSetLeavesRevision(ctx context.Context, t *testing.T, tadmin trillian.TrillianAdminClient, tmap trillian.TrillianMapClient) {
	type batch struct {
		leaves  []*trillian.MapLeaf
		rev     int64
		wantErr bool
	}
	for _, tc := range []struct {
		desc string
		sets []batch
		gets []batch
	}{
		{
			desc: "no_revision",
			sets: []batch{
				{}, // Advance revision without changing anything.
				{leaves: []*trillian.MapLeaf{{Index: index1, LeafValue: []byte("A")}}},
			},
			gets: []batch{
				{leaves: []*trillian.MapLeaf{{Index: index1, LeafValue: nil}}, rev: 0},
				{leaves: []*trillian.MapLeaf{{Index: index1, LeafValue: nil}}, rev: 1},
				{leaves: []*trillian.MapLeaf{{Index: index1, LeafValue: []byte("A")}}, rev: 2},
				{leaves: []*trillian.MapLeaf{{Index: index1}}, rev: 10, wantErr: true},
			},
		},
		{
			desc: "use_revision",
			sets: []batch{
				{leaves: []*trillian.MapLeaf{{Index: index1, LeafValue: []byte("A")}}, rev: 1},
				{leaves: []*trillian.MapLeaf{{Index: index1, LeafValue: []byte("B")}}, rev: 1, wantErr: true},
				{rev: 2}, // Advance revision without changing anything.
				{
					leaves: []*trillian.MapLeaf{
						{Index: index0, LeafValue: []byte("X")},
						{Index: indexF, LeafValue: []byte("Y")},
					},
					rev: 3,
				},
				{leaves: []*trillian.MapLeaf{{Index: indexF, LeafValue: []byte("O")}}, rev: 1, wantErr: true},
				{leaves: []*trillian.MapLeaf{{Index: index1, LeafValue: []byte("S")}}, rev: 10, wantErr: true},
			},
			gets: []batch{
				{leaves: []*trillian.MapLeaf{{Index: index1, LeafValue: nil}}, rev: 0},
				{leaves: []*trillian.MapLeaf{{Index: index1, LeafValue: []byte("A")}}, rev: 1},
				{leaves: []*trillian.MapLeaf{{Index: index1, LeafValue: []byte("A")}}, rev: 2},
				{leaves: []*trillian.MapLeaf{{Index: index1, LeafValue: []byte("A")}}, rev: 3},
				{leaves: []*trillian.MapLeaf{{Index: index1}}, rev: 4, wantErr: true},
				{leaves: []*trillian.MapLeaf{{Index: index1}}, rev: 10, wantErr: true},
				{leaves: []*trillian.MapLeaf{{Index: indexF, LeafValue: []byte("Y")}, {Index: index0, LeafValue: []byte("X")}}, rev: 3},
			},
		},
		{
			desc: "use_revision_and_no_revision",
			sets: []batch{
				{leaves: []*trillian.MapLeaf{{Index: index1, LeafValue: []byte("A")}}, rev: 1},
				{leaves: []*trillian.MapLeaf{{Index: index2, LeafValue: []byte("B")}}},
				{leaves: []*trillian.MapLeaf{{Index: index3, LeafValue: []byte("C")}}, rev: 3},
				{leaves: []*trillian.MapLeaf{{Index: index2, LeafValue: []byte("BB")}}},
			},
			gets: []batch{
				{
					leaves: []*trillian.MapLeaf{
						{Index: index1, LeafValue: []byte("A")},
						{Index: index2, LeafValue: []byte("B")},
						{Index: index3, LeafValue: nil},
					},
					rev: 2,
				},
				{
					leaves: []*trillian.MapLeaf{
						{Index: index1, LeafValue: []byte("A")},
						{Index: index2, LeafValue: []byte("BB")},
						{Index: index3, LeafValue: []byte("C")},
					},
					rev: 4,
				},
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			tree, err := newTreeWithHasher(ctx, tadmin, tmap, trillian.HashStrategy_CONIKS_SHA256)
			if err != nil {
				t.Fatalf("newTreeWithHasher: %v", err)
			}
			for _, b := range tc.sets {
				_, err := tmap.SetLeaves(ctx, &trillian.SetMapLeavesRequest{
					MapId:    tree.TreeId,
					Leaves:   b.leaves,
					Revision: b.rev,
				})
				if got, want := err != nil, b.wantErr; got != want {
					t.Errorf("SetLeaves(%+v): %v, wantErr=%v", b, err, want)
				}
			}

			for _, b := range tc.gets {
				indices := make([][]byte, len(b.leaves))
				for i, leaf := range b.leaves {
					indices[i] = leaf.Index
				}
				rsp, err := tmap.GetLeavesByRevision(ctx, &trillian.GetMapLeavesByRevisionRequest{
					MapId:    tree.TreeId,
					Index:    indices,
					Revision: b.rev,
				})
				if got, want := err != nil, b.wantErr; got != want {
					t.Errorf("GetLeavesByRevision(%d): %v, wantErr=%v", b.rev, err, want)
				}
				if err != nil {
					continue
				}
				if got, want := len(rsp.MapLeafInclusion), len(b.leaves); got != want {
					t.Fatalf("GetLeavesByRevision(%d) returned %d leaves, want %d", b.rev, got, want)
				}
				for i, leaf := range b.leaves {
					got := rsp.MapLeafInclusion[i].Leaf
					if !bytes.Equal(got.LeafValue, leaf.LeafValue) {
						t.Errorf("Got leaf %+v, want %+v", got, leaf)
					}
				}
			}
		})
	}
}

// RunLeafHistory performs checks on Trillian Map leaf updates under a variety of Hash Strategies.
func RunLeafHistory(ctx context.Context, t *testing.T, tadmin trillian.TrillianAdminClient, tmap trillian.TrillianMapClient) {
	for _, tc := range []struct {
		desc         string
		HashStrategy []trillian.HashStrategy
		set          [][]*trillian.MapLeaf
		get          []struct {
			revision  int64
			Index     []byte
			LeafValue []byte
		}
	}{
		{
			desc:         "single leaf update",
			HashStrategy: []trillian.HashStrategy{trillian.HashStrategy_TEST_MAP_HASHER, trillian.HashStrategy_CONIKS_SHA512_256, trillian.HashStrategy_CONIKS_SHA256},
			set: [][]*trillian.MapLeaf{
				{}, // Advance revision without changing anything.
				{
					{Index: index0, LeafValue: []byte("A")},
				},
				{}, // Advance revision without changing anything.
				{
					{Index: index0, LeafValue: []byte("B")},
				},
				{
					{Index: index0, LeafValue: []byte("C")},
				},
			},
			get: []struct {
				revision  int64
				Index     []byte
				LeafValue []byte
			}{
				{revision: 1, Index: index0, LeafValue: nil},                                     // Empty to empty root.
				{revision: 2, Index: []byte("doesnotexist...................."), LeafValue: nil}, // Empty to first root, through empty branch.
				{revision: 2, Index: index0, LeafValue: []byte("A")},                             // Value to first root.
				{revision: 3, Index: index0, LeafValue: []byte("A")},
				{revision: 4, Index: index0, LeafValue: []byte("B")},
				{revision: 5, Index: index0, LeafValue: []byte("C")},
			},
		},
	} {
		for _, hashStrategy := range tc.HashStrategy {
			t.Run(fmt.Sprintf("%v/%v", tc.desc, hashStrategy), func(t *testing.T) {
				tree, err := newTreeWithHasher(ctx, tadmin, tmap, hashStrategy)
				if err != nil {
					t.Fatalf("newTreeWithHasher(%v): %v", hashStrategy, err)
				}
				mapVerifier, err := client.NewMapVerifierFromTree(tree)
				if err != nil {
					t.Fatalf("NewMapVerifierFromTree(): %v", err)
				}

				for _, batch := range tc.set {
					_, err := tmap.SetLeaves(ctx, &trillian.SetMapLeavesRequest{
						MapId:  tree.TreeId,
						Leaves: batch,
					})
					if err != nil {
						t.Fatalf("SetLeaves(): %v", err)
					}
				}

				for _, batch := range tc.get {
					indexes := [][]byte{batch.Index}
					getResp, err := tmap.GetLeavesByRevision(ctx, &trillian.GetMapLeavesByRevisionRequest{
						MapId:    tree.TreeId,
						Index:    indexes,
						Revision: batch.revision,
					})
					if err != nil {
						t.Errorf("GetLeavesByRevision(rev: %d)=_, err %v want nil", batch.revision, err)
						continue
					}

					if got, want := len(getResp.GetMapLeafInclusion()), 1; got < want {
						t.Errorf("GetLeavesByRevision(rev: %v).len: %v, want >= %v", batch.revision, got, want)
					}
					if got, want := getResp.GetMapLeafInclusion()[0].GetLeaf().GetLeafValue(), batch.LeafValue; !bytes.Equal(got, want) {
						t.Errorf("GetLeavesByRevision(rev: %v).LeafValue: %s, want %s", batch.revision, got, want)
					}

					if err := verifyGetMapLeavesResponse(mapVerifier, getResp, indexes, int64(batch.revision)); err != nil {
						t.Errorf("verifyGetMapLeavesResponse(rev %v): %v", batch.revision, err)
					}
				}
			})
		}
	}
}

// RunInclusion performs checks on Trillian Map inclusion proofs after setting and getting leafs,
// for a variety of hash strategies.
func RunInclusion(ctx context.Context, t *testing.T, tadmin trillian.TrillianAdminClient, tmap trillian.TrillianMapClient) {
	for _, tc := range []struct {
		desc         string
		HashStrategy []trillian.HashStrategy
		leaves       []*trillian.MapLeaf
	}{
		{
			desc:         "single",
			HashStrategy: []trillian.HashStrategy{trillian.HashStrategy_TEST_MAP_HASHER, trillian.HashStrategy_CONIKS_SHA512_256, trillian.HashStrategy_CONIKS_SHA256},
			leaves: []*trillian.MapLeaf{
				{Index: index0, LeafValue: []byte("A")},
			},
		},
		{
			desc:         "multi",
			HashStrategy: []trillian.HashStrategy{trillian.HashStrategy_TEST_MAP_HASHER, trillian.HashStrategy_CONIKS_SHA512_256, trillian.HashStrategy_CONIKS_SHA256},
			leaves: []*trillian.MapLeaf{
				{Index: index0, LeafValue: []byte("A")},
				{Index: index1, LeafValue: []byte("B")},
				{Index: index2, LeafValue: []byte("C")},
				{Index: index3, LeafValue: nil},
			},
		},
		{
			desc:         "across subtrees",
			HashStrategy: []trillian.HashStrategy{trillian.HashStrategy_TEST_MAP_HASHER, trillian.HashStrategy_CONIKS_SHA512_256, trillian.HashStrategy_CONIKS_SHA256},
			leaves: []*trillian.MapLeaf{
				{Index: index0, LeafValue: []byte("Z")},
			},
		},
	} {
		for _, hashStrategy := range tc.HashStrategy {
			t.Run(fmt.Sprintf("%v/%v", tc.desc, hashStrategy), func(t *testing.T) {
				tree, err := newTreeWithHasher(ctx, tadmin, tmap, hashStrategy)
				if err != nil {
					t.Fatalf("newTreeWithHasher(%v): %v", hashStrategy, err)
				}
				mapVerifier, err := client.NewMapVerifierFromTree(tree)
				if err != nil {
					t.Fatalf("NewMapVerifierFromTree(): %v", err)
				}

				if _, err := tmap.SetLeaves(ctx, &trillian.SetMapLeavesRequest{
					MapId:  tree.TreeId,
					Leaves: tc.leaves,
					Metadata: testonly.MustMarshalAnyNoT(&ctmapperpb.MapperMetadata{
						HighestFullyCompletedSeq: 0xcafe,
					}),
				}); err != nil {
					t.Fatalf("SetLeaves(): %v", err)
				}

				indexes := [][]byte{}
				for _, l := range tc.leaves {
					indexes = append(indexes, l.Index)
				}
				getResp, err := tmap.GetLeaves(ctx, &trillian.GetMapLeavesRequest{
					MapId: tree.TreeId,
					Index: indexes,
				})
				if err != nil {
					t.Fatalf("GetLeaves(): %v", err)
				}

				if err := verifyGetMapLeavesResponse(mapVerifier, getResp, indexes, 1); err != nil {
					t.Errorf("verifyGetMapLeavesResponse(): %v", err)
				}
			})
		}
	}
}

func RunGetLeafByRevisionNoProof(ctx context.Context, t *testing.T, tadmin trillian.TrillianAdminClient, tmap trillian.TrillianMapClient) {
	tree, err := newTreeWithHasher(ctx, tadmin, tmap, trillian.HashStrategy_TEST_MAP_HASHER)
	if err != nil {
		t.Fatalf("newTreeWithHasher(): %v", err)
	}
	batchSize := 10
	numBatches := 3

	leafMap := writeBatch(ctx, t, tmap, tree, batchSize, numBatches)
	indexes := make([][]byte, 0, len(leafMap))
	leaves := make([]*trillian.MapLeaf, 0, len(leafMap))
	for i, l := range leafMap {
		indexes = append(indexes, []byte(i))
		leaves = append(leaves, l)
	}

	getResp, err := tmap.GetLeavesByRevisionNoProof(ctx, &trillian.GetMapLeavesByRevisionRequest{
		MapId:    tree.TreeId,
		Index:    indexes,
		Revision: int64(numBatches),
	})
	if err != nil {
		t.Fatalf("GetLeavesByRevisionNoProof(): %v", err)
	}

	if got, want := len(getResp.Leaves), len(indexes); got != want {
		t.Errorf("len: %v, want %v", got, want)
	}

	// Remove LeafHash because SetLeaves does not supply it.
	for _, l := range getResp.Leaves {
		l.LeafHash = nil
	}

	if got, want := getResp.Leaves, leaves; !cmp.Equal(got, want,
		cmp.Comparer(proto.Equal),
		cmpopts.SortSlices(func(a, b *trillian.MapLeaf) bool { return bytes.Compare(a.Index, b.Index) < 0 })) {
		t.Errorf("got - want: %v", cmp.Diff(got, want))
	}
}

// RunInclusionBatch performs checks on Trillian Map inclusion proofs, after setting and getting leafs in
// larger batches, checking also the SignedMapRoot revisions along the way, for a variety of hash strategies.
func RunInclusionBatch(ctx context.Context, t *testing.T, tadmin trillian.TrillianAdminClient, tmap trillian.TrillianMapClient) {
	for _, tc := range []struct {
		desc                  string
		HashStrategy          trillian.HashStrategy
		batchSize, numBatches int
		large                 bool
	}{

		{
			desc:         "maphasher short batch",
			HashStrategy: trillian.HashStrategy_TEST_MAP_HASHER,
			batchSize:    10, numBatches: 10,
			large: false,
		},
		{
			desc:         "maphasher batch",
			HashStrategy: trillian.HashStrategy_TEST_MAP_HASHER,
			batchSize:    64, numBatches: 32,
			large: true,
		},
		// TODO(gdbelvin): investigate batches of size > 150.
		// We are currently getting DB connection starvation: Too many connections.
	} {
		if testing.Short() && tc.large {
			glog.Infof("testing.Short() is true. Skipping %v", tc.desc)
			continue
		}
		tree, err := newTreeWithHasher(ctx, tadmin, tmap, tc.HashStrategy)
		if err != nil {
			t.Fatalf("%v: newTreeWithHasher(%v): %v", tc.desc, tc.HashStrategy, err)
		}

		if err := runMapBatchTest(ctx, t, tc.desc, tmap, tree, tc.batchSize, tc.numBatches); err != nil {
			t.Errorf("BatchSize: %v, Batches: %v: %v", tc.batchSize, tc.numBatches, err)
		}
	}
}

// runMapBatchTest is a helper for RunInclusionBatch.
func runMapBatchTest(ctx context.Context, t *testing.T, desc string, tmap trillian.TrillianMapClient, tree *trillian.Tree, batchSize, numBatches int) error {
	t.Helper()

	mapVerifier, err := client.NewMapVerifierFromTree(tree)
	if err != nil {
		t.Fatalf("NewMapVerifierFromTree(): %v", err)
	}

	// Ensure we're starting with an empty map
	if err := isEmptyMap(ctx, tmap, tree); err != nil {
		t.Fatalf("%s: isEmptyMap() err=%v want nil", desc, err)
	}

	leafMap := writeBatch(ctx, t, tmap, tree, batchSize, numBatches)

	// Check your head
	r, err := tmap.GetSignedMapRoot(ctx, &trillian.GetSignedMapRootRequest{MapId: tree.TreeId})
	if err != nil || r.MapRoot == nil {
		t.Fatalf("%s: failed to get map head: %v", desc, err)
	}

	if err := verifyGetSignedMapRootResponse(mapVerifier, r.GetMapRoot(), int64(numBatches)); err != nil {
		t.Fatalf("%s: %v", desc, err)
	}

	// Shuffle the indexes. Map access is randomized.
	indexBatch := make([][][]byte, 0, numBatches)
	i := 0
	for _, v := range leafMap {
		if i%batchSize == 0 {
			indexBatch = append(indexBatch, make([][]byte, 0, batchSize))
		}
		batchIndex := i / batchSize
		indexBatch[batchIndex] = append(indexBatch[batchIndex], v.Index)
		i++
	}

	for i, indexes := range indexBatch {
		getResp, err := tmap.GetLeaves(ctx, &trillian.GetMapLeavesRequest{
			MapId: tree.TreeId,
			Index: indexes,
		})
		if err != nil {
			t.Errorf("%s: GetLeaves(): %v", desc, err)
			continue
		}

		if err := verifyGetMapLeavesResponse(mapVerifier, getResp, indexes, int64(numBatches)); err != nil {
			t.Errorf("%s: batch %v: verifyGetMapLeavesResponse(): %v", desc, i, err)
			continue
		}

		// Verify leaf contents
		for _, incl := range getResp.MapLeafInclusion {
			index := incl.GetLeaf().GetIndex()
			leaf := incl.GetLeaf().GetLeafValue()
			ev, ok := leafMap[string(index)]
			if !ok {
				t.Errorf("%s: unexpected key returned: %s", desc, index)
			}
			if got, want := leaf, ev.LeafValue; !bytes.Equal(got, want) {
				t.Errorf("%s: got value %s, want %s", desc, got, want)
			}
		}
	}
	return nil
}

func writeBatch(ctx context.Context, t *testing.T, tmap trillian.TrillianMapClient, tree *trillian.Tree, batchSize, numBatches int) map[string]*trillian.MapLeaf {
	t.Helper()
	// Generate leaves.
	leafBatch := make([][]*trillian.MapLeaf, numBatches)
	leafMap := make(map[string]*trillian.MapLeaf)
	for i := range leafBatch {
		leafBatch[i] = createBatchLeaves(i, batchSize)
		for _, l := range leafBatch[i] {
			leafMap[string(l.Index)] = l
		}
	}

	// Write some data in batches
	for _, b := range leafBatch {
		if _, err := tmap.SetLeaves(ctx, &trillian.SetMapLeavesRequest{
			MapId:  tree.TreeId,
			Leaves: b,
		}); err != nil {
			t.Fatalf("SetLeaves(): %v", err)
		}
	}

	return leafMap
}
