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

package merkle

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"os"
	"runtime/pprof"
	"strings"
	"sync"
	"testing"

	"github.com/golang/glog"
	"github.com/golang/mock/gomock"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/testonly"
)

var (
	cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
	memprofile = flag.String("memprofile", "", "write mem profile to file")
)

// These nodes were generated randomly and reviewed to ensure node IDs do not collide with
// those fetched during the test.
var inclusionProofIncorrectTestNodes = []storage.Node{
	{NodeID: storage.NodeID{Path: []uint8{0x2c, 0x8b, 0xcf, 0xe1, 0xc5, 0x71, 0xf4, 0x2d, 0xc2, 0xe9, 0x22, 0x7d, 0x91, 0xd5, 0x93, 0x70, 0x8f, 0x8c, 0x40, 0xca, 0xf, 0xd3, 0xd8, 0x4b, 0x43, 0x6a, 0x3, 0x2f, 0xf1, 0x4, 0x7, 0x9b}, PrefixLenBits: 174, PathLenBits: 256}, Hash: []uint8{0x4, 0x7b, 0xe5, 0xab, 0x12, 0x2d, 0x44, 0x98, 0xd8, 0xcc, 0xc7, 0x27, 0x4d, 0xc5, 0xda, 0x59, 0x38, 0xf5, 0x4d, 0x9c, 0x98, 0x33, 0x2a, 0x95, 0xb1, 0x20, 0xe2, 0x8c, 0x7, 0x5f, 0xb5, 0x9a}, NodeRevision: 34},
	{NodeID: storage.NodeID{Path: []uint8{0x7c, 0xf5, 0x65, 0xc6, 0xd5, 0xbe, 0x2d, 0x39, 0xff, 0xf4, 0x58, 0xc2, 0x9f, 0x4f, 0x9, 0x3c, 0x54, 0x62, 0xf5, 0x35, 0x19, 0x87, 0x56, 0xb5, 0x4c, 0x6c, 0x11, 0xf3, 0xd7, 0x2, 0xc, 0x80}, PrefixLenBits: 234, PathLenBits: 256}, Hash: []uint8{0xbc, 0x33, 0xbe, 0x74, 0x79, 0x43, 0x59, 0x83, 0x5d, 0x93, 0x87, 0x13, 0x22, 0x98, 0xa0, 0x69, 0xed, 0xa5, 0xca, 0xfb, 0x7c, 0x16, 0x91, 0x51, 0xa2, 0xb, 0x9f, 0x17, 0xe4, 0x3f, 0xe3, 0x3}, NodeRevision: 34},
	{NodeID: storage.NodeID{Path: []uint8{0x5f, 0xc6, 0x73, 0x1c, 0x5d, 0x57, 0x23, 0xdc, 0x6a, 0xd, 0x38, 0xcb, 0x41, 0x25, 0x97, 0x2, 0x63, 0x8d, 0xa, 0x2d, 0xbe, 0x8e, 0x88, 0xff, 0x9e, 0x54, 0x5b, 0xb4, 0x5d, 0x4e, 0x6e, 0x5b}, PrefixLenBits: 223, PathLenBits: 256}, Hash: []uint8{0xb6, 0xd4, 0xbd, 0x76, 0x5e, 0x9b, 0x80, 0x2f, 0x71, 0x32, 0x5e, 0xf8, 0x41, 0xea, 0x47, 0xc7, 0x4, 0x7d, 0xd, 0x64, 0xa8, 0xf6, 0x22, 0xe4, 0xb4, 0xe1, 0xef, 0x2f, 0x67, 0xf8, 0x8b, 0xaa}, NodeRevision: 34},
	{NodeID: storage.NodeID{Path: []uint8{0x30, 0xe, 0x65, 0x75, 0x4d, 0xd9, 0x7a, 0x1, 0xc5, 0x2b, 0x2a, 0x6f, 0x4b, 0x59, 0x5d, 0xa8, 0xeb, 0x65, 0x25, 0x3a, 0xc5, 0xf7, 0xd2, 0x4b, 0xcc, 0x54, 0xbf, 0xe8, 0x6e, 0xe8, 0x96, 0xb7}, PrefixLenBits: 156, PathLenBits: 256}, Hash: []uint8{0x74, 0x93, 0x28, 0x98, 0xbc, 0xd0, 0xfd, 0x28, 0xa9, 0x39, 0xb5, 0xb5, 0xe9, 0xcc, 0x17, 0xe0, 0xe2, 0xd, 0x16, 0x14, 0xfd, 0xb1, 0x67, 0x19, 0x31, 0x3, 0x73, 0x35, 0xb4, 0x1d, 0x6d, 0x1d}, NodeRevision: 34},
	{NodeID: storage.NodeID{Path: []uint8{0x8e, 0x3b, 0x81, 0xe4, 0x2f, 0xe6, 0xd6, 0x52, 0x9b, 0xbd, 0x36, 0xc5, 0x3, 0x52, 0xe9, 0x60, 0xbb, 0xcb, 0xc9, 0xbd, 0x57, 0x96, 0xaf, 0x18, 0xd4, 0x94, 0xdd, 0x8, 0xa2, 0x43, 0x1e, 0x10}, PrefixLenBits: 157, PathLenBits: 256}, Hash: []uint8{0xe0, 0xb6, 0xea, 0x8a, 0xf1, 0x57, 0x1e, 0x5c, 0xbe, 0xbe, 0xd9, 0x5b, 0x29, 0x5f, 0x3, 0x7c, 0x32, 0x33, 0x77, 0xf7, 0x1c, 0x9e, 0x19, 0x4d, 0xc6, 0xdb, 0x5, 0xf7, 0x3e, 0x6c, 0xcb, 0x85}, NodeRevision: 34},
}

