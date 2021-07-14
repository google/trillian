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

package merkle

import (
	"fmt"
	"testing"

	_ "github.com/golang/glog" // Logging flags for overarching "go test" runs.
	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian/merkle/compact"
	"github.com/google/trillian/merkle/proof"
)

// TestCalcInclusionProofNodeAddresses contains inclusion proof tests. For
// reference, consider the following example of a tree from RFC 6962:
//
//                hash              <== Level 3
//               /    \
//              /      \
//             /        \
//            /          \
//           /            \
//          k              l        <== Level 2
//         / \            / \
//        /   \          /   \
//       /     \        /     \
//      g       h      i      [ ]   <== Level 1
//     / \     / \    / \    /
//     a b     c d    e f    j      <== Level 0
//     | |     | |    | |    |
//     d0 d1   d2 d3  d4 d5  d6
//
// Our storage node layers are always populated from the bottom up, hence the
// gap at level 1, index 3 in the above picture.
func TestCalcInclusionProofNodeAddresses(t *testing.T) {
	id := compact.NewNodeID
	nodes := func(ids ...compact.NodeID) proof.Nodes {
		return proof.Nodes{IDs: ids}
	}
	rehash := func(begin, end int, ephem compact.NodeID, ids ...compact.NodeID) proof.Nodes {
		return proof.Nodes{IDs: ids, Ephem: ephem, Begin: begin, End: end}
	}
	for _, tc := range []struct {
		size    int64 // The requested past tree size.
		index   int64 // Leaf index in the requested tree.
		want    proof.Nodes
		wantErr bool
	}{
		// Errors.
		{size: 0, index: 0, wantErr: true},
		{size: 0, index: 1, wantErr: true},
		{size: 1, index: 2, wantErr: true},
		{size: 0, index: 3, wantErr: true},
		{size: -1, index: 3, wantErr: true},
		{size: 7, index: -1, wantErr: true},
		{size: 7, index: 8, wantErr: true},

		// Small trees.
		{size: 1, index: 0, want: proof.Nodes{IDs: []compact.NodeID{}}},
		{size: 2, index: 0, want: nodes(id(0, 1))},           // b
		{size: 2, index: 1, want: nodes(id(0, 0))},           // a
		{size: 3, index: 1, want: nodes(id(0, 0), id(0, 2))}, // a c

		// Tree of size 7.
		{size: 7, index: 0, want: rehash(2, 4, id(2, 1), // l=hash(i,j)
			id(0, 1), id(1, 1), id(0, 6), id(1, 2))}, // b h j i
		{size: 7, index: 1, want: rehash(2, 4, id(2, 1), // l=hash(i,j)
			id(0, 0), id(1, 1), id(0, 6), id(1, 2))}, // a h j i
		{size: 7, index: 2, want: rehash(2, 4, id(2, 1), // l=hash(i,j)
			id(0, 3), id(1, 0), id(0, 6), id(1, 2))}, // d g j i
		{size: 7, index: 3, want: rehash(2, 4, id(2, 1), // l=hash(i,j)
			id(0, 2), id(1, 0), id(0, 6), id(1, 2))}, // c g j i
		{size: 7, index: 4, want: nodes(id(0, 5), id(0, 6), id(2, 0))}, // f j k
		{size: 7, index: 5, want: nodes(id(0, 4), id(0, 6), id(2, 0))}, // e j k
		{size: 7, index: 6, want: nodes(id(1, 2), id(2, 0))},           // i k

		// Smaller trees within a bigger stored tree.
		{size: 4, index: 2, want: nodes(id(0, 3), id(1, 0))},           // d g
		{size: 5, index: 3, want: nodes(id(0, 2), id(1, 0), id(0, 4))}, // c g e
		{size: 6, index: 3, want: nodes(id(0, 2), id(1, 0), id(1, 2))}, // c g i
		{size: 6, index: 4, want: nodes(id(0, 5), id(2, 0))},           // f k
		{size: 7, index: 1, want: rehash(2, 4, id(2, 1), // l=hash(i,j)
			id(0, 0), id(1, 1), id(0, 6), id(1, 2))}, // a h j i
		{size: 7, index: 3, want: rehash(2, 4, id(2, 1), // l=hash(i,j)
			id(0, 2), id(1, 0), id(0, 6), id(1, 2))}, // c g j i

		// Some rehashes in the middle of the returned list.
		{size: 15, index: 10, want: rehash(2, 4, id(2, 3),
			id(0, 11), id(1, 4),
			id(0, 14), id(1, 6),
			id(3, 0),
		)},
		{size: 31, index: 24, want: rehash(2, 4, id(2, 7),
			id(0, 25), id(1, 13),
			id(0, 30), id(1, 14),
			id(3, 2), id(4, 0),
		)},
		{size: 95, index: 81, want: rehash(3, 6, id(3, 11),
			id(0, 80), id(1, 41), id(2, 21),
			id(0, 94), id(1, 46), id(2, 22),
			id(4, 4), id(6, 0),
		)},
	} {
		t.Run(fmt.Sprintf("%d:%d", tc.size, tc.index), func(t *testing.T) {
			proof, err := CalcInclusionProofNodeAddresses(tc.size, tc.index)
			if tc.wantErr {
				if err == nil {
					t.Fatal("accepted bad params")
				}
				return
			} else if err != nil {
				t.Fatalf("CalcInclusionProofNodeAddresses: %v", err)
			}
			if diff := cmp.Diff(tc.want, proof); diff != "" {
				t.Errorf("paths mismatch:\n%v", diff)
			}
		})
	}
}

