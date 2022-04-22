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
	"github.com/google/trillian/merkle/compact" // nolint:staticcheck
	"github.com/google/trillian/merkle/rfc6962" // nolint:staticcheck
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
	node := func(level uint, index uint64) NodeFetch {
		return newNodeFetch(level, index, false)
	}
	rehash := func(level uint, index uint64) NodeFetch {
		return newNodeFetch(level, index, true)
	}
	for _, tc := range []struct {
		size    int64 // The requested past tree size.
		index   int64 // Leaf index in the requested tree.
		want    []NodeFetch
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
		{size: 1, index: 0, want: []NodeFetch{}},
		{size: 2, index: 0, want: []NodeFetch{node(0, 1)}},             // b
		{size: 2, index: 1, want: []NodeFetch{node(0, 0)}},             // a
		{size: 3, index: 1, want: []NodeFetch{node(0, 0), node(0, 2)}}, // a c

		// Tree of size 7.
		{size: 7, index: 0, want: []NodeFetch{
			node(0, 1), node(1, 1), rehash(0, 6), rehash(1, 2),
		}}, // b h l=hash(i,j)
		{size: 7, index: 1, want: []NodeFetch{
			node(0, 0), node(1, 1), rehash(0, 6), rehash(1, 2),
		}}, // a h l=hash(i,j)
		{size: 7, index: 2, want: []NodeFetch{
			node(0, 3), node(1, 0), rehash(0, 6), rehash(1, 2),
		}}, // d g l=hash(i,j)
		{size: 7, index: 3, want: []NodeFetch{
			node(0, 2), node(1, 0), rehash(0, 6), rehash(1, 2),
		}}, // c g l=hash(i,j)
		{size: 7, index: 4, want: []NodeFetch{
			node(0, 5), node(0, 6), node(2, 0),
		}}, // f j k
		{size: 7, index: 5, want: []NodeFetch{
			node(0, 4), node(0, 6), node(2, 0),
		}}, // e j k
		{size: 7, index: 6, want: []NodeFetch{
			node(1, 2), node(2, 0),
		}}, // i k

		// Smaller trees within a bigger stored tree.
		{size: 4, index: 2, want: []NodeFetch{
			node(0, 3), node(1, 0),
		}}, // d g
		{size: 5, index: 3, want: []NodeFetch{
			node(0, 2), node(1, 0), node(0, 4),
		}}, // c g e
		{size: 6, index: 3, want: []NodeFetch{
			node(0, 2), node(1, 0), node(1, 2),
		}}, // c g i
		{size: 6, index: 4, want: []NodeFetch{
			node(0, 5), node(2, 0),
		}}, // f k
		{size: 7, index: 1, want: []NodeFetch{
			node(0, 0), node(1, 1), rehash(0, 6), rehash(1, 2),
		}}, // a h l=hash(i,j)
		{size: 7, index: 3, want: []NodeFetch{
			node(0, 2), node(1, 0), rehash(0, 6), rehash(1, 2),
		}}, // c g l=hash(i,j)

		// Some rehashes in the middle of the returned list.
		{size: 15, index: 10, want: []NodeFetch{
			node(0, 11), node(1, 4), rehash(0, 14), rehash(1, 6), node(3, 0),
		}},
		{size: 31, index: 24, want: []NodeFetch{
			node(0, 25), node(1, 13),
			rehash(0, 30), rehash(1, 14),
			node(3, 2), node(4, 0),
		}},
		{size: 95, index: 81, want: []NodeFetch{
			node(0, 80), node(1, 41), node(2, 21),
			rehash(0, 94), rehash(1, 46), rehash(2, 22),
			node(4, 4), node(6, 0),
		}},
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
	node := func(level uint, index uint64) NodeFetch {
		return newNodeFetch(level, index, false)
	}
	rehash := func(level uint, index uint64) NodeFetch {
		return newNodeFetch(level, index, true)
	}
	for _, tc := range []struct {
		size1   int64 // The smaller of the two tree sizes.
		size2   int64 // The bigger of the two tree sizes.
		want    []NodeFetch
		wantErr bool
	}{
		// Errors.
		{size1: 0, size2: -1, wantErr: true},
		{size1: -10, size2: 0, wantErr: true},
		{size1: -1, size2: -1, wantErr: true},
		{size1: 0, size2: 0, wantErr: true},
		{size1: 9, size2: 8, wantErr: true},

		{size1: 1, size2: 2, want: []NodeFetch{node(0, 1)}},             // b
		{size1: 1, size2: 4, want: []NodeFetch{node(0, 1), node(1, 1)}}, // b h
		{size1: 1, size2: 6, want: []NodeFetch{
			node(0, 1), // b
			node(1, 1), // h
			node(1, 2), // i
		}},
		{size1: 2, size2: 3, want: []NodeFetch{node(0, 2)}},             // c
		{size1: 2, size2: 8, want: []NodeFetch{node(1, 1), node(2, 1)}}, // h l
		{size1: 3, size2: 7, want: []NodeFetch{
			node(0, 2),                 // c
			node(0, 3),                 // d
			node(1, 0),                 // g
			rehash(0, 6), rehash(1, 2), // l=hash(i,j)
		}},
		{size1: 4, size2: 7, want: []NodeFetch{rehash(0, 6), rehash(1, 2)}}, // l=hash(i,j)
		{size1: 5, size2: 7, want: []NodeFetch{
			node(0, 4), // e
			node(0, 5), // f
			node(0, 6), // j
			node(2, 0), // k
		}},
		{size1: 6, size2: 7, want: []NodeFetch{
			node(1, 2), // i
			node(0, 6), // j
			node(2, 0), // k
		}},
		{size1: 7, size2: 8, want: []NodeFetch{
			node(0, 6), // j
			node(0, 7), // leaf #7
			node(1, 2), // i
			node(2, 0), // k
		}},

		// Same tree size.
		{size1: 1, size2: 1, want: []NodeFetch{}},
		{size1: 2, size2: 2, want: []NodeFetch{}},
		{size1: 3, size2: 3, want: []NodeFetch{}},
		{size1: 4, size2: 4, want: []NodeFetch{}},
		{size1: 5, size2: 5, want: []NodeFetch{}},
		{size1: 7, size2: 7, want: []NodeFetch{}},
		{size1: 8, size2: 8, want: []NodeFetch{}},

		// Smaller trees within a bigger stored tree.
		{size1: 2, size2: 4, want: []NodeFetch{node(1, 1)}}, // h
		{size1: 3, size2: 5, want: []NodeFetch{
			node(0, 2), node(0, 3), node(1, 0), node(0, 4),
		}}, // c d g e
		{size1: 3, size2: 6, want: []NodeFetch{
			node(0, 2), node(0, 3), node(1, 0), node(1, 2),
		}}, // c d g i
		{size1: 4, size2: 6, want: []NodeFetch{node(1, 2)}}, // i
		{size1: 1, size2: 7, want: []NodeFetch{
			node(0, 1), node(1, 1), rehash(0, 6), rehash(1, 2),
		}}, // b h l=hash(i,j)
		{size1: 3, size2: 7, want: []NodeFetch{
			node(0, 2), node(0, 3), node(1, 0), rehash(0, 6), rehash(1, 2),
		}}, // c d g l=hash(i,j)

		// Some rehashes in the middle of the returned list.
		{size1: 10, size2: 15, want: []NodeFetch{
			node(1, 4), node(1, 5), rehash(0, 14), rehash(1, 6), node(3, 0),
		}},
		{size1: 24, size2: 31, want: []NodeFetch{
			node(3, 2),
			rehash(0, 30), rehash(1, 14), rehash(2, 6),
			node(4, 0),
		}},
		{size1: 81, size2: 95, want: []NodeFetch{
			node(0, 80), node(0, 81), node(1, 41), node(2, 21),
			rehash(0, 94), rehash(1, 46), rehash(2, 22),
			node(4, 4), node(6, 0),
		}},
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

func TestRehasher(t *testing.T) {
	th := rfc6962.DefaultHasher
	h := [][]byte{
		th.HashLeaf([]byte("Hash 1")),
		th.HashLeaf([]byte("Hash 2")),
		th.HashLeaf([]byte("Hash 3")),
		th.HashLeaf([]byte("Hash 4")),
		th.HashLeaf([]byte("Hash 5")),
	}

	for _, tc := range []struct {
		desc   string
		hashes [][]byte
		rehash []bool
		want   [][]byte
	}{
		{
			desc:   "no rehash",
			hashes: h[:3],
			rehash: []bool{false, false, false},
			want:   h[:3],
		},
		{
			desc:   "single rehash",
			hashes: h[:5],
			rehash: []bool{false, true, true, false, false},
			want:   [][]byte{h[0], th.HashChildren(h[2], h[1]), h[3], h[4]},
		},
		{
			desc:   "single rehash at end",
			hashes: h[:3],
			rehash: []bool{false, true, true},
			want:   [][]byte{h[0], th.HashChildren(h[2], h[1])},
		},
		{
			desc:   "single rehash multiple nodes",
			hashes: h[:5],
			rehash: []bool{false, true, true, true, false},
			want:   [][]byte{h[0], th.HashChildren(h[3], th.HashChildren(h[2], h[1])), h[4]},
		},
		{
			// TODO(pavelkalinnikov): This will never happen in our use-case. Design
			// the type to not allow multi-rehash by design.
			desc:   "multiple rehash",
			hashes: h[:5],
			rehash: []bool{true, true, false, true, true},
			want:   [][]byte{th.HashChildren(h[1], h[0]), h[2], th.HashChildren(h[4], h[3])},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			nf := make([]NodeFetch, len(tc.rehash))
			for i, r := range tc.rehash {
				nf[i].Rehash = r
			}
			h := append([][]byte{}, tc.hashes...)
			got, err := Rehash(h, nf, th.HashChildren)
			if err != nil {
				t.Errorf("Rehash: %v", err)
			}
			if want := tc.want; !cmp.Equal(got, want) {
				t.Errorf("proofs mismatch:\ngot: %x\nwant: %x", got, want)
			}
		})
	}
}

func newNodeFetch(level uint, index uint64, rehash bool) NodeFetch {
	return NodeFetch{ID: compact.NewNodeID(level, index), Rehash: rehash}
}
