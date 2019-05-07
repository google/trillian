// Copyright 2019 Google Inc. All Rights Reserved.
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

package compact

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"math/bits"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/google/trillian/merkle/rfc6962"
)

var (
	hashChildren = rfc6962.DefaultHasher.HashChildren
	factory      = &RangeFactory{Hash: hashChildren}
)

// treeNode represents a Merkle tree node which roots a full binary subtree.
type treeNode struct {
	hash   []byte // The Merkle hash of the subtree.
	visits int    // The number of times this node was visited.
}

// tree contains a static Merkle tree, for testing.
type tree struct {
	size  uint64       // The number of leaves.
	nodes [][]treeNode // All perfect subtrees indexed by (level, index).
}

// newTree creates a new Merkle tree of the given size.
func newTree(t *testing.T, size uint64) (*tree, VisitFn) {
	levels := bits.Len64(size)
	// Allocate the nodes.
	nodes := make([][]treeNode, levels)
	tr := &tree{size: size, nodes: nodes}
	// Attach a visitor to the nodes and the testing handler.
	visit := func(level uint, index uint64, hash []byte) {
		if err := tr.visit(level, index, hash); err != nil {
			t.Errorf("visit(%d,%d): %v", level, index, err)
		}
	}

	for lvl := range nodes {
		nodes[lvl] = make([]treeNode, size>>uint(lvl))
	}
	// Compute leaf hashes.
	for i := uint64(0); i < size; i++ {
		nodes[0][i].hash = hashLeaf([]byte(fmt.Sprintf("data: %d", i)))
	}
	// Compute internal node hashes.
	for lvl := 1; lvl < levels; lvl++ {
		for i := range nodes[lvl] {
			nodes[lvl][i].hash = hashChildren(nodes[lvl-1][i*2].hash, nodes[lvl-1][i*2+1].hash)
		}
	}

	return tr, visit
}

// rootHash returns a canonical hash of the whole (possibly imperfect) tree.
func (tr *tree) rootHash() []byte {
	var hash []byte
	for _, level := range tr.nodes {
		if len(level)%2 == 1 {
			root := level[len(level)-1].hash
			if hash == nil {
				hash = root
			} else {
				hash = hashChildren(root, hash)
			}
		}
	}
	return hash
}

func (tr *tree) leaf(index uint64) []byte {
	return tr.nodes[0][index].hash
}

func (tr *tree) visit(level uint, index uint64, hash []byte) error {
	if level >= uint(len(tr.nodes)) || index >= uint64(len(tr.nodes[level])) {
		return errors.New("node does not exist")
	}
	tr.nodes[level][index].visits++
	if want := tr.nodes[level][index].hash; !bytes.Equal(hash, want) {
		return fmt.Errorf("hash mismatch: got %08x, want %08x", shorten(hash), shorten(want))
	}
	return nil
}

// verifyRange checks that the compact range's hashes match the tree.
func (tr *tree) verifyRange(t *testing.T, r *Range, wantMatch bool) {
	t.Helper()
	pos := r.Begin()
	if r.End() > tr.size {
		t.Fatalf("range is too long: %d > %d", r.End(), tr.size)
	}

	// Naively build the expected list of hashes comprising the compact range.
	left, right := decompose(pos, r.End())
	var hashes [][]byte
	for lvl := uint(0); lvl < 64; lvl++ {
		if left&(1<<lvl) != 0 {
			hashes = append(hashes, tr.nodes[lvl][pos>>lvl].hash)
			pos += 1 << lvl
		}
	}
	for lvl := uint(63); lvl < 64; lvl-- { // Overflows on the last iteration.
		if right&(1<<lvl) != 0 {
			hashes = append(hashes, tr.nodes[lvl][pos>>lvl].hash)
			pos += 1 << lvl
		}
	}

	if pos != r.End() {
		t.Fatalf("decompose: range [%d,%d) is not covered; end=%d", r.Begin(), r.End(), pos)
	}
	if match := reflect.DeepEqual(r.Hashes(), hashes); match != wantMatch {
		t.Errorf("hashes match: %v, expected %v", match, wantMatch)
	}
}

