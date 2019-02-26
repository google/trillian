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

package tile

import (
	"crypto/sha256"
	"fmt"
	"math/bits"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/google/trillian/merkle/compact"
	"github.com/google/trillian/merkle/rfc6962"
)

func mc(l uint64, i int64) MerkleCoords {
	if i < 0 {
		return MerkleCoords{Level: l, TreeSize: uint64(-i)}
	}
	return MerkleCoords{Level: l, Index: uint64(i)}
}

func TestMerkleCoordsString(t *testing.T) {
	tests := []struct {
		in   MerkleCoords
		want string
	}{
		{in: mc(0, 0), want: "0.0"},
		{in: mc(0, 1), want: "0.1"},
		{in: mc(0, 100), want: "0.100"},
		{in: mc(1, 0), want: "1.0"},
		{in: mc(1, 10), want: "1.10"},
		{in: mc(10, 0), want: "10.0"},
		{in: mc(100, 10), want: "100.10"},
		{in: mc(3, -11), want: "3.x@11=[8,11)"},
		// String() gives answers even for non-valid pseudo-node coords:
		//                    3.x=[0,7)
		//           2.0                2.x=[4,7)
		//       1.0      1.1      1.2     |
		//     0.0 0.1  0.2 0.3  0.4 0.5  0.6
		// There is no 1.x node because 2.x@7 = 1.2 : 0.6
		{in: mc(1, -7), want: "1.x@7=[6,7)"},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("string-L%d-I%d-Z%d", test.in.Level, test.in.Index, test.in.TreeSize), func(t *testing.T) {
			got := test.in.String()
			if got != test.want {
				t.Errorf("%+v.String()=%v, want %v", test.in, got, test.want)
			}
			wantPseudo, gotPseudo := test.in.IsPseudoNode(), strings.Contains(got, "@")
			if gotPseudo != wantPseudo {
				t.Errorf("%v.IsPseudoNode()=%v, want %v", test.in, gotPseudo, wantPseudo)
			}
		})
	}
}

func TestMerkleCoordsLeafRange(t *testing.T) {
	tests := []struct {
		in        MerkleCoords
		want      [2]uint64
		wantPanic bool
	}{
		{in: mc(0, 0), want: [2]uint64{0, 1}},
		{in: mc(0, 1), want: [2]uint64{1, 2}},
		{in: mc(0, 100), want: [2]uint64{100, 101}},
		{in: mc(1, 0), want: [2]uint64{0, 2}},
		{in: mc(1, 10), want: [2]uint64{20, 22}},
		{in: mc(1, 11), want: [2]uint64{22, 24}},
		{in: mc(10, 0), want: [2]uint64{0, 1024}},
		{in: mc(20, 10), want: [2]uint64{10 * 1048576, 11 * 1048576}},
		{in: mc(2, -3), want: [2]uint64{0, 3}},
		{in: mc(2, -5), want: [2]uint64{4, 5}},
		{in: mc(2, -6), want: [2]uint64{4, 6}},
		{in: mc(2, -7), want: [2]uint64{4, 7}},
		{in: mc(1, -7), want: [2]uint64{6, 7}},
		{in: mc(1, -5), want: [2]uint64{4, 5}},
		{in: mc(3, -5), want: [2]uint64{0, 5}},
		{in: mc(3, -13), want: [2]uint64{8, 13}},
		{in: mc(62, 0), wantPanic: true},
		{in: mc(59, 65535), wantPanic: true},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("range-for-%s", test.in), func(t *testing.T) {
			defer func() {
				if p := recover(); p != nil && !test.wantPanic {
					t.Errorf("%+v.LeafRange() panic %q", test.in, p)
				}
			}()
			gotL, gotR := test.in.LeafRange()
			if gotL != test.want[0] || gotR != test.want[1] {
				t.Errorf("%+v.LeafRange()=[%d, %d), want [%d,%d)", test.in, gotL, gotR, test.want[0], test.want[1])
			}
			if test.wantPanic {
				t.Errorf("%+v.LeafRange()=%v,%v, want panic", test.in, gotL, gotR)
			}
		})
	}
}