func maybeProfileCPU(t *testing.T) func() {
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			t.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		return pprof.StopCPUProfile
	}
	return func() {}
}

func maybeProfileMemory(t *testing.T) {
	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			t.Fatal(err)
		}
		pprof.WriteHeapProfile(f)
		f.Close()
	}
}

func newTX(tx storage.MapTreeTX) func() (storage.TreeTX, error) {
	return func() (storage.TreeTX, error) {
		glog.Infof("new tx")
		return tx, nil
	}
}

func getSparseMerkleTreeReaderWithMockTX(ctrl *gomock.Controller, rev int64) (*SparseMerkleTreeReader, *storage.MockMapTreeTX) {
	tx := storage.NewMockMapTreeTX(ctrl)
	return NewSparseMerkleTreeReader(rev, NewMapHasher(testonly.Hasher), tx), tx
}

func getSparseMerkleTreeWriterWithMockTX(ctrl *gomock.Controller, rev int64) (*SparseMerkleTreeWriter, *storage.MockMapTreeTX) {
	tx := storage.NewMockMapTreeTX(ctrl)
	tx.EXPECT().WriteRevision().AnyTimes().Return(rev)
	tree, err := NewSparseMerkleTreeWriter(rev, NewMapHasher(testonly.Hasher), newTX(tx))
	if err != nil {
		panic(err)
	}
	return tree, tx
}

type rootNodeMatcher struct{}

func (r rootNodeMatcher) Matches(x interface{}) bool {
	nodes, ok := x.([]storage.NodeID)
	if !ok {
		return false
	}
	return len(nodes) == 1 &&
		nodes[0].PrefixLenBits == 0
}

func (r rootNodeMatcher) String() string {
	return "is a single root node"
}

func randomBytes(t *testing.T, n int) []byte {
	r := make([]byte, n)
	g, err := rand.Read(r)
	if g != n || err != nil {
		t.Fatalf("Failed to read %d bytes of entropy for path, read %d and got error: %v", n, g, err)
	}
	return r
}

