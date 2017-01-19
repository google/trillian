package server

import (
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/testonly"
)

// rehashTest encapsulates one test case for the rehasher in isolation. Input data like the storage
// hashes and revisions can be arbitrary but the nodes should have distinct values
type rehashTest struct {
	desc    string
	index   int64
	nodes   []storage.Node
	fetches []merkle.NodeFetch
	output  trillian.Proof
}

// Raw hashes for dummy storage nodes
var h1 = th.HashLeaf([]byte("Hash 1"))
var h2 = th.HashLeaf([]byte("Hash 2"))
var h3 = th.HashLeaf([]byte("Hash 3"))
var h4 = th.HashLeaf([]byte("Hash 4"))
var h5 = th.HashLeaf([]byte("Hash 5"))

// And the dummy nodes themselves
var sn1 = storage.Node{NodeID: storage.NewNodeIDFromHash(h1), Hash: h1, NodeRevision: 11}
var sn2 = storage.Node{NodeID: storage.NewNodeIDFromHash(h2), Hash: h2, NodeRevision: 22}
var sn3 = storage.Node{NodeID: storage.NewNodeIDFromHash(h3), Hash: h3, NodeRevision: 33}
var sn4 = storage.Node{NodeID: storage.NewNodeIDFromHash(h4), Hash: h4, NodeRevision: 44}
var sn5 = storage.Node{NodeID: storage.NewNodeIDFromHash(h5), Hash: h5, NodeRevision: 55}

// And the output proof nodes expected for them if they are passed through without rehashing
var n1 = &trillian.Node{NodeHash: h1, NodeId: mustMarshalNodeID(sn1.NodeID), NodeRevision: sn1.NodeRevision}
var n2 = &trillian.Node{NodeHash: h2, NodeId: mustMarshalNodeID(sn2.NodeID), NodeRevision: sn2.NodeRevision}
var n3 = &trillian.Node{NodeHash: h3, NodeId: mustMarshalNodeID(sn3.NodeID), NodeRevision: sn3.NodeRevision}
var n4 = &trillian.Node{NodeHash: h4, NodeId: mustMarshalNodeID(sn4.NodeID), NodeRevision: sn4.NodeRevision}
var n5 = &trillian.Node{NodeHash: h5, NodeId: mustMarshalNodeID(sn5.NodeID), NodeRevision: sn5.NodeRevision}

// Nodes containing composite hashes. They don't have node ids or revisions as they're recomputed
var n1n2 = &trillian.Node{NodeHash: th.HashChildren(h2, h1)}
var n2n3 = &trillian.Node{NodeHash: th.HashChildren(h3, h2)}
var n2n3n4 = &trillian.Node{NodeHash: th.HashChildren(h4, th.HashChildren(h3, h2))}
var n4n5 = &trillian.Node{NodeHash: th.HashChildren(h5, h4)}

var rehashTests = []rehashTest{
	{
		desc:    "no rehash",
		index:   int64(126),
		nodes:   []storage.Node{sn1, sn2, sn3},
		fetches: []merkle.NodeFetch{{Rehash: false}, {Rehash: false}, {Rehash: false}},
		output: trillian.Proof{
			LeafIndex: int64(126),
			ProofNode: []*trillian.Node{n1, n2, n3},
		},
	},
	{
		desc:    "single rehash",
		index:   int64(999),
		nodes:   []storage.Node{sn1, sn2, sn3, sn4, sn5},
		fetches: []merkle.NodeFetch{{Rehash: false}, {Rehash: true}, {Rehash: true}, {Rehash: false}, {Rehash: false}},
		output: trillian.Proof{
			LeafIndex: int64(999),
			ProofNode: []*trillian.Node{n1, n2n3, n4, n5},
		},
	},
	{
		desc:    "single rehash at end",
		index:   int64(11),
		nodes:   []storage.Node{sn1, sn2, sn3},
		fetches: []merkle.NodeFetch{{Rehash: false}, {Rehash: true}, {Rehash: true}},
		output: trillian.Proof{
			LeafIndex: int64(11),
			ProofNode: []*trillian.Node{n1, n2n3},
		},
	},
	{
		desc:    "single rehash multiple nodes",
		index:   int64(23),
		nodes:   []storage.Node{sn1, sn2, sn3, sn4, sn5},
		fetches: []merkle.NodeFetch{{Rehash: false}, {Rehash: true}, {Rehash: true}, {Rehash: true}, {Rehash: false}},
		output: trillian.Proof{
			LeafIndex: int64(23),
			ProofNode: []*trillian.Node{n1, n2n3n4, n5},
		},
	},
	{
		desc:    "multiple rehash",
		index:   int64(45),
		nodes:   []storage.Node{sn1, sn2, sn3, sn4, sn5},
		fetches: []merkle.NodeFetch{{Rehash: true}, {Rehash: true}, {Rehash: false}, {Rehash: true}, {Rehash: true}},
		output: trillian.Proof{
			LeafIndex: int64(45),
			ProofNode: []*trillian.Node{n1n2, n3, n4n5},
		},
	},
}