func TestMerkleCoordsChildren(t *testing.T) {
	tests := []struct {
		in           MerkleCoords
		wantL, wantR MerkleCoords
		wantPanic    bool
	}{
		{in: mc(0, 0), wantPanic: true},
		{in: mc(0, 10), wantPanic: true},
		{in: mc(0, -2), wantPanic: true},
		{in: mc(1, 0), wantL: mc(0, 0), wantR: mc(0, 1)},
		{in: mc(1, 10), wantL: mc(0, 20), wantR: mc(0, 21)},
		{in: mc(1, 11), wantL: mc(0, 22), wantR: mc(0, 23)},
		{in: mc(10, 0), wantL: mc(9, 0), wantR: mc(9, 1)},
		{in: mc(100, 10), wantL: mc(99, 20), wantR: mc(99, 21)},
		{in: mc(2, -3), wantL: mc(1, 0), wantR: mc(0, 2)},
		{in: mc(2, -7), wantL: mc(1, 2), wantR: mc(0, 6)},
		{in: mc(3, -5), wantL: mc(2, 0), wantR: mc(0, 4)},
		// Example:
		//                                      4.x=[0,11)
		//                    3.0                         |
		//           2.0                2.1             2.x=[8,11)
		//       1.0      1.1      1.2      1.3       1.4    |
		//     0.0 0.1  0.2 0.3  0.4 0.5  0.6 0.7   0.8 0.9 0.10
		{in: mc(4, -11), wantL: mc(3, 0), wantR: mc(2, -11)},
		{in: mc(2, -11), wantL: mc(1, 4), wantR: mc(0, 10)},
		{in: mc(3, -13), wantL: mc(2, 2), wantR: mc(0, 12)},
	}
	for i, test := range tests {
		t.Run(test.in.String(), func(t *testing.T) {
			defer func() {
				if p := recover(); p != nil && !test.wantPanic {
					t.Errorf("%+v.{LR}Child() panic %q", test.in, p)
				}
			}()
			var gotL, gotR MerkleCoords
			// Hack: alternate the order of asking for L/R so we hit full
			// code coverage in both.
			if i%2 == 0 {
				gotL, gotR = test.in.LeftChild(), test.in.RightChild()
			} else {
				gotR, gotL = test.in.RightChild(), test.in.LeftChild()
			}
			if gotL != test.wantL || gotR != test.wantR {
				t.Errorf("%+v.{LR}Child()=%v,%v, want %v,%v", test.in, gotL, gotR, test.wantL, test.wantR)
			}
			if test.wantPanic {
				t.Errorf("%+v.{LR}Child()=%v,%v, want panic", test.in, gotL, gotR)
			}
		})
	}
}