func getRandomRootNode(t *testing.T, rev int64) storage.Node {
	return storage.Node{
		NodeID:       storage.NewEmptyNodeID(0),
		Hash:         randomBytes(t, 32),
		NodeRevision: rev,
	}
}

func getRandomNonRootNode(t *testing.T, rev int64) storage.Node {
	nodeID := storage.NewNodeIDFromHash(randomBytes(t, 32))
	// Make sure it's not a root node.
	nodeID.PrefixLenBits = int(1 + randomBytes(t, 1)[0]%254)
	return storage.Node{
		NodeID:       nodeID,
		Hash:         randomBytes(t, 32),
		NodeRevision: rev,
	}
}

func TestRootAtRevision(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	r, tx := getSparseMerkleTreeReaderWithMockTX(mockCtrl, 100)
	node := getRandomRootNode(t, 14)
	tx.EXPECT().Commit().AnyTimes().Return(nil)
	tx.EXPECT().GetMerkleNodes(int64(23), rootNodeMatcher{}).Return([]storage.Node{node}, nil)
	root, err := r.RootAtRevision(23)
	if err != nil {
		t.Fatalf("Failed when calling RootAtRevision(23): %v", err)
	}
	if expected, got := root, node.Hash; !bytes.Equal(expected, got) {
		t.Fatalf("Expected root %v, got %v", expected, got)
	}
}

func TestRootAtUnknownRevision(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	r, tx := getSparseMerkleTreeReaderWithMockTX(mockCtrl, 100)
	tx.EXPECT().Commit().AnyTimes().Return(nil)
	tx.EXPECT().GetMerkleNodes(int64(23), rootNodeMatcher{}).Return([]storage.Node{}, nil)
	_, err := r.RootAtRevision(23)
	if err != ErrNoSuchRevision {
		t.Fatalf("Attempt to retrieve root an non-existent revision did not result in ErrNoSuchRevision: %v", err)
	}
}

func TestRootAtRevisionHasMultipleRoots(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	r, tx := getSparseMerkleTreeReaderWithMockTX(mockCtrl, 100)
	n1, n2 := getRandomRootNode(t, 14), getRandomRootNode(t, 15)
	tx.EXPECT().Commit().AnyTimes().Return(nil)
	tx.EXPECT().GetMerkleNodes(int64(23), rootNodeMatcher{}).Return([]storage.Node{n1, n2}, nil)
	_, err := r.RootAtRevision(23)
	if err == nil || err == ErrNoSuchRevision {
		t.Fatalf("Attempt to retrieve root an non-existent revision did not result in error: %v", err)
	}
}

func TestRootAtRevisionCatchesFutureRevision(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	const rev = 100
	r, tx := getSparseMerkleTreeReaderWithMockTX(mockCtrl, rev)
	// Sanity checking in RootAtRevision should catch this node being incorrectly
	// returned by the storage layer.
	n1 := getRandomRootNode(t, rev+1)
	tx.EXPECT().Commit().AnyTimes().Return(nil)
	tx.EXPECT().GetMerkleNodes(int64(rev), rootNodeMatcher{}).Return([]storage.Node{n1}, nil)
	_, err := r.RootAtRevision(rev)
	if err == nil || err == ErrNoSuchRevision {
		t.Fatalf("Attempt to retrieve root with corrupt node did not result in error: %v", err)
	}
}

func TestRootAtRevisionCatchesNonRootNode(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	const rev = 100
	r, tx := getSparseMerkleTreeReaderWithMockTX(mockCtrl, rev)
	// Sanity checking in RootAtRevision should catch this node being incorrectly
	// returned by the storage layer.
	n1 := getRandomNonRootNode(t, rev)
	tx.EXPECT().GetMerkleNodes(int64(rev), rootNodeMatcher{}).Return([]storage.Node{n1}, nil)
	_, err := r.RootAtRevision(rev)
	if err == nil || err == ErrNoSuchRevision {
		t.Fatalf("Attempt to retrieve root with corrupt node did not result in error: %v", err)
	}
}