// TestCalcConsistencyProofNodeAddresses contains consistency proof tests. For
// reference, consider the following example:
//
//                hash5                         hash7
//               /    \                        /    \
//              /      \                      /      \
//             /        \                    /        \
//            /          \                  /          \
//           /            \                /            \
//          k             [ ]    -->      k              l
//         / \            /              / \            / \
//        /   \          /              /   \          /   \
//       /     \        /              /     \        /     \
//      g       h     [ ]             g       h      i      [ ]
//     / \     / \    /              / \     / \    / \    /
//     a b     c d    e              a b     c d    e f    j
//     | |     | |    |              | |     | |    | |    |
//     d0 d1   d2 d3  d4             d0 d1   d2 d3  d4 d5  d6
//
// The consistency proof between tree size 5 and 7 consists of nodes e, f, j,
// and k. The node j is taken instead of its missing parent.
func TestCalcConsistencyProofNodeAddresses(t *testing.T) {
	id := compact.NewNodeID
	nodes := func(ids ...compact.NodeID) proof.Nodes {
		return proof.Nodes{IDs: ids}
	}
	rehash := func(begin, end int, ephem compact.NodeID, ids ...compact.NodeID) proof.Nodes {
		return proof.Nodes{IDs: ids, Ephem: ephem, Begin: begin, End: end}
	}
	for _, tc := range []struct {
		size1   int64 // The smaller of the two tree sizes.
		size2   int64 // The bigger of the two tree sizes.
		want    proof.Nodes
		wantErr bool
	}{
		// Errors.
		{size1: 0, size2: -1, wantErr: true},
		{size1: -10, size2: 0, wantErr: true},
		{size1: -1, size2: -1, wantErr: true},
		{size1: 0, size2: 0, wantErr: true},
		{size1: 9, size2: 8, wantErr: true},

		{size1: 1, size2: 2, want: nodes(id(0, 1))},                     // b
		{size1: 1, size2: 4, want: nodes(id(0, 1), id(1, 1))},           // b h
		{size1: 1, size2: 6, want: nodes(id(0, 1), id(1, 1), id(1, 2))}, // b h i
		{size1: 2, size2: 3, want: nodes(id(0, 2))},                     // c
		{size1: 2, size2: 8, want: nodes(id(1, 1), id(2, 1))},           // h l
		{size1: 3, size2: 7, want: rehash(3, 5, id(2, 1), // l=hash(i,j)
			id(0, 2), id(0, 3), id(1, 0), id(0, 6), id(1, 2))}, // c d g j i
		{size1: 4, size2: 7, want: rehash(0, 2, id(2, 1), // l=hash(i,j)
			id(0, 6), id(1, 2))}, // j i
		{size1: 5, size2: 7, want: nodes(
			id(0, 4), id(0, 5), id(0, 6), id(2, 0))}, // e f j k
		{size1: 6, size2: 7, want: nodes(
			id(1, 2), id(0, 6), id(2, 0))}, // i j k
		{size1: 7, size2: 8, want: nodes(
			id(0, 6), id(0, 7), id(1, 2), id(2, 0))}, // j leaf#7 i k

		// Same tree size.
		{size1: 1, size2: 1, want: proof.Nodes{IDs: []compact.NodeID{}}},
		{size1: 2, size2: 2, want: proof.Nodes{IDs: []compact.NodeID{}}},
		{size1: 3, size2: 3, want: proof.Nodes{IDs: []compact.NodeID{}}},
		{size1: 4, size2: 4, want: proof.Nodes{IDs: []compact.NodeID{}}},
		{size1: 5, size2: 5, want: proof.Nodes{IDs: []compact.NodeID{}}},
		{size1: 7, size2: 7, want: proof.Nodes{IDs: []compact.NodeID{}}},
		{size1: 8, size2: 8, want: proof.Nodes{IDs: []compact.NodeID{}}},

		// Smaller trees within a bigger stored tree.
		{size1: 2, size2: 4, want: nodes(id(1, 1))}, // h
		{size1: 3, size2: 5, want: nodes(
			id(0, 2), id(0, 3), id(1, 0), id(0, 4))}, // c d g e
		{size1: 3, size2: 6, want: nodes(
			id(0, 2), id(0, 3), id(1, 0), id(1, 2))}, // c d g i
		{size1: 4, size2: 6, want: nodes(id(1, 2))}, // i
		{size1: 1, size2: 7, want: rehash(2, 4, id(2, 1), // l=hash(i,j)
			id(0, 1), id(1, 1), id(0, 6), id(1, 2))}, // b h j i

		// Some rehashes in the middle of the returned list.
		{size1: 10, size2: 15, want: rehash(2, 4, id(2, 3),
			id(1, 4), id(1, 5), id(0, 14), id(1, 6), id(3, 0))},
		{size1: 24, size2: 31, want: rehash(1, 4, id(3, 3),
			id(3, 2),
			id(0, 30), id(1, 14), id(2, 6),
			id(4, 0),
		)},
		{size1: 81, size2: 95, want: rehash(4, 7, id(3, 11),
			id(0, 80), id(0, 81), id(1, 41), id(2, 21),
			id(0, 94), id(1, 46), id(2, 22),
			id(4, 4), id(6, 0),
		)},
	} {
		t.Run(fmt.Sprintf("%d:%d", tc.size1, tc.size2), func(t *testing.T) {
			proof, err := CalcConsistencyProofNodeAddresses(tc.size1, tc.size2)
			if tc.wantErr {
				if err == nil {
					t.Fatal("accepted bad params")
				}
				return
			} else if err != nil {
				t.Fatalf("CalcConsistencyProofNodeAddresses: %v", err)
			}
			if diff := cmp.Diff(tc.want, proof); diff != "" {
				t.Errorf("paths mismatch:\n%v", diff)
			}
		})
	}
}

func TestInclusionSucceedsUpToTreeSize(t *testing.T) {
	const maxSize = 555
	for ts := 1; ts <= maxSize; ts++ {
		for i := ts; i < ts; i++ {
			if _, err := CalcInclusionProofNodeAddresses(int64(ts), int64(i)); err != nil {
				t.Errorf("CalcInclusionProofNodeAddresses(ts:%d, i:%d) = %v", ts, i, err)
			}
		}
	}
}

func TestConsistencySucceedsUpToTreeSize(t *testing.T) {
	const maxSize = 100
	for s1 := 1; s1 < maxSize; s1++ {
		for s2 := s1 + 1; s2 <= maxSize; s2++ {
			if _, err := CalcConsistencyProofNodeAddresses(int64(s1), int64(s2)); err != nil {
				t.Errorf("CalcConsistencyProofNodeAddresses(%d, %d) = %v", s1, s2, err)
			}
		}
	}
}