func TestMerkleCoordsContains(t *testing.T) {
	tests := []struct {
		in, candidate MerkleCoords
		want          bool
	}{
		{in: mc(0, 0), candidate: mc(1, 0), want: false},
		{in: mc(0, 0), candidate: mc(12, 10), want: false},
		{in: mc(0, 0), candidate: mc(0, 1), want: false},
		{in: mc(0, 0), candidate: mc(0, 0), want: true},
		{in: mc(10, 10), candidate: mc(11, 0), want: false},
		{in: mc(10, 10), candidate: mc(22, 10), want: false},
		{in: mc(10, 10), candidate: mc(10, 1), want: false},
		{in: mc(10, 10), candidate: mc(10, 10), want: true},
		{in: mc(2, 0), candidate: mc(1, 0), want: true},
		{in: mc(2, 0), candidate: mc(1, 1), want: true},
		{in: mc(2, 0), candidate: mc(1, 2), want: false},
		{in: mc(2, 0), candidate: mc(1, 3), want: false},
		{in: mc(2, 0), candidate: mc(0, 0), want: true},
		{in: mc(2, 0), candidate: mc(0, 1), want: true},
		{in: mc(2, 0), candidate: mc(0, 2), want: true},
		{in: mc(2, 0), candidate: mc(0, 3), want: true},
		{in: mc(2, 0), candidate: mc(0, 4), want: false},
		{in: mc(2, 20), candidate: mc(1, 39), want: false},
		{in: mc(2, 20), candidate: mc(1, 40), want: true},
		{in: mc(2, 20), candidate: mc(1, 41), want: true},
		{in: mc(2, 20), candidate: mc(1, 42), want: false},
		{in: mc(2, 20), candidate: mc(1, 43), want: false},
		{in: mc(2, 20), candidate: mc(0, 80), want: true},
		{in: mc(2, 20), candidate: mc(0, 81), want: true},
		{in: mc(2, 20), candidate: mc(0, 82), want: true},
		{in: mc(2, 20), candidate: mc(0, 83), want: true},
		{in: mc(2, 20), candidate: mc(0, 84), want: false},
		{in: mc(2, -5), candidate: mc(0, 3), want: false},
		{in: mc(2, -5), candidate: mc(0, 4), want: true},
		{in: mc(2, -5), candidate: mc(0, 5), want: false},
		{in: mc(2, -6), candidate: mc(0, 5), want: true},
		{in: mc(2, -6), candidate: mc(1, 2), want: true},
		{in: mc(2, -6), candidate: mc(2, 0), want: false},
		{in: mc(2, -6), candidate: mc(1, 3), want: false},
		{in: mc(2, -7), candidate: mc(0, 6), want: true},
		{in: mc(2, -7), candidate: mc(1, -7), want: true},
	}
	for _, test := range tests {
		got := test.in.Contains(test.candidate)
		if got != test.want {
			outerL, outerR := test.in.LeafRange()
			innerL, innerR := test.candidate.LeafRange()
			t.Errorf("%+v.Contains(%+v)=%v, want %v ([%d,%d) contains? [%d,%d))", test.in, test.candidate, got, test.want, outerL, outerR, innerL, innerR)
		}
	}
}

func TestMerkleCoordsInclusionPath(t *testing.T) {
	tests := []struct {
		idx, sz uint64
		want    []MerkleCoords
	}{
		{idx: 0, sz: 0, want: []MerkleCoords{}},
		{idx: 0, sz: 1, want: []MerkleCoords{}},
		{idx: 100, sz: 8, want: nil},
		{idx: 0, sz: 4, want: []MerkleCoords{mc(0, 1), mc(1, 1)}},
		{idx: 0, sz: 5, want: []MerkleCoords{mc(0, 1), mc(1, 1), mc(0, 4)}},
		{idx: 9, sz: 32, want: []MerkleCoords{mc(0, 8), mc(1, 5), mc(2, 3), mc(3, 0), mc(4, 1)}},
		{idx: 3, sz: 9, want: []MerkleCoords{mc(0, 2), mc(1, 0), mc(2, 1), mc(0, 8)}},
		// Examples from RFC 6962 section 2.1.3
		{idx: 0, sz: 7, want: []MerkleCoords{mc(0, 1), mc(1, 1), mc(2, -7)}},
		{idx: 3, sz: 7, want: []MerkleCoords{mc(0, 2), mc(1, 0), mc(2, -7)}},
		{idx: 4, sz: 7, want: []MerkleCoords{mc(0, 5), mc(0, 6), mc(2, 0)}},
		{idx: 6, sz: 7, want: []MerkleCoords{mc(1, 2), mc(2, 0)}},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%d-in-size-%d", test.idx, test.sz), func(t *testing.T) {
			got := InclusionPath(test.idx, test.sz)
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("InclusionPath(%d, %d)=%v, want %v", test.idx, test.sz, got, test.want)
			}
			checkInclusionPath(t, test.idx, test.sz, got)
		})
	}
}