func TestInclusionProofForNullEntryInEmptyTree(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	const rev = 100
	r, tx := getSparseMerkleTreeReaderWithMockTX(mockCtrl, rev)
	tx.EXPECT().Commit().AnyTimes().Return(nil)
	tx.EXPECT().GetMerkleNodes(int64(rev), gomock.Any()).Return([]storage.Node{}, nil)
	const key = "SomeArbitraryKey"
	proof, err := r.InclusionProof(rev, testonly.HashKey(key))
	if err != nil {
		t.Fatalf("Got error while retrieving inclusion proof: %v", err)
	}

	if expected, got := 256, len(proof); expected != got {
		t.Fatalf("Expected proof of len %d, but got len %d", expected, got)
	}

	// Verify these are null hashes
	for i := len(proof) - 1; i > 0; i-- {
		if got := proof[i]; got != nil {
			t.Errorf("proof[%d] = %v, expected nil", i, got)
		}
	}
}

// TODO(al): Add some more inclusion proof tests here

func TestInclusionProofGetsIncorrectNode(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// This test requests inclusion proofs where storage returns a single node that should not be part
	// of the proof for the supplied key. This should not succeed.
	for _, testNode := range inclusionProofIncorrectTestNodes {
		const rev = 100
		r, tx := getSparseMerkleTreeReaderWithMockTX(mockCtrl, rev)
		tx.EXPECT().Commit().AnyTimes().Return(nil)
		tx.EXPECT().GetMerkleNodes(int64(rev), gomock.Any()).Return([]storage.Node{testNode}, nil)
		const key = "SomeArbitraryKey"
		index := testonly.HashKey(key)
		proof, err := r.InclusionProof(rev, index)
		if err == nil {
			t.Errorf("InclusionProof() = %v, nil want: nil, 1 remain(s) unused", proof)
		}
		if !strings.Contains(err.Error(), "1 remain(s) unused") {
			t.Errorf("InclusionProof() = %v, %v. want: nil, 1 remain(s) unused", proof, err)
		}
	}
}

func TestInclusionProofPassesThroughStorageError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	const rev = 100
	r, tx := getSparseMerkleTreeReaderWithMockTX(mockCtrl, rev)
	e := errors.New("boo")
	tx.EXPECT().GetMerkleNodes(int64(rev), gomock.Any()).Return([]storage.Node{}, e)
	_, err := r.InclusionProof(rev, testonly.HashKey("Whatever"))
	if err != e {
		t.Fatalf("InclusionProof() should've returned an error '%v', but got '%v'", e, err)
	}
}

func TestInclusionProofGetsTooManyNodes(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	const rev = 100
	r, tx := getSparseMerkleTreeReaderWithMockTX(mockCtrl, rev)
	const key = "SomeArbitraryKey"
	keyHash := testonly.HashKey(key)
	// going to return one too many nodes
	nodes := make([]storage.Node, 257, 257)
	// First build a plausible looking set of proof nodes.
	for i := 1; i < 256; i++ {
		nodes[255-i].NodeID = storage.NewNodeIDFromHash(keyHash)
		nodes[255-i].NodeID.PrefixLenBits = i + 1
		nodes[255-i].NodeID.SetBit(i, nodes[255-i].NodeID.Bit(i)^1)
	}
	// and then tack on some rubbish:
	nodes[256] = getRandomNonRootNode(t, 42)

	tx.EXPECT().Commit().AnyTimes().Return(nil)
	tx.EXPECT().GetMerkleNodes(int64(rev), gomock.Any()).AnyTimes().Return(nodes, nil)
	_, err := r.InclusionProof(rev, testonly.HashKey(key))
	if err == nil {
		t.Fatal("InclusionProof() should've returned an error due to extra unused node")
	}
	if !strings.Contains(err.Error(), "failed to consume") {
		t.Fatalf("Saw unexpected error: %v", err)
	}
}