// Test data used to test node fech deduplication in isolation
type dedupTest struct {
	desc         string
	input        []merkle.NodeFetch // input fetches to deduper
	storageIDs   []storage.NodeID   // ids sent to storage layer
	storageNodes []storage.Node     // nodes returned by storage layer
	storageError error              // error returned by storage layer
	result       []storage.Node     // expected result with dupes reinstated
}

// Contents of these don't really matter as long as they have distinct IDs
var f11 = merkle.NodeFetch{NodeID: mustCreateNodeID(1, 1)}
var f12 = merkle.NodeFetch{NodeID: mustCreateNodeID(1, 2)}
var f31 = merkle.NodeFetch{NodeID: mustCreateNodeID(3, 1)}

var n11 = storage.Node{NodeID: f11.NodeID, NodeRevision: 37}
var n12 = storage.Node{NodeID: f12.NodeID, NodeRevision: 37}
var n31 = storage.Node{NodeID: f31.NodeID, NodeRevision: 37}

var dedupTests = []dedupTest{
	{
		desc:         "no dupes",
		input:        []merkle.NodeFetch{f11, f12, f31},
		storageIDs:   []storage.NodeID{f11.NodeID, f12.NodeID, f31.NodeID},
		storageNodes: []storage.Node{n11, n12, n31},
		result:       []storage.Node{n11, n12, n31},
	},
	{
		desc:         "one dupe",
		input:        []merkle.NodeFetch{f11, f12, f31, f11},
		storageIDs:   []storage.NodeID{f11.NodeID, f12.NodeID, f31.NodeID},
		storageNodes: []storage.Node{n11, n12, n31},
		result:       []storage.Node{n11, n12, n31, n11},
	},
	{
		desc:         "multi dupe",
		input:        []merkle.NodeFetch{f11, f12, f11, f11, f31, f11},
		storageIDs:   []storage.NodeID{f11.NodeID, f12.NodeID, f31.NodeID},
		storageNodes: []storage.Node{n11, n12, n31},
		result:       []storage.Node{n11, n12, n11, n11, n31, n11},
	},
	{
		desc:         "storage fail",
		input:        []merkle.NodeFetch{f11, f12, f11, f11, f31, f11},
		storageIDs:   []storage.NodeID{f11.NodeID, f12.NodeID, f31.NodeID},
		storageNodes: []storage.Node{},
		storageError: errors.New("storage"),
	},
	{
		desc:         "storage wrong node",
		input:        []merkle.NodeFetch{f11, f12, f11, f11, f31, f11},
		storageIDs:   []storage.NodeID{f11.NodeID, f12.NodeID, f31.NodeID},
		storageNodes: []storage.Node{},
		storageError: errors.New("storage"),
	},
}