func checkInclusionPath(t *testing.T, idx, size uint64, path []MerkleCoords) {
	t.Helper()
	if idx >= size {
		if len(path) != 0 {
			t.Errorf("non-empty path %v for index %d beyond size %d", path, idx, size)
		}
		return
	}
	// The inclusion path plus the original leaf should form a disjoint cover of
	// [0, size), and each interval along the way should be contiguous.
	coveredL, coveredR := idx, idx+1
	for i, node := range path {
		l, r := node.LeafRange()
		switch {
		case l == coveredR:
			// Add new range onto right-hand side of covered interval.
			coveredR = r
		case r == coveredL:
			// Add new range onto left-hand side of covered interval.
			coveredL = l
		default:
			t.Fatalf("non-contiguous range: covered [%d,%d) so far, node[%d]=%s adds [%d,%d)", coveredL, coveredR, i, node, l, r)
		}
	}
	if coveredL != 0 || coveredR != size {
		t.Errorf("incomplete range: covered [%d,%d) of [0, %d)", coveredL, coveredR, size)
	}
}

func TestInclusionPathInvariants(t *testing.T) {
	for tc := 0; tc < 1000; tc++ {
		height := uint64(1 + rand.Int63n(20))
		size := uint64(1) << (height - 1)
		idx := uint64(rand.Int63n(int64(size)))
		path := InclusionPath(idx, size)
		checkInclusionPath(t, idx, size, path)
	}
}

func TestMerkleCoordsConsistency(t *testing.T) {
	tests := []struct {
		from, to uint64
		want     []MerkleCoords
	}{
		{from: 0, to: 0, want: nil},
		{from: 22, to: 21, want: nil},
		// Examples from RFC 6962 section 2.1.3
		{from: 3, to: 7, want: []MerkleCoords{mc(0, 2), mc(0, 3), mc(1, 0), mc(2, -7)}},
		{from: 4, to: 7, want: []MerkleCoords{mc(2, -7)}},
		{from: 6, to: 7, want: []MerkleCoords{mc(1, 2), mc(0, 6), mc(2, 0)}},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("size-%d-to-size-%d", test.from, test.to), func(t *testing.T) {
			got := ConsistencyPath(test.from, test.to)
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("ConsistencyPath(%d, %d)=%v, want %v", test.from, test.to, got, test.want)
			}
			checkConsistencyPath(t, test.from, test.to, got)
		})
	}
}

func checkConsistencyPath(t *testing.T, from, to uint64, path []MerkleCoords) {
	t.Helper()
	if from >= to {
		if len(path) != 0 {
			t.Errorf("non-empty path %v for size %d to size %d", path, from, to)
		}
		return
	}
	coveredL, coveredR := uint64(0), from
	for i, node := range path {
		l, r := node.LeafRange()
		if l < coveredL {
			if r < coveredL {
				t.Fatalf("[%d,%d) union [%d,%d) from [%d]=%v is non-contiguous", coveredL, coveredR, l, r, i, node)
			}
			coveredL = l
		}
		if r > coveredR {
			if l > coveredR {
				t.Fatalf("[%d,%d) union [%d,%d) from [%d]=%v is non-contiguous", coveredL, coveredR, l, r, i, node)
			}
			coveredR = r
		}
	}
	if coveredL != 0 || coveredR != to {
		t.Errorf("incomplete range: covered [%d,%d) of [0, %d)", coveredL, coveredR, to)
	}
}