type sparseKeyValue struct {
	k, v string
}

type sparseTestVector struct {
	kv           []sparseKeyValue
	expectedRoot []byte
}

func testSparseTreeCalculatedRoot(t *testing.T, vec sparseTestVector) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	const rev = 100
	w, tx := getSparseMerkleTreeWriterWithMockTX(mockCtrl, rev)

	tx.EXPECT().Commit().AnyTimes().Return(nil)
	tx.EXPECT().Close().AnyTimes().Return(nil)
	tx.EXPECT().GetMerkleNodes(int64(rev), gomock.Any()).AnyTimes().Return([]storage.Node{}, nil)
	tx.EXPECT().SetMerkleNodes(gomock.Any()).AnyTimes().Return(nil)

	testSparseTreeCalculatedRootWithWriter(t, rev, vec, w)
}

func testSparseTreeCalculatedRootWithWriter(t *testing.T, rev int64, vec sparseTestVector, w *SparseMerkleTreeWriter) {
	var leaves []HashKeyValue
	for _, kv := range vec.kv {
		leaves = append(leaves, HashKeyValue{testonly.HashKey(kv.k), w.hasher.HashLeaf([]byte(kv.v))})
	}

	if err := w.SetLeaves(leaves); err != nil {
		t.Fatalf("Got error adding leaves: %v", err)
	}
	root, err := w.CalculateRoot()
	if err != nil {
		t.Fatalf("Failed to commit map changes: %v", err)
	}
	if expected, got := vec.expectedRoot, root; !bytes.Equal(expected, got) {
		t.Errorf("Expected root:\n%s, but got root:\n%s", base64.StdEncoding.EncodeToString(expected), base64.StdEncoding.EncodeToString(got))
	}
}

func TestSparseMerkleTreeWriterEmptyTree(t *testing.T) {
	testSparseTreeCalculatedRoot(t, sparseTestVector{[]sparseKeyValue{}, testonly.MustDecodeBase64("xmifEIEqCYCXbZUz2Dh1KCFmFZVn7DUVVxbBQTr1PWo=")})
}

func TestSparseMerkleTreeWriter(t *testing.T) {
	vec := sparseTestVector{
		[]sparseKeyValue{{"key1", "value1"}, {"key2", "value2"}, {"key3", "value3"}},
		testonly.MustDecodeBase64("Ms8A+VeDImofprfgq7Hoqh9cw+YrD/P/qibTmCm5JvQ="),
	}
	testSparseTreeCalculatedRoot(t, vec)
}

type nodeIDFuncMatcher struct {
	f func(ids []storage.NodeID) bool
}

func (f nodeIDFuncMatcher) Matches(x interface{}) bool {
	n, ok := x.([]storage.NodeID)
	if !ok {
		return false
	}
	return f.f(n)
}

func (f nodeIDFuncMatcher) String() string {
	return "matches function"
}

