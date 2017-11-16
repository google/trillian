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
	"crypto"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys/der"
	"github.com/google/trillian/examples/ct/ctmapper/ctmapperpb"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/testonly"

	"github.com/kylelemons/godebug/pretty"

	tcrypto "github.com/google/trillian/crypto"
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
	{"LeafHistory", RunLeafHistory},
	{"Inclusion", RunInclusion},
	{"InclusionBatch", RunInclusionBatch},
}

var h2b = testonly.MustHexDecode

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
	r, err := tmap.GetSignedMapRoot(ctx, &trillian.GetSignedMapRootRequest{
		MapId: tree.TreeId,
	})
	if err != nil {
		return fmt.Errorf("failed to get empty map head: %v", err)
	}

	if got, want := r.GetMapRoot().GetMapRevision(), int64(0); got != want {
		return fmt.Errorf("got SMR with revision %d, want %d", got, want)
	}
	return nil
}

func verifyGetSignedMapRootResponse(mapRoot *trillian.SignedMapRoot,
	wantRevision int64, pubKey crypto.PublicKey, hasher hashers.MapHasher, treeID int64) error {
	if got, want := mapRoot.GetMapRevision(), wantRevision; got != want {
		return fmt.Errorf("got SMR with revision %d, want %d", got, want)
	}
	// SignedMapRoot contains its own signature. To verify, we need to create a local
	// copy of the object and return the object to the state it was in when signed
	// by removing the signature from the object.
	smr := *mapRoot
	smr.Signature = nil // Remove the signature from the object to be verified.
	if err := tcrypto.VerifyObject(pubKey, smr, mapRoot.GetSignature()); err != nil {
		return fmt.Errorf("VerifyObject(SMR): %v", err)
	}
	return nil
}

func verifyGetMapLeavesResponse(getResp *trillian.GetMapLeavesResponse, indexes [][]byte,
	wantRevision int64, pubKey crypto.PublicKey, hasher hashers.MapHasher, treeID int64) error {
	if got, want := len(getResp.MapLeafInclusion), len(indexes); got != want {
		return fmt.Errorf("got %d values, want %d", got, want)
	}
	if err := verifyGetSignedMapRootResponse(getResp.GetMapRoot(), wantRevision, pubKey, hasher, treeID); err != nil {
		return err
	}
	rootHash := getResp.GetMapRoot().GetRootHash()
	for _, incl := range getResp.MapLeafInclusion {
		leaf := incl.GetLeaf().GetLeafValue()
		index := incl.GetLeaf().GetIndex()
		leafHash := incl.GetLeaf().GetLeafHash()
		proof := incl.GetInclusion()

		wantLeafHash, err := hasher.HashLeaf(treeID, index, leaf)
		if err != nil {
			return err
		}
		if got, want := leafHash, wantLeafHash; !bytes.Equal(got, want) {
			return fmt.Errorf("HashLeaf(%s): %x, want %x", leaf, got, want)
		}
		if err := merkle.VerifyMapInclusionProof(treeID, index,
			leaf, rootHash, proof, hasher); err != nil {
			return fmt.Errorf("VerifyMapInclusionProof(%x): %v", index, err)
		}
	}
	return nil
}

// newTreeWithHasher is a test setup helper for creating new trees with a given hasher.
func newTreeWithHasher(ctx context.Context, tadmin trillian.TrillianAdminClient, hashStrategy trillian.HashStrategy) (*trillian.Tree, hashers.MapHasher, error) {
	treeParams := stestonly.MapTree
	treeParams.HashStrategy = hashStrategy
	tree, err := tadmin.CreateTree(ctx, &trillian.CreateTreeRequest{
		Tree: treeParams,
	})
	if err != nil {
		return nil, nil, nil
	}

	hasher, err := hashers.NewMapHasher(tree.HashStrategy)
	if err != nil {
		return nil, nil, nil
	}
	return tree, hasher, nil
}