// verifyAllVisited checks that all nodes of the tree are visited exactly once.
// This is to verify the efficiency property of compact ranges: any merging
// process resulting in a single range generates *all* internal nodes, and each
// node is generated only once.
func (tr *tree) verifyAllVisited(t *testing.T, r *Range) {
	t.Helper()
	if r.Begin() != 0 || r.End() != tr.size {
		t.Errorf("range mismatch: got [%d,%d), want [%d,%d)", r.Begin(), r.End(), 0, tr.size)
	}
	for lvl, level := range tr.nodes {
		for index, node := range level {
			if got, want := node.visits, 1; got != want {
				t.Errorf("Node (%d,%d) visited %d times, want %d", lvl, index, got, want)
			}
		}
	}
}

// Merge up from [0,0) to [0, 177) by appending single entries.
func TestMergeForward(t *testing.T) {
	const numNodes = uint64(177)
	tree, visit := newTree(t, numNodes)
	rng := factory.NewEmptyRange(0)
	tree.verifyRange(t, rng, true)
	for i := uint64(0); i < numNodes; i++ {
		visit(0, i, tree.leaf(i))
		rng.Append(tree.leaf(i), visit)
		tree.verifyRange(t, rng, true)
	}
	tree.verifyAllVisited(t, rng)
}

// Merge down from [339,340) to [0,340) by prepending single entries.
func TestMergeBackwards(t *testing.T) {
	const numNodes = uint64(340)
	tree, visit := newTree(t, numNodes)
	rng := factory.NewEmptyRange(numNodes)
	tree.verifyRange(t, rng, true)
	for i := numNodes; i > 0; i-- {
		visit(0, i-1, tree.leaf(i-1))
		prepend := factory.NewEmptyRange(i - 1)
		tree.verifyRange(t, prepend, true)
		prepend.Append(tree.leaf(i-1), visit)
		tree.verifyRange(t, prepend, true)
		if err := prepend.AppendRange(rng, visit); err != nil {
			t.Fatalf("AppendRange: %v", err)
		}
		rng = prepend
		tree.verifyRange(t, rng, true)
	}
	tree.verifyAllVisited(t, rng)
}

// Build ranges [0, 13), [13, 26), ... [208,220) by appending single entries to
// each. Then append those ranges one by one to [0,0), to get [0,220).
func TestMergeInBatches(t *testing.T) {
	const numNodes = uint64(220)
	const batch = uint64(13)
	tree, visit := newTree(t, numNodes)

	batches := make([]*Range, 0)
	// Merge all the nodes within the batches.
	for i := uint64(0); i < numNodes; i += batch {
		rng := factory.NewEmptyRange(i)
		tree.verifyRange(t, rng, true)
		for node := i; node < i+batch && node < numNodes; node++ {
			visit(0, node, tree.leaf(node))
			if err := rng.Append(tree.leaf(node), visit); err != nil {
				t.Fatalf("Append: %v", err)
			}
			tree.verifyRange(t, rng, true)
		}
		batches = append(batches, rng)
	}

	total := factory.NewEmptyRange(0)
	// Merge the batches.
	for _, batch := range batches {
		if err := total.AppendRange(batch, visit); err != nil {
			t.Fatalf("AppendRange: %v", err)
		}
		tree.verifyRange(t, total, true)
	}
	tree.verifyAllVisited(t, total)
}

// Build many trees of random size by randomly merging their sub-ranges.
func TestMergeRandomly(t *testing.T) {
	for seed := int64(1); seed < 100; seed++ {
		t.Run(fmt.Sprintf("seed:%d", seed), func(t *testing.T) {
			rnd := rand.New(rand.NewSource(seed))
			numNodes := rand.Uint64() % 500
			t.Logf("Tree size: %d", numNodes)

			tree, visit := newTree(t, numNodes)
			var mergeAll func(begin, end uint64) *Range // Enable recursion.
			mergeAll = func(begin, end uint64) *Range {
				rng := factory.NewEmptyRange(begin)
				if begin+1 == end {
					visit(0, begin, tree.leaf(begin))
					if err := rng.Append(tree.leaf(begin), visit); err != nil {
						t.Fatalf("Append(%d): %v", begin, err)
					}
				} else if begin < end {
					mid := begin + uint64(rnd.Int63n(int64(end-begin)))
					if err := rng.AppendRange(mergeAll(begin, mid), visit); err != nil {
						t.Fatalf("AppendRange(%d,%d): %v", begin, mid, err)
					}
					if err := rng.AppendRange(mergeAll(mid, end), visit); err != nil {
						t.Fatalf("AppendRange(%d,%d): %v", mid, end, err)
					}
				}
				tree.verifyRange(t, rng, true)
				return rng
			}
			rng := mergeAll(0, numNodes)
			tree.verifyAllVisited(t, rng)
		})
	}
}

