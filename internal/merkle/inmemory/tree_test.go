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

package inmemory

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/rand"
	"testing"

	_ "github.com/golang/glog"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/transparency-dev/merkle/rfc6962"
	to "github.com/transparency-dev/merkle/testonly"
)

// TODO(pavelkalinnikov): Rewrite this file entirely.

var fuzzTestSize = int64(256)

// hx decodes a hex string or panics.
func hx(hs string) []byte {
	data, err := hex.DecodeString(hs)
	if err != nil {
		panic(fmt.Errorf("failed to decode test data: %s", hs))
	}
	return data
}

func makeEmptyTree() *Tree {
	return New(rfc6962.DefaultHasher)
}

func makeFuzzTestData() [][]byte {
	var data [][]byte

	for s := int64(0); s < fuzzTestSize; s++ {
		data = append(data, make([]byte, 1))
		data[s][0] = byte(s)
	}

	return data
}

func validateTree(t *testing.T, mt *Tree, size uint64) {
	t.Helper()
	if got, want := mt.Size(), size; got != want {
		t.Errorf("Size: %d, want %d", got, want)
	}
	roots := to.RootHashes()
	if got, want := mt.Hash(), roots[size]; !bytes.Equal(got, want) {
		t.Errorf("Hash(%d): %x, want %x", size, got, want)
	}
	for s := uint64(0); s <= size; s++ {
		if got, want := mt.HashAt(s), roots[s]; !bytes.Equal(got, want) {
			t.Errorf("HashAt(%d/%d): %x, want %x", s, size, got, want)
		}
	}
}

func TestBuildTreeBuildOneAtATime(t *testing.T) {
	mt := makeEmptyTree()
	validateTree(t, mt, 0)
	for i, entry := range to.LeafInputs() {
		mt.AppendData(entry)
		validateTree(t, mt, uint64(i+1))
	}
}

func TestBuildTreeBuildTwoChunks(t *testing.T) {
	entries := to.LeafInputs()
	mt := makeEmptyTree()
	mt.AppendData(entries[:3]...)
	validateTree(t, mt, 3)
	mt.AppendData(entries[3:8]...)
	validateTree(t, mt, 8)
}

func TestBuildTreeBuildAllAtOnce(t *testing.T) {
	mt := makeEmptyTree()
	mt.AppendData(to.LeafInputs()...)
	validateTree(t, mt, 8)
}

func TestTreeHashAt(t *testing.T) {
	test := func(desc string, entries [][]byte) {
		t.Run(desc, func(t *testing.T) {
			mt := makeEmptyTree()
			mt.AppendData(entries...)
			for size := 0; size <= len(entries); size++ {
				got := mt.HashAt(uint64(size))
				want := refRootHash(entries[:size], mt.hasher)
				if !bytes.Equal(got, want) {
					t.Errorf("HashAt(%d): %x, want %x", size, got, want)
				}
			}
		})
	}

	entries := to.LeafInputs()
	for size := 0; size <= len(entries); size++ {
		test(fmt.Sprintf("size:%d", size), entries[:size])
	}
	test("generated", makeFuzzTestData())
}

// Make random path queries and check against the reference implementation.
func TestMerkleTreePathFuzz(t *testing.T) {
	data := makeFuzzTestData()

	for treeSize := int64(1); treeSize <= fuzzTestSize; treeSize++ {
		// mt := makeLoggingEmptyTree(t)
		mt := makeEmptyTree()
		mt.AppendData(data[:treeSize]...)

		// Since the tree is evaluated lazily, the order of queries is significant.
		// Generate a random sequence of 8 queries for each tree.
		for j := 0; j < 8; j++ {
			// A snapshot in the range 1..treeSize.
			snapshot := uint64(rand.Int63n(treeSize)) + 1
			// A leaf in the range 0..snapshot-1.
			leaf := uint64(rand.Int63n(int64(snapshot)))

			p1, err := mt.InclusionProof(leaf, snapshot)
			if err != nil {
				t.Fatalf("InclusionProof: %v", err)
			}

			p2 := refInclusionProof(data[:snapshot], leaf, mt.hasher)

			if len(p1) != len(p2) {
				t.Errorf("Different path lengths %v, %v", p1, p2)
			} else {
				for i := 0; i < len(p1); i++ {
					if !bytes.Equal(p1[i], p2[i]) {
						t.Errorf("Mismatched hash %d %d %d: %v, %v", snapshot, leaf, i,
							p1[i], p2[i])
					}
				}
			}
		}
	}
}