func testSparseTreeFetches(t *testing.T, vec sparseTestVector) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	const rev = 100
	w, tx := getSparseMerkleTreeWriterWithMockTX(mockCtrl, rev)
	tx.EXPECT().Commit().AnyTimes().Return(nil)
	tx.EXPECT().Close().AnyTimes().Return(nil)

	reads := make(map[string]string)
	readMutex := sync.Mutex{}
	var leafNodeIDs []storage.NodeID

	{
		readMutex.Lock()

		// calculate the set of expected node reads.
		for _, kv := range vec.kv {
			keyHash := testonly.HashKey(kv.k)
			nodeID := storage.NewNodeIDFromHash(keyHash)
			leafNodeIDs = append(leafNodeIDs, nodeID)
			sibs := nodeID.Siblings()

			// start with the set of siblings of all leaves:
			for j := range sibs {
				j := j
				id := sibs[j].String()
				pathNode := nodeID.String()[:len(id)]
				if _, ok := reads[pathNode]; ok {
					// we're modifying both children of a node because two keys are
					// intersecting, since both will be recalculated neither will be read
					// from storage so we remove the previously set expectation for this
					// node's sibling, and skip adding one for this node:
					delete(reads, pathNode)
					continue
				}
				reads[sibs[j].String()] = "unmet"
			}
		}

		// Next, remove any expectations for leaf-siblings which also happen to be
		// one of the keys being set by the test vector (unlikely to happen tbh):
		for i := range leafNodeIDs {
			delete(reads, leafNodeIDs[i].String())
		}

		readMutex.Unlock()
	}

	// Now, set up a mock call for GetMerkleNodes for the nodeIDs in the map
	// we've just created:
	tx.EXPECT().GetMerkleNodes(int64(rev), nodeIDFuncMatcher{func(ids []storage.NodeID) bool {
		if len(ids) == 0 {
			return false
		}
		readMutex.Lock()
		defer readMutex.Unlock()

		state, ok := reads[ids[0].String()]
		reads[ids[0].String()] = "met"
		return ok && state == "unmet"
	}}).AnyTimes().Return([]storage.Node{}, nil)

	// Now add a general catch-all for any unexpected calls. If we don't do this
	// it'll panic() with an unhelpful message on the first unexpected nodeID, so
	// rather than doing that we'll make a note of all the unexpected IDs here
	// instead, and we can then print them out later on.
	tx.EXPECT().GetMerkleNodes(int64(rev), gomock.Any()).AnyTimes().Do(
		func(rev int64, a []storage.NodeID) {
			if a == nil {
				return
			}

			readMutex.Lock()
			defer readMutex.Unlock()

			for i := range a {
				reads[a[i].String()] = "unexpected"
			}
		}).Return([]storage.Node{}, nil)

	// Figure out which nodes should be written:
	writes := make(map[string]string)
	writeMutex := sync.Mutex{}

	{
		writeMutex.Lock()
		for i := range leafNodeIDs {
			s := leafNodeIDs[i].String()
			for x := 0; x <= len(s); x++ {
				writes[s[:x]] = "unmet"
			}
		}
		writeMutex.Unlock()
	}

	tx.EXPECT().SetMerkleNodes(gomock.Any()).AnyTimes().Do(
		func(a []storage.Node) {
			writeMutex.Lock()
			defer writeMutex.Unlock()
			if a == nil {
				return
			}
			for i := range a {
				id := a[i].NodeID.String()
				state, ok := writes[id]
				switch {
				case !ok:
					writes[id] = "unexpected"
				case state == "unmet":
					writes[id] = "met"
				default:
					writes[id] = "duplicate"
				}
			}
		}).Return(nil)

	testSparseTreeCalculatedRootWithWriter(t, rev, vec, w)

	{
		readMutex.Lock()
		n, s := nonMatching(reads, "met")
		// Fail if there are any nodes which we expected to be read but weren't, or vice-versa:
		if n != 0 {
			t.Fatalf("saw unexpected/unmet calls to GetMerkleNodes for the following nodeIDs:\n%s", s)
		}
		readMutex.Unlock()
	}

	{
		writeMutex.Lock()
		n, s := nonMatching(writes, "met")
		// Fail if there are any nodes which we expected to be written but weren't, or vice-versa:
		if n != 0 {
			t.Fatalf("saw unexpected/unmet calls to SetMerkleNodes for the following nodeIDs:\n%s", s)
		}
		writeMutex.Unlock()
	}
}

func nonMatching(m map[string]string, needle string) (int, string) {
	s := ""
	n := 0
	for k, v := range m {
		if v != needle {
			s += fmt.Sprintf("%s: %s\n", k, v)
			n++
		}
	}
	return n, s
}