// RunMapRevisionZero performs checks on Trillian Map behavior for new, empty maps.
func RunMapRevisionZero(ctx context.Context, t *testing.T, tadmin trillian.TrillianAdminClient, tmap trillian.TrillianMapClient) {
	for _, tc := range []struct {
		desc         string
		hashStrategy []trillian.HashStrategy
		wantRev      int64
	}{
		{
			desc:         "empty map has SMR at rev 0 but not rev 1",
			hashStrategy: []trillian.HashStrategy{trillian.HashStrategy_TEST_MAP_HASHER, trillian.HashStrategy_CONIKS_SHA512_256},
			wantRev:      0,
		},
	} {
		for _, hashStrategy := range tc.hashStrategy {
			tree, hasher, err := newTreeWithHasher(ctx, tadmin, hashStrategy)
			if err != nil {
				t.Errorf("%v: newTreeWithHasher(%v): %v", tc.desc, hashStrategy, err)
			}
			pubKey, err := der.UnmarshalPublicKey(tree.GetPublicKey().GetDer())
			if err != nil {
				t.Errorf("%v: UnmarshalPublicKey(%v): %v", tc.desc, hashStrategy, err)
			}
			//
			getSmrResp, err := tmap.GetSignedMapRoot(ctx, &trillian.GetSignedMapRootRequest{
				MapId: tree.TreeId,
			})
			if err != nil {
				t.Errorf("%v: GetSignedMapRoot(): %v", tc.desc, err)
			}
			if err := verifyGetSignedMapRootResponse(getSmrResp.GetMapRoot(), tc.wantRev,
				pubKey, hasher, tree.TreeId); err != nil {
				t.Errorf("%v: verifyGetSignedMapRootResponse(rev %v): %v", tc.desc, tc.wantRev, err)
			}
			//
			getSmrByRevResp, err := tmap.GetSignedMapRootByRevision(ctx, &trillian.GetSignedMapRootByRevisionRequest{
				MapId:    tree.TreeId,
				Revision: 0,
			})
			if err != nil {
				t.Errorf("%v: GetSignedMapRootByRevision(): %v", tc.desc, err)
			}
			if err := verifyGetSignedMapRootResponse(getSmrByRevResp.GetMapRoot(), tc.wantRev,
				pubKey, hasher, tree.TreeId); err != nil {
				t.Errorf("%v: verifyGetSignedMapRootResponse(rev %v): %v", tc.desc, tc.wantRev, err)
			}
			//
			got, want := getSmrByRevResp.GetMapRoot(), getSmrResp.GetMapRoot()
			if diff := pretty.Compare(got, want); diff != "" {
				t.Errorf("%v: GetSignedMapRootByRevision() != GetSignedMapRoot(); diff (-got +want):\n%v", tc.desc, diff)
			}
			//
			getSmrByRevResp, err = tmap.GetSignedMapRootByRevision(ctx, &trillian.GetSignedMapRootByRevisionRequest{
				MapId:    tree.TreeId,
				Revision: 1,
			})
			if err == nil {
				t.Errorf("%v: GetSignedMapRootByRevision(rev: 1) err? false want? true", tc.desc)
			}
			// TODO(phad): ideally we'd inspect err's type and check it contains a NOT_FOUND Code (5), but I don't want
			// a dependency on gRPC here.
		}
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
			HashStrategy: []trillian.HashStrategy{trillian.HashStrategy_TEST_MAP_HASHER, trillian.HashStrategy_CONIKS_SHA512_256},
			set: [][]*trillian.MapLeaf{
				{}, // Advance revision without changing anything.
				{
					{Index: h2b("0000000000000000000000000000000000000000000000000000000000000000"), LeafValue: []byte("A")},
				},
				{}, // Advance revision without changing anything.
				{
					{Index: h2b("0000000000000000000000000000000000000000000000000000000000000000"), LeafValue: []byte("B")},
				},
				{
					{Index: h2b("0000000000000000000000000000000000000000000000000000000000000000"), LeafValue: []byte("C")},
				},
			},
			get: []struct {
				revision  int64
				Index     []byte
				LeafValue []byte
			}{
				{revision: 1, Index: h2b("0000000000000000000000000000000000000000000000000000000000000000"), LeafValue: nil},         // Empty to empty root.
				{revision: 2, Index: []byte("doesnotexist...................."), LeafValue: nil},                                      // Empty to first root, through empty branch.
				{revision: 2, Index: h2b("0000000000000000000000000000000000000000000000000000000000000000"), LeafValue: []byte("A")}, // Value to first root.
				{revision: 3, Index: h2b("0000000000000000000000000000000000000000000000000000000000000000"), LeafValue: []byte("A")},
				{revision: 4, Index: h2b("0000000000000000000000000000000000000000000000000000000000000000"), LeafValue: []byte("B")},
				{revision: 5, Index: h2b("0000000000000000000000000000000000000000000000000000000000000000"), LeafValue: []byte("C")},
			},
		},
	} {
		for _, hashStrategy := range tc.HashStrategy {
			tree, hasher, err := newTreeWithHasher(ctx, tadmin, hashStrategy)
			if err != nil {
				t.Errorf("%v: newTreeWithHasher(%v): %v", tc.desc, hashStrategy, err)
			}
			pubKey, err := der.UnmarshalPublicKey(tree.GetPublicKey().GetDer())
			if err != nil {
				t.Errorf("%v: UnmarshalPublicKey(%v): %v", tc.desc, hashStrategy, err)
			}

			for _, batch := range tc.set {
				setResp, err := tmap.SetLeaves(ctx, &trillian.SetMapLeavesRequest{
					MapId:  tree.TreeId,
					Leaves: batch,
				})
				if err != nil {
					t.Fatalf("%v: SetLeaves(): %v", tc.desc, err)
				}
				glog.Infof("Rev: %v Set(): %x", setResp.GetMapRoot().GetMapRevision(), setResp.GetMapRoot().GetRootHash())
			}

			for _, batch := range tc.get {
				indexes := [][]byte{batch.Index}
				getResp, err := tmap.GetLeaves(ctx, &trillian.GetMapLeavesRequest{
					MapId:    tree.TreeId,
					Index:    indexes,
					Revision: batch.revision,
				})
				if err != nil {
					t.Errorf("%v: GetLeaves(): %v", tc.desc, err)
					continue
				}
				glog.Infof("Rev: %v Get(): %x", getResp.GetMapRoot().GetMapRevision(), getResp.GetMapRoot().GetRootHash())

				if got, want := len(getResp.GetMapLeafInclusion()), 1; got < want {
					t.Errorf("GetLeaves(rev: %v).len: %v, want >= %v", batch.revision, got, want)
				}
				if got, want := getResp.GetMapLeafInclusion()[0].GetLeaf().GetLeafValue(), batch.LeafValue; !bytes.Equal(got, want) {
					t.Errorf("GetLeaves(rev: %v).LeafValue: %s, want %s", batch.revision, got, want)
				}

				if err := verifyGetMapLeavesResponse(getResp, indexes, batch.revision,
					pubKey, hasher, tree.TreeId); err != nil {
					t.Errorf("%v: verifyGetMapLeavesResponse(rev %v): %v", tc.desc, batch.revision, err)
				}
			}
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
			HashStrategy: []trillian.HashStrategy{trillian.HashStrategy_TEST_MAP_HASHER, trillian.HashStrategy_CONIKS_SHA512_256},
			leaves: []*trillian.MapLeaf{
				{Index: h2b("0000000000000000000000000000000000000000000000000000000000000000"), LeafValue: []byte("A")},
			},
		},
		{
			desc:         "multi",
			HashStrategy: []trillian.HashStrategy{trillian.HashStrategy_TEST_MAP_HASHER, trillian.HashStrategy_CONIKS_SHA512_256},
			leaves: []*trillian.MapLeaf{
				{Index: h2b("0000000000000000000000000000000000000000000000000000000000000000"), LeafValue: []byte("A")},
				{Index: h2b("0000000000000000000000000000000000000000000000000000000000000001"), LeafValue: []byte("B")},
				{Index: h2b("0000000000000000000000000000000000000000000000000000000000000002"), LeafValue: []byte("C")},
				{Index: h2b("0000000000000000000000000000000000000000000000000000000000000003"), LeafValue: nil},
			},
		},
		{
			desc:         "across subtrees",
			HashStrategy: []trillian.HashStrategy{trillian.HashStrategy_TEST_MAP_HASHER, trillian.HashStrategy_CONIKS_SHA512_256},
			leaves: []*trillian.MapLeaf{
				{Index: h2b("0000000000000180000000000000000000000000000000000000000000000000"), LeafValue: []byte("Z")},
			},
		},
	} {
		for _, hashStrategy := range tc.HashStrategy {
			tree, hasher, err := newTreeWithHasher(ctx, tadmin, hashStrategy)
			if err != nil {
				t.Errorf("%v: newTreeWithHasher(%v): %v", tc.desc, hashStrategy, err)
			}
			pubKey, err := der.UnmarshalPublicKey(tree.GetPublicKey().GetDer())
			if err != nil {
				t.Errorf("%v: UnmarshalPublicKey(%v): %v", tc.desc, hashStrategy, err)
			}

			if _, err := tmap.SetLeaves(ctx, &trillian.SetMapLeavesRequest{
				MapId:  tree.TreeId,
				Leaves: tc.leaves,
				Metadata: testonly.MustMarshalAnyNoT(&ctmapperpb.MapperMetadata{
					HighestFullyCompletedSeq: 0xcafe,
				}),
			}); err != nil {
				t.Errorf("%v: SetLeaves(): %v", tc.desc, err)
			}

			indexes := [][]byte{}
			for _, l := range tc.leaves {
				indexes = append(indexes, l.Index)
			}
			getResp, err := tmap.GetLeaves(ctx, &trillian.GetMapLeavesRequest{
				MapId:    tree.TreeId,
				Index:    indexes,
				Revision: -1,
			})
			if err != nil {
				t.Errorf("%v: GetLeaves(): %v", tc.desc, err)
			}

			if err := verifyGetMapLeavesResponse(getResp, indexes, 1,
				pubKey, hasher, tree.TreeId); err != nil {
				t.Errorf("%v: verifyGetMapLeavesResponse(): %v", tc.desc, err)
			}
		}
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
		tree, _, err := newTreeWithHasher(ctx, tadmin, tc.HashStrategy)
		if err != nil {
			t.Errorf("%v: newTreeWithHasher(%v): %v", tc.desc, tc.HashStrategy, err)
		}

		if err := runMapBatchTest(ctx, t, tc.desc, tmap, tree, tc.batchSize, tc.numBatches); err != nil {
			t.Errorf("BatchSize: %v, Batches: %v: %v", tc.batchSize, tc.numBatches, err)
		}
	}
}