func TestMerkleCoordsRootNodeForRange(t *testing.T) {
	tests := []struct {
		in        [2]uint64
		want      MerkleCoords
		wantPanic bool
	}{
		{in: [2]uint64{1, 1}, wantPanic: true}, // inverted range
		{in: [2]uint64{2, 1}, wantPanic: true}, // inverted range
		{in: [2]uint64{0, 1}, want: mc(0, 0)},
		{in: [2]uint64{0, 2}, want: mc(1, 0)},
		{in: [2]uint64{3, 5}, wantPanic: true}, // left of range not power of two
		{in: [2]uint64{0, 3}, want: mc(2, -3)},
		{in: [2]uint64{0, 4}, want: mc(2, 0)},
		{in: [2]uint64{0, 5}, want: mc(3, -5)},
		{in: [2]uint64{0, 6}, want: mc(3, -6)},
		{in: [2]uint64{0, 7}, want: mc(3, -7)},
		{in: [2]uint64{0, 8}, want: mc(3, 0)},
		{in: [2]uint64{0, 9}, want: mc(4, -9)},
		{in: [2]uint64{0, 10}, want: mc(4, -10)},
		{in: [2]uint64{1, 3}, wantPanic: true}, // mixes subtrees
		{in: [2]uint64{2, 6}, wantPanic: true}, // mixes subtrees
		{in: [2]uint64{4, 5}, want: mc(0, 4)},
		{in: [2]uint64{4, 6}, want: mc(1, 2)},
		{in: [2]uint64{8, 11}, want: mc(2, -11)},
		{in: [2]uint64{8, 9}, want: mc(0, 8)},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("range-%d-to-%d", test.in[0], test.in[1]), func(t *testing.T) {
			defer func() {
				if p := recover(); p != nil && !test.wantPanic {
					t.Errorf("rootNodeForRange([%d,%d)) panic %q", test.in[0], test.in[1], p)
				}
			}()

			got := rootNodeForRange(test.in[0], test.in[1])
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("rootNodeForRange([%d,%d))=%v, want %v", test.in[0], test.in[1], got, test.want)
			}
			if test.wantPanic {
				t.Errorf("rootNodeForRange([%d,%d))=%v, want panic", test.in[0], test.in[1], got)
			}
			rangeL, rangeR := got.LeafRange()
			if rangeL != test.in[0] || rangeR != test.in[1] {
				t.Errorf("rootNodeForRange([%d,%d))=%v has range [%d,%d), want to recover input", test.in[0], test.in[1], got, rangeL, rangeR)
			}
		})
	}
}

func TestMerkleCoordsRootNodeForRangeRandom(t *testing.T) {
	trials := 5000
	max2 := int64(50)
	for i := 0; i < trials; i++ {
		d := 1 + uint64(rand.Int63n(max2))
		// Left edge needs to be zero or a power of two.
		l := uint64(0)
		r := uint64(rand.Int63n(1500))
		if d != uint64(max2-1) {
			// l is a power of two.
			l = uint64(1) << d
			r = l + 1 + uint64(rand.Int63n(int64(l-1)))
		}
		got := rootNodeForRange(l, r)
		rangeL, rangeR := got.LeafRange()
		if rangeL != l || rangeR != r {
			t.Errorf("rootNodeForRange([%d,%d))=%v has range [%d,%d), want to recover input", l, r, got, rangeL, rangeR)
		}
	}
}

func TestCompleteSubtreesCoords(t *testing.T) {
	tests := []struct {
		in   uint64
		want []MerkleCoords
	}{
		{in: 0, want: nil},
		{in: 1, want: []MerkleCoords{mc(0, 0)}},
		{in: 2, want: []MerkleCoords{mc(1, 0)}},
		{in: 3, want: []MerkleCoords{mc(1, 0), mc(0, 2)}},
		{in: 4, want: []MerkleCoords{mc(2, 0)}},
		{in: 5, want: []MerkleCoords{mc(2, 0), mc(0, 4)}},
		{in: 6, want: []MerkleCoords{mc(2, 0), mc(1, 2)}},
		{in: 7, want: []MerkleCoords{mc(2, 0), mc(1, 2), mc(0, 6)}},
		{in: 8, want: []MerkleCoords{mc(3, 0)}},
		{in: 9, want: []MerkleCoords{mc(3, 0), mc(0, 8)}},
		{in: 10, want: []MerkleCoords{mc(3, 0), mc(1, 4)}},
		{in: 11, want: []MerkleCoords{mc(3, 0), mc(1, 4), mc(0, 10)}},
	}
	for _, test := range tests {
		got := CompleteSubtrees(test.in)
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("CompleteSubtrees(%d)=%v, want %v", test.in, got, test.want)
		}
	}
}