func TestNewRange(t *testing.T) {
	const numNodes = uint64(123)
	tree, visit := newTree(t, numNodes)
	rng := factory.NewEmptyRange(0)
	for i := uint64(0); i < numNodes; i++ {
		rng.Append(tree.leaf(i), visit)
	}

	if _, err := factory.NewRange(10, 5, nil); err == nil {
		t.Error("NewRange succeeded unexpectedly")
	}

	rng1, err := factory.NewRange(rng.Begin(), rng.End(), rng.Hashes())
	if err != nil {
		t.Fatalf("NewRange: %v", err)
	}
	tree.verifyRange(t, rng1, true)

	// The number of hashes is incorrect.
	_, err = factory.NewRange(rng.Begin(), rng.End(), append(rng.Hashes(), nil))
	if err == nil {
		t.Error("NewRange succeeded unexpectedly")
	}
	// The number of hashes does not correspond to the range.
	_, err = factory.NewRange(rng.Begin(), rng.End()-1, rng.Hashes())
	if err == nil {
		t.Error("NewRange succeeded unexpectedly")
	}

	rng.Hashes()[0][0] ^= 1 // Corrupt the original hashes.
	rng1, err = factory.NewRange(rng.Begin(), rng.End(), rng.Hashes())
	if err != nil {
		t.Fatalf("NewRange: %v", err)
	}
	tree.verifyRange(t, rng1, false)
}