func TestSparseMerkleTreeWriterFetchesSingleLeaf(t *testing.T) {
	vec := sparseTestVector{
		[]sparseKeyValue{{"key1", "value1"}},
		testonly.MustDecodeBase64("PPI818D5CiUQQMZulH58LikjxeOFWw2FbnGM0AdVHWA="),
	}

	testSparseTreeFetches(t, vec)
}

func TestSparseMerkleTreeWriterFetchesMultipleLeaves(t *testing.T) {
	vec := sparseTestVector{
		[]sparseKeyValue{{"key1", "value1"}, {"key2", "value2"}, {"key3", "value3"}},
		testonly.MustDecodeBase64("Ms8A+VeDImofprfgq7Hoqh9cw+YrD/P/qibTmCm5JvQ="),
	}

	testSparseTreeFetches(t, vec)
}

func DISABLEDTestSparseMerkleTreeWriterBigBatch(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	defer maybeProfileCPU(t)()
	const rev = 100
	w, tx := getSparseMerkleTreeWriterWithMockTX(mockCtrl, rev)

	tx.EXPECT().Commit().AnyTimes().Return(nil)
	tx.EXPECT().GetMerkleNodes(int64(rev), gomock.Any()).Return([]storage.Node{}, nil)
	tx.EXPECT().SetMerkleNodes(gomock.Any()).Return(nil)

	const batchSize = 1024
	const numBatches = 4
	for x := 0; x < numBatches; x++ {
		h := make([]HashKeyValue, batchSize)
		for y := 0; y < batchSize; y++ {
			h[y].HashedKey = testonly.HashKey(fmt.Sprintf("key-%d-%d", x, y))
			h[y].HashedValue = w.hasher.HashLeaf([]byte(fmt.Sprintf("value-%d-%d", x, y)))
		}
		if err := w.SetLeaves(h); err != nil {
			t.Fatalf("Failed to batch %d: %v", x, err)
		}
	}
	root, err := w.CalculateRoot()
	if err != nil {
		t.Fatalf("Failed to calculate root hash: %v", err)
	}

	// calculated using python code.
	const expectedRootB64 = "Av30xkERsepT6F/AgbZX3sp91TUmV1TKaXE6QPFfUZA="
	if expected, got := testonly.MustDecodeBase64(expectedRootB64), root; !bytes.Equal(expected, root) {
		// Error, not Fatal so that we get our benchmark results regardless of the
		// result - useful if you want to up the amount of data without having to
		// figure out the expected root!
		t.Errorf("Expected root %s, got root: %s", base64.StdEncoding.EncodeToString(expected), base64.StdEncoding.EncodeToString(got))
	}
	maybeProfileMemory(t)
}

func bigIntFromString(dec string) *big.Int {
	r, ok := new(big.Int).SetString(dec, 10)
	if !ok {
		panic(fmt.Errorf("Couldn't parse %s as base10", dec))
	}
	return r
}

func TestNodeIDFromAddress(t *testing.T) {
	testVec := []struct {
		size           int
		prefix         []byte
		index          *big.Int
		depth          int
		expectedString string
	}{
		{1, []byte{}, big.NewInt(0xaa), 8, "10101010"},
		{3, []byte{0x12}, big.NewInt(0xaaaa), 16, "000100101010101010101010"},
		{2, []byte{0x12}, big.NewInt(0x05), 8, "0001001000000101"},
		{32, []byte{0xd1}, bigIntFromString("1180198625301186407651343252449371352369139634241136100932791642269679616"), 16, "110100010000000010101011"},
	}
	for i, vec := range testVec {
		nID := nodeIDFromAddress(vec.size, vec.prefix, vec.index, vec.depth)
		if expected, got := vec.expectedString, nID.String(); expected != got {
			t.Errorf("(test %d) expected %s, got %s (path: %x)", i, expected, got, nID.Path)
		}
	}
}