func TestRehasher(t *testing.T) {
	for _, rehashTest := range rehashTests {
		r := newRehasher()
		for i, node := range rehashTest.nodes {
			r.process(node, rehashTest.fetches[i])
		}

		want := rehashTest.output
		got, err := r.rehashedProof(rehashTest.index)

		if err != nil {
			t.Fatalf("rehash test %s unexpected error: %v", rehashTest.desc, err)
		}

		if !proto.Equal(&got, &want) {
			t.Errorf("rehash test %s:\ngot: %v\nwant: %v", rehashTest.desc, got, want)
		}
	}
}

func TestDedupFetcher(t *testing.T) {
	for _, dedupTest := range dedupTests {
		ctrl := gomock.NewController(t)
		tx := storage.NewMockLogTX(ctrl)
		tx.EXPECT().GetMerkleNodes(int64(37), dedupTest.storageIDs).Return(dedupTest.storageNodes, dedupTest.storageError)
		nodes, err := dedupAndFetchNodes(tx, 37, dedupTest.input)

		if err == nil && dedupTest.storageError != nil {
			t.Fatalf("%s: got nil, want error: %v", dedupTest.desc, err)
		}

		if err != nil && dedupTest.storageError == nil {
			t.Fatalf("%s: got error: %v, want nil", dedupTest.desc, err)
		}

		if got, want := nodes, dedupTest.result; len(want) > 0 && !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got: %v, want: %v", dedupTest.desc, got, want)
		}

		ctrl.Finish()
	}
}

func TestTree32InclusionProofFetchAll(t *testing.T) {
	for ts := 2; ts <= 32; ts++ {
		mt := treeAtSize(ts)
		r := testonly.NewMultiFakeNodeReaderFromLeaves([]testonly.LeafBatch{
			{TreeRevision: 3, Leaves: expandLeaves(0, ts-1), ExpectedRoot: expectedRootAtSize(mt)},
		})

		for s := 2; s <= ts; s++ {
			for l := 0; l < s; l++ {
				fetches, err := merkle.CalcInclusionProofNodeAddresses(int64(s), int64(l), int64(ts), 64)
				if err != nil {
					t.Fatal(err)
				}

				proof, err := fetchNodesAndBuildProof(r, 3, int64(l), fetches)
				if err != nil {
					t.Fatal(err)
				}

				// We use +1 here because of the 1 based leaf indexing of this implementation
				refProof := mt.PathToRootAtSnapshot(l+1, s)

				if got, want := len(proof.ProofNode), len(refProof); got != want {
					t.Fatalf("(%d, %d, %d): got proof len: %d, want: %d: %v\n%v", ts, s, l, got, want, fetches, refProof)
				}

				for i := 0; i < len(proof.ProofNode); i++ {
					if got, want := hex.EncodeToString(proof.ProofNode[i].NodeHash), hex.EncodeToString(refProof[i].Value.Hash()); got != want {
						t.Fatalf("(%d, %d, %d): %d got proof node: %s, want: %s l:%d fetches: %v", ts, s, l, i, got, want, len(proof.ProofNode), fetches)
					}
				}
			}
		}
	}
}