// Make random proof queries and check against the reference implementation.
func TestMerkleTreeConsistencyFuzz(t *testing.T) {
	data := makeFuzzTestData()

	for treeSize := int64(1); treeSize <= fuzzTestSize; treeSize++ {
		mt := makeEmptyTree()
		mt.AppendData(data[:treeSize]...)

		// Since the tree is evaluated lazily, the order of queries is significant.
		// Generate a random sequence of 8 queries for each tree.
		for j := 0; j < 8; j++ {
			// A snapshot in the range 0... length.
			snapshot2 := uint64(rand.Int63n(treeSize + 1))
			// A snapshot in the range 0... snapshot.
			snapshot1 := uint64(rand.Int63n(int64(snapshot2) + 1))

			c1, err := mt.ConsistencyProof(snapshot1, snapshot2)
			if err != nil {
				t.Fatalf("ConsistencyProof: %v", err)
			}
			c2 := refConsistencyProof(data[:snapshot2], snapshot2, snapshot1, mt.hasher, true)

			if len(c1) != len(c2) {
				t.Errorf("Different proof lengths: %d %d %d", treeSize, snapshot2,
					snapshot1)
			}

			for i := 0; i < len(c1); i++ {
				if !bytes.Equal(c1[i], c2[i]) {
					t.Errorf("Different proof: %d %d %d %d, %s, %s", treeSize,
						snapshot2, snapshot1, i, hex.EncodeToString(c1[i]),
						hex.EncodeToString(c2[i]))
				}
			}
		}
	}
}

func TestTreeInslucionProof(t *testing.T) {
	entries := to.LeafInputs()
	mt := makeEmptyTree()
	mt.AppendData(entries...)
	validateTree(t, mt, 8)

	if _, err := mt.InclusionProof(8, 8); err == nil {
		t.Error("InclusionProof(8, 8) succeeded unexpectedly")
	}

	for size := uint64(1); size <= 8; size++ {
		for index := uint64(0); index < size; index++ {
			t.Run(fmt.Sprintf("%d:%d", index, size), func(t *testing.T) {
				got, err := mt.InclusionProof(index, size)
				if err != nil {
					t.Fatalf("InclusionProof: %v", err)
				}
				want := refInclusionProof(entries[:size], index, mt.hasher)
				if diff := cmp.Diff(got, want, cmpopts.EquateEmpty()); diff != "" {
					t.Errorf("InclusionProof: diff (-got +want)\n%s", diff)
				}
			})
		}
	}
}

func TestMerkleTreePathBuildIncrementally(t *testing.T) {
	entries := to.LeafInputs()
	// Second tree: build incrementally.
	// First tree: build in one go.
	mt := makeEmptyTree()
	mt.AppendData(entries...)

	mt2 := makeEmptyTree()

	for i := uint64(0); i < 8; i++ {
		mt2.AppendData(entries[i])

		for j := uint64(0); j < i+1; j++ {
			p1, err := mt.InclusionProof(j, i+1)
			if err != nil {
				t.Fatalf("InclusionProof: %v", err)
			}
			p2, err := mt2.InclusionProof(j, mt2.Size())
			if err != nil {
				t.Fatalf("InclusionProof: %v", err)
			}

			if len(p1) != len(p2) {
				t.Errorf("Different path lengths %d %d", len(p1), len(p2))
				t.FailNow()
			}

			for j := 0; j < len(p2); j++ {
				if !bytes.Equal(p1[j], p2[j]) {
					t.Errorf("Path mismatch: %s %s", hex.EncodeToString(p1[j]),
						hex.EncodeToString(p2[j]))
				}
			}
		}

		for k := i + 2; k <= 9; k++ {
			if proof, err := mt.InclusionProof(k, i+1); err == nil {
				t.Errorf("Got non empty path unexpectedly: %d %d %d", i, k, len(proof))
			}
		}
	}
}

func TestTreeConsistencyProof(t *testing.T) {
	entries := to.LeafInputs()
	mt := makeEmptyTree()
	mt.AppendData(entries...)
	validateTree(t, mt, 8)

	if _, err := mt.ConsistencyProof(6, 3); err == nil {
		t.Error("ConsistencyProof(6, 3) succeeded unexpectedly")
	}

	for size1 := uint64(0); size1 <= 8; size1++ {
		for size2 := size1; size2 <= 8; size2++ {
			t.Run(fmt.Sprintf("%d:%d", size1, size2), func(t *testing.T) {
				got, err := mt.ConsistencyProof(size1, size2)
				if err != nil {
					t.Fatalf("ConsistencyProof: %v", err)
				}
				want := refConsistencyProof(entries[:size2], size2, size1, mt.hasher, true)
				if diff := cmp.Diff(got, want, cmpopts.EquateEmpty()); diff != "" {
					t.Errorf("ConsistencyProof: diff (-got +want)\n%s", diff)
				}
			})
		}
	}
}