func TestCompleteSubtreesCoordsInvariants(t *testing.T) {
	for size := uint64(0); size < 50000; size++ {
		// Check expected invariants for a wide range of tree sizes.
		got := CompleteSubtrees(size)

		totalSize := uint64(0)
		prevEnd := uint64(0)
		prevLevel := uint64(99999)
		prevSubtreeSize := uint64(0)
		for _, c := range got {
			// Level should be strictly monotonically decreasing.
			if c.Level >= prevLevel {
				t.Errorf("CompleteSubtrees(%d)=%+v has non-descending levels", size, got)
			}
			prevLevel = c.Level

			// Leaf ranges should be contiguous.
			start, end := c.LeafRange()
			if start != prevEnd {
				t.Errorf("CompleteSubtrees(%d)=%+v has non-contiguous ranges", size, got)
			}
			prevEnd = end
			subtreeSize := (end - start)
			totalSize += subtreeSize

			// Starting leaf index should be a multiple of the previous subtree size.
			if prevSubtreeSize != 0 && (start%prevSubtreeSize) != 0 {
				t.Errorf("CompleteSubtrees(%d)=%+v has index %d not a multiple of previous subtree size %d", size, got, start, prevSubtreeSize)
			}
			prevSubtreeSize = subtreeSize
		}
		// Subtree sizes should add up to size.
		if totalSize != size {
			t.Errorf("CompleteSubtrees(%d)=%+v adds up to %d", size, got, totalSize)
		}
	}
}

func TestCompleteSubtreesCoordsCompact(t *testing.T) {
	hasher := rfc6962.DefaultHasher
	for size := int64(2); size < 10000; size++ {
		sizeBits := bits.Len64(uint64(size))

		// Find the subtree coordinates for the size and populate some hashes.
		subtreeCoords := CompleteSubtrees(uint64(size))
		hashVals := make(map[MerkleCoords][]byte)
		wantHashes := make([][]byte, sizeBits)
		for _, coord := range subtreeCoords {
			h := sha256.Sum256([]byte(coord.String()))
			hashVals[coord] = h[:]
			wantHashes[coord.Level] = h[:]
		}

		var expectedRoot []byte
		index := size
		first := true
		mask := int64(1)
		for bit := 0; bit < sizeBits; bit++ {
			index >>= 1
			if size&mask != 0 {
				if first {
					expectedRoot = wantHashes[bit]
					first = false
				} else {
					expectedRoot = hasher.HashChildren(wantHashes[bit], expectedRoot)
				}
			}
			mask <<= 1
		}

		getNodesFn := func(ids []compact.NodeID) ([][]byte, error) {
			hashes := make([][]byte, len(ids))
			for i, id := range ids {
				var ok bool
				hashes[i], ok = hashVals[MerkleCoords{Level: uint64(id.Level), Index: uint64(id.Index)}]
				if !ok {
					return nil, fmt.Errorf("no hash available for %d.%d", id.Level, id.Index)
				}
			}
			return hashes, nil
		}

		// Build a compact tree that uses the hashes we made up.
		compactTree, err := compact.NewTreeWithState(hasher, size, getNodesFn, expectedRoot)
		if err != nil {
			t.Errorf("compact.NewTreeWithState(%d)=nil,%v, want _,nil", size, err)
			continue
		}
		if !isPerfectTree(size) {
			// Check the internal representations are the same.
			gotHashes := compactTree.Hashes()
			if !reflect.DeepEqual(gotHashes, wantHashes) {
				t.Errorf("compactTree(%d).Hashes()=%+v, want %+v", size, gotHashes, wantHashes)
			}
		}
	}
}

func isPerfectTree(size int64) bool {
	return size != 0 && (size&(size-1) == 0)
}