// runMapBatchTest is a helper for RunInclusionBatch.
func runMapBatchTest(ctx context.Context, t *testing.T, desc string, tmap trillian.TrillianMapClient, tree *trillian.Tree, batchSize, numBatches int) error {
	t.Helper()

	// Parse variables from tree
	pubKey, err := der.UnmarshalPublicKey(tree.GetPublicKey().GetDer())
	if err != nil {
		t.Fatalf("%s: UnmarshalPublicKey(%s)=_, err=%v want nil", desc, tree.GetPublicKey(), err)
	}
	hasher, err := hashers.NewMapHasher(tree.HashStrategy)
	if err != nil {
		t.Fatalf("%s: NewMapHasher(%v)=_, err=%v want nil", desc, tree.HashStrategy, err)
	}

	// Ensure we're starting with an empty map
	if err := isEmptyMap(ctx, tmap, tree); err != nil {
		t.Errorf("%s: isEmptyMap() err=%v want nil", desc, err)
	}

	// Generate leaves.
	leafBatch := make([][]*trillian.MapLeaf, numBatches)
	leafMap := make(map[string]*trillian.MapLeaf)
	for i := range leafBatch {
		leafBatch[i] = createBatchLeaves(i, batchSize)
		for _, l := range leafBatch[i] {
			leafMap[hex.EncodeToString(l.Index)] = l
		}
	}

	// Write some data in batches
	for _, b := range leafBatch {
		if _, err := tmap.SetLeaves(ctx, &trillian.SetMapLeavesRequest{
			MapId:  tree.TreeId,
			Leaves: b,
		}); err != nil {
			t.Errorf("%s: SetLeaves(): %v", desc, err)
		}
	}

	// Check your head
	r, err := tmap.GetSignedMapRoot(ctx, &trillian.GetSignedMapRootRequest{
		MapId: tree.TreeId,
	})
	if err != nil {
		t.Errorf("%s: failed to get map head: %v", desc, err)
	}

	if got, want := r.MapRoot.MapRevision, int64(numBatches); got != want {
		t.Errorf("%s: got SMR with revision %d, want %d", desc, got, want)
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
			MapId:    tree.TreeId,
			Index:    indexes,
			Revision: -1,
		})
		if err != nil {
			t.Errorf("%s: GetLeaves(): %v", desc, err)
			continue
		}

		if err := verifyGetMapLeavesResponse(getResp, indexes, int64(numBatches),
			pubKey, hasher, tree.TreeId); err != nil {
			t.Errorf("%s: batch %v: verifyGetMapLeavesResponse(): %v", desc, i, err)
			continue
		}

		// Verify leaf contents
		for _, incl := range getResp.MapLeafInclusion {
			index := incl.GetLeaf().GetIndex()
			leaf := incl.GetLeaf().GetLeafValue()
			ev, ok := leafMap[hex.EncodeToString(index)]
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