func TestAppendRangeErrors(t *testing.T) {
	anotherFactory := &RangeFactory{Hash: hashChildren}
	nonEmpty1, _ := factory.NewRange(7, 8, [][]byte{[]byte("hash")})
	nonEmpty2, _ := factory.NewRange(0, 6, [][]byte{[]byte("hash0"), []byte("hash1")})
	nonEmpty3, _ := factory.NewRange(6, 7, [][]byte{[]byte("hash")})
	corrupt := func(rng *Range, dBegin, dEnd int64) *Range {
		rng.begin = uint64(int64(rng.begin) + dBegin)
		rng.end = uint64(int64(rng.end) + dEnd)
		return rng
	}
	for _, tc := range []struct {
		desc    string
		l, r    *Range
		wantErr string
	}{
		{
			desc: "ok",
			l:    factory.NewEmptyRange(0),
			r:    factory.NewEmptyRange(0),
		},
		{
			desc:    "incompatible",
			l:       factory.NewEmptyRange(0),
			r:       anotherFactory.NewEmptyRange(0),
			wantErr: "incompatible ranges",
		},
		{
			desc:    "disjoint",
			l:       factory.NewEmptyRange(0),
			r:       factory.NewEmptyRange(1),
			wantErr: "ranges are disjoint",
		},
		{
			desc:    "left_corrupted",
			l:       corrupt(factory.NewEmptyRange(7), -7, 0),
			r:       nonEmpty1,
			wantErr: "corrupted lhs range",
		},
		{
			desc:    "right_corrupted",
			l:       nonEmpty2,
			r:       corrupt(nonEmpty3, 0, 20),
			wantErr: "corrupted rhs range",
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			err := tc.l.AppendRange(tc.r, nil)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("AppendRange: %v; want nil", err)
				}
			} else if err == nil || !strings.HasPrefix(err.Error(), tc.wantErr) {
				t.Fatalf("AppendRange: %v; want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestGetRootHash(t *testing.T) {
	for size := uint64(0); size < 16; size++ {
		t.Run(fmt.Sprintf("size:%d", size), func(t *testing.T) {
			tree, _ := newTree(t, size)
			rng := factory.NewEmptyRange(0)
			for i := uint64(0); i < size; i++ {
				rng.Append(tree.leaf(i), nil)
			}
			root, err := rng.GetRootHash(nil)
			if err != nil {
				t.Fatalf("GetRootHash: %v", err)
			}
			if want := tree.rootHash(); !bytes.Equal(root, want) {
				t.Fatalf("GetRootHash: got %08x, want %08x", shorten(root), shorten(want))
			}
		})
	}

	// Should accept only [0, N) ranges.
	rng := factory.NewEmptyRange(10)
	if _, err := rng.GetRootHash(nil); err == nil {
		t.Error("GetRootHash succeeded unexpectedly")
	}
}

func TestGetRootHashGolden(t *testing.T) {
	// TODO(pavelkalinnikov): Values are copied from tree_test. Commonize them.
	for _, tc := range []struct {
		size     int
		wantRoot string
	}{
		{size: 10, wantRoot: "VjWMPSYNtCuCNlF/RLnQy6HcwSk6CIipfxm+hettA+4="},
		{size: 15, wantRoot: "j4SulYmocFuxdeyp12xXCIgK6PekBcxzAIj4zbQzNEI="},
		{size: 16, wantRoot: "c+4Uc6BCMOZf/v3NZK1kqTUJe+bBoFtOhP+P3SayKRE="},
		{size: 100, wantRoot: "dUh9hYH88p0CMoHkdr1wC2szbhcLAXOejWpINIooKUY="},
		{size: 255, wantRoot: "SmdsuKUqiod3RX2jyF2M6JnbdE4QuTwwipfAowI4/i0="},
		{size: 256, wantRoot: "qFI0t/tZ1MdOYgyPpPzHFiZVw86koScXy9q3FU5casA="},
		{size: 1000, wantRoot: "RXrgb8xHd55Y48FbfotJwCbV82Kx22LZfEbmBGAvwlQ="},
		{size: 4095, wantRoot: "cWRFdQhPcjn9WyBXE/r1f04ejxIm5lvg40DEpRBVS0w="},
		{size: 4096, wantRoot: "6uU/phfHg1n/GksYT6TO9aN8EauMCCJRl3dIK0HDs2M="},
		{size: 10000, wantRoot: "VZcav65F9haHVRk3wre2axFoBXRNeUh/1d9d5FQfxIg="},
		{size: 65535, wantRoot: "iPuVYJhP6SEE4gUFp8qbafd2rYv9YTCDYqAxCj8HdLM="},
	} {
		t.Run(fmt.Sprintf("size:%v", tc.size), func(t *testing.T) {
			rng := factory.NewEmptyRange(0)
			for i := 0; i < tc.size; i++ {
				data := []byte{byte(i & 0xff), byte((i >> 8) & 0xff)}
				hash, err := rfc6962.DefaultHasher.HashLeaf(data)
				if err != nil {
					t.Fatalf("HashLeaf(%x): %v", data, err)
				}
				if err := rng.Append(hash, nil); err != nil {
					t.Fatalf("Append(%d): %v", i, err)
				}
			}
			hash, err := rng.GetRootHash(nil)
			if err != nil {
				t.Fatalf("GetRootHash: %v", err)
			}
			if got, want := base64.StdEncoding.EncodeToString(hash), tc.wantRoot; got != want {
				t.Errorf("root hash mismatch: got %q, want %q", got, want)
			}
		})
	}
}

func TestDecomposeCases(t *testing.T) {
	for _, tc := range []struct {
		begin, end   uint64
		wantL, wantR uint64
	}{
		{begin: 0, end: 0, wantL: 0x00, wantR: 0x00},   // subtree sizes [],[]
		{begin: 0, end: 2, wantL: 0x00, wantR: 0x02},   // subtree sizes [], [2]
		{begin: 0, end: 4, wantL: 0x00, wantR: 0x04},   // subtree sizes [], [4]
		{begin: 1, end: 3, wantL: 0x01, wantR: 0x01},   // subtree sizes [1], [1]
		{begin: 3, end: 7, wantL: 0x01, wantR: 0x03},   // subtree sizes [1], [2, 1]
		{begin: 3, end: 17, wantL: 0x0d, wantR: 0x01},  // subtree sizes [1, 4, 8], [1]
		{begin: 4, end: 28, wantL: 0x0c, wantR: 0x0c},  // subtree sizes [4, 8], [8, 4]
		{begin: 8, end: 24, wantL: 0x08, wantR: 0x08},  // subtree sizes [8], [8]
		{begin: 8, end: 28, wantL: 0x08, wantR: 0x0c},  // subtree sizes [8], [8, 4]
		{begin: 11, end: 25, wantL: 0x05, wantR: 0x09}, // subtree sizes [1, 4], [8, 1]
		{begin: 31, end: 45, wantL: 0x01, wantR: 0x0d}, // subtree sizes [1], [8, 4, 1]
	} {
		t.Run(fmt.Sprintf("[%d,%d)", tc.begin, tc.end), func(t *testing.T) {
			gotL, gotR := decompose(tc.begin, tc.end)
			if gotL != tc.wantL || gotR != tc.wantR {
				t.Errorf("decompose(%d,%d)=0b%b,0b%b, want 0b%b,0b%b", tc.begin, tc.end, gotL, gotR, tc.wantL, tc.wantR)
			}
		})
	}
}

func verifyDecompose(begin, end uint64) error {
	left, right := decompose(begin, end)
	// Smoke test the sum of decomposition masks.
	if left+right != uint64(end-begin) {
		return fmt.Errorf("%d+%d != %d-%d", left, right, begin, end)
	}

	pos := begin
	for lvl := uint(0); lvl < 64; lvl++ {
		if size := uint64(1) << lvl; left&size != 0 {
			if pos%size != 0 {
				return fmt.Errorf("left: level %d not aligned", lvl)
			}
			pos += size
		}
	}
	for lvl := uint(63); lvl < 64; lvl-- { // Overflows on the last iteration.
		if size := uint64(1) << lvl; right&size != 0 {
			if pos%size != 0 {
				return fmt.Errorf("right: level %d not aligned", lvl)
			}
			pos += size
		}
	}
	if pos != end {
		return fmt.Errorf("decomposition covers up to %d, want %d", pos, end)
	}
	return nil
}

func TestDecompose(t *testing.T) {
	const n = uint64(100)
	for i := uint64(0); i <= n; i++ {
		for j := i; j <= n; j++ {
			if err := verifyDecompose(i, j); err != nil {
				t.Fatalf("verifyDecompose(%d,%d): %v", i, j, err)
			}
		}
	}
}

func TestDecomposePow2(t *testing.T) {
	for p := 0; p < 64; p++ {
		t.Run(fmt.Sprintf("2^%d", p), func(t *testing.T) {
			end := uint64(1) << uint(p)
			if err := verifyDecompose(0, end); err != nil {
				t.Fatalf("verifyDecompose(%d,%d): %v", 0, end, err)
			}
			end += end - 1
			if err := verifyDecompose(0, end); err != nil {
				t.Fatalf("verifyDecompose(%d,%d): %v", 0, end, err)
			}
		})
	}
}

func TestGetMergePath(t *testing.T) {
	for _, tc := range []struct {
		begin, mid, end uint64
		wantLow         uint
		wantHigh        uint
		wantEmpty       bool
	}{
		{begin: 0, mid: 0, end: 0, wantEmpty: true},
		{begin: 0, mid: 0, end: 1, wantEmpty: true},
		{begin: 0, mid: 0, end: uint64(1) << 63, wantEmpty: true},
		{begin: 0, mid: 1, end: 1, wantEmpty: true},
		{begin: 0, mid: 1, end: 2, wantLow: 0, wantHigh: 1},
		{begin: 0, mid: 16, end: 32, wantLow: 4, wantHigh: 5},
		{begin: 0, mid: uint64(1) << 63, end: ^uint64(0), wantEmpty: true},
		{begin: 0, mid: uint64(1) << 63, end: uint64(1)<<63 + 100500, wantEmpty: true},
		{begin: 2, mid: 9, end: 13, wantLow: 0, wantHigh: 2},
		{begin: 6, mid: 13, end: 17, wantLow: 0, wantHigh: 3},
		{begin: 4, mid: 8, end: 16, wantEmpty: true},
		{begin: 8, mid: 12, end: 16, wantLow: 2, wantHigh: 3},
		{begin: 4, mid: 6, end: 12, wantLow: 1, wantHigh: 2},
		{begin: 8, mid: 10, end: 16, wantLow: 1, wantHigh: 3},
		{begin: 11, mid: 17, end: 27, wantLow: 0, wantHigh: 3},
		{begin: 11, mid: 16, end: 27, wantEmpty: true},
	} {
		t.Run(fmt.Sprintf("%d:%d:%d", tc.begin, tc.mid, tc.end), func(t *testing.T) {
			low, high := getMergePath(tc.begin, tc.mid, tc.end)
			if tc.wantEmpty {
				if low < high {
					t.Fatalf("getMergePath(%d,%d,%d)=%d,%d; want empty", tc.begin, tc.mid, tc.end, low, high)
				}
			} else if low != tc.wantLow || high != tc.wantHigh {
				t.Fatalf("getMergePath(%d,%d,%d)=%d,%d; want %d,%d", tc.begin, tc.mid, tc.end, low, high, tc.wantLow, tc.wantHigh)
			}
		})
	}
}

func TestEqual(t *testing.T) {
	for _, test := range []struct {
		desc      string
		lhs       *Range
		rhs       *Range
		wantEqual bool
	}{
		{
			desc: "incompatible trees",
			lhs: &Range{
				f:      factory,
				begin:  17,
				end:    23,
				hashes: [][]byte{[]byte("hash 1"), []byte("hash 2")},
			},
			rhs: &Range{
				f:      &RangeFactory{Hash: hashChildren},
				begin:  17,
				end:    23,
				hashes: [][]byte{[]byte("hash 1"), []byte("hash 2")},
			},
		},

		{
			desc: "unequal begin",
			lhs: &Range{
				f:      factory,
				begin:  17,
				end:    23,
				hashes: [][]byte{[]byte("hash 1"), []byte("hash 2")},
			},
			rhs: &Range{
				f:      factory,
				begin:  18,
				end:    23,
				hashes: [][]byte{[]byte("hash 1"), []byte("hash 2")},
			},
		},

		{
			desc: "unequal end",
			lhs: &Range{
				f:      factory,
				begin:  17,
				end:    23,
				hashes: [][]byte{[]byte("hash 1"), []byte("hash 2")},
			},
			rhs: &Range{
				f:      factory,
				begin:  17,
				end:    24,
				hashes: [][]byte{[]byte("hash 1"), []byte("hash 2")},
			},
		},

		{
			desc: "unequal number of hashes",
			lhs: &Range{
				f:      factory,
				begin:  17,
				end:    23,
				hashes: [][]byte{[]byte("hash 1"), []byte("hash 2")},
			},
			rhs: &Range{
				f:      factory,
				begin:  17,
				end:    23,
				hashes: [][]byte{[]byte("hash 1")},
			},
		},

		{
			desc: "mismatched hash",
			lhs: &Range{
				f:      factory,
				begin:  17,
				end:    23,
				hashes: [][]byte{[]byte("hash 1"), []byte("hash 2")},
			},
			rhs: &Range{
				f:      factory,
				begin:  17,
				end:    23,
				hashes: [][]byte{[]byte("hash 1"), []byte("not hash 2")},
			},
		},

		{
			desc: "equal ranges",
			lhs: &Range{
				f:      factory,
				begin:  17,
				end:    23,
				hashes: [][]byte{[]byte("hash 1"), []byte("hash 2")},
			},
			rhs: &Range{
				f:      factory,
				begin:  17,
				end:    23,
				hashes: [][]byte{[]byte("hash 1"), []byte("hash 2")},
			},
			wantEqual: true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			if got, want := test.lhs.Equal(test.rhs), test.wantEqual; got != want {
				t.Errorf("%+v.Equal(%+v) = %v, want %v", test.lhs, test.rhs, got, want)
			}
		})
	}
}

func hashLeaf(data []byte) []byte {
	hash, err := rfc6962.DefaultHasher.HashLeaf(data)
	if err != nil {
		panic(fmt.Sprintf("rfc6962: HashLeaf: %v", err))
	}
	return hash
}

func shorten(hash []byte) []byte {
	if len(hash) < 4 {
		return hash
	}
	return hash[:4]
}