func TestTree32InclusionProofFetchMultiBatch(t *testing.T) {
	mt := treeAtSize(32)
	// The reader is built up with multiple batches, 4 batches x 8 leaves each
	r := testonly.NewMultiFakeNodeReaderFromLeaves([]testonly.LeafBatch{
		{TreeRevision: 3, Leaves: expandLeaves(0, 7), ExpectedRoot: expectedRootAtSize(treeAtSize(8))},
		{TreeRevision: 4, Leaves: expandLeaves(8, 15), ExpectedRoot: expectedRootAtSize(treeAtSize(16))},
		{TreeRevision: 5, Leaves: expandLeaves(16, 23), ExpectedRoot: expectedRootAtSize(treeAtSize(24))},
		{TreeRevision: 6, Leaves: expandLeaves(24, 31), ExpectedRoot: expectedRootAtSize(mt)},
	})

	for s := 2; s <= 32; s++ {
		for l := 0; l < s; l++ {
			fetches, err := merkle.CalcInclusionProofNodeAddresses(int64(s), int64(l), 32, 64)
			if err != nil {
				t.Fatal(err)
			}

			proof, err := fetchNodesAndBuildProof(r, 6, int64(l), fetches)
			if err != nil {
				t.Fatal(err)
			}

			// We use +1 here because of the 1 based leaf indexing of this implementation
			refProof := mt.PathToRootAtSnapshot(l+1, s)

			if got, want := len(proof.ProofNode), len(refProof); got != want {
				t.Fatalf("(%d, %d, %d): got proof len: %d, want: %d: %v\n%v", 32, s, l, got, want, fetches, refProof)
			}

			for i := 0; i < len(proof.ProofNode); i++ {
				if got, want := hex.EncodeToString(proof.ProofNode[i].NodeHash), hex.EncodeToString(refProof[i].Value.Hash()); got != want {
					t.Fatalf("(%d, %d, %d): %d got proof node: %s, want: %s l:%d fetches: %v", 32, s, l, i, got, want, len(proof.ProofNode), fetches)
				}
			}
		}
	}
}

func TestTree32ConsistencyProofFetchAll(t *testing.T) {
	for ts := 2; ts <= 32; ts++ {
		mt := treeAtSize(ts)
		r := testonly.NewMultiFakeNodeReaderFromLeaves([]testonly.LeafBatch{
			{TreeRevision: 3, Leaves: expandLeaves(0, ts-1), ExpectedRoot: expectedRootAtSize(mt)},
		})

		for s1 := 2; s1 < ts; s1++ {
			for s2 := s1 + 1; s2 < ts; s2++ {
				fetches, err := merkle.CalcConsistencyProofNodeAddresses(int64(s1), int64(s2), int64(ts), 64)
				if err != nil {
					t.Fatal(err)
				}

				proof, err := fetchNodesAndBuildProof(r, 3, int64(s1), fetches)
				if err != nil {
					t.Fatal(err)
				}

				refProof := mt.SnapshotConsistency(s1, s2)

				if got, want := len(proof.ProofNode), len(refProof); got != want {
					t.Fatalf("(%d, %d, %d): got proof len: %d, want: %d: %v\n%v", ts, s1, s2, got, want, fetches, refProof)
				}

				for i := 0; i < len(proof.ProofNode); i++ {
					if got, want := hex.EncodeToString(proof.ProofNode[i].NodeHash), hex.EncodeToString(refProof[i].Value.Hash()); got != want {
						t.Fatalf("(%d, %d, %d): %d got proof node: %s, want: %s l:%d fetches: %v", ts, s1, s2, i, got, want, len(proof.ProofNode), fetches)
					}
				}
			}
		}
	}
}

func mustMarshalNodeID(nodeID storage.NodeID) []byte {
	idBytes, err := proto.Marshal(nodeID.AsProto())
	if err != nil {
		panic(err)
	}

	return idBytes
}

func mustCreateNodeID(depth, node int64) storage.NodeID {
	nodeID, err := storage.NewNodeIDForTreeCoords(depth, node, 64)
	if err != nil {
		panic(err)
	}

	return nodeID
}

func expandLeaves(n, m int) []string {
	leaves := make([]string, 0, m-n+1)
	for l := n; l <= m; l++ {
		leaves = append(leaves, fmt.Sprintf("Leaf %d", l))
	}

	return leaves
}

// expectedRootAtSize uses the in memory tree, the tree built with Compact Merkle Tree should
// have the same root.
func expectedRootAtSize(mt *merkle.InMemoryMerkleTree) string {
	return hex.EncodeToString(mt.CurrentRoot().Hash())
}

func treeAtSize(n int) *merkle.InMemoryMerkleTree {
	leaves := expandLeaves(0, n-1)
	mt := merkle.NewInMemoryMerkleTree(merkle.NewRFC6962TreeHasher(crypto.NewSHA256()))

	for _, leaf := range leaves {
		mt.AddLeaf([]byte(leaf))
	}

	return mt
}
