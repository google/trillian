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

package cache

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/merkle/maphasher"
	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/storagepb"
	stestonly "github.com/google/trillian/storage/testonly"
	"github.com/kylelemons/godebug/pretty"
)

var defaultLogStrata = []int{8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8}
var defaultMapStrata = []int{8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 176}

const treeID = int64(0)

func TestSplitNodeID(t *testing.T) {
	c := NewSubtreeCache(defaultMapStrata, PopulateMapSubtreeNodes(treeID, maphasher.Default), PrepareMapSubtreeWrite())
	for i, v := range []struct {
		inPath        []byte
		inPathLenBits int
		outPrefix     []byte
		outSuffixBits int
		outSuffix     []byte
	}{
		{[]byte{0x12, 0x34, 0x56, 0x7f}, 32, []byte{0x12, 0x34, 0x56}, 8, []byte{0x7f}},
		{[]byte{0x12, 0x34, 0x56, 0xff}, 29, []byte{0x12, 0x34, 0x56}, 5, []byte{0xf8}},
		{[]byte{0x12, 0x34, 0x56, 0xff}, 25, []byte{0x12, 0x34, 0x56}, 1, []byte{0x80}},
		{[]byte{0x12, 0x34, 0x56, 0x78}, 16, []byte{0x12}, 8, []byte{0x34}},
		{[]byte{0x12, 0x34, 0x56, 0x78}, 9, []byte{0x12}, 1, []byte{0x00}},
		{[]byte{0x12, 0x34, 0x56, 0x78}, 8, []byte{}, 8, []byte{0x12}},
		{[]byte{0x12, 0x34, 0x56, 0x78}, 7, []byte{}, 7, []byte{0x12}},
		{[]byte{0x12, 0x34, 0x56, 0x78}, 0, []byte{}, 0, []byte{0}},
		{[]byte{0x70}, 2, []byte{}, 2, []byte{0x40}},
		{[]byte{0x70}, 3, []byte{}, 3, []byte{0x60}},
		{[]byte{0x70}, 4, []byte{}, 4, []byte{0x70}},
		{[]byte{0x70}, 5, []byte{}, 5, []byte{0x70}},
		{[]byte{0x00, 0x03}, 16, []byte{0x00}, 8, []byte{0x03}},
		{[]byte{0x00, 0x03}, 15, []byte{0x00}, 7, []byte{0x02}},
	} {
		n := storage.NewNodeIDFromHash(v.inPath)
		n.PrefixLenBits = v.inPathLenBits

		p, s := c.splitNodeID(n)
		if expected, got := v.outPrefix, p; !bytes.Equal(expected, got) {
			t.Fatalf("(test %d) Expected prefix %x, got %x", i, expected, got)
		}

		if expected, got := v.outSuffixBits, int(s.bits); expected != got {
			t.Fatalf("(test %d) Expected suffix num bits %d, got %d", i, expected, got)
		}

		if expected, got := v.outSuffix, s.path; !bytes.Equal(expected, got) {
			t.Fatalf("(test %d) Expected suffix path of %x, got %x", i, expected, got)
		}
	}
}

func TestCacheFillOnlyReadsSubtrees(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	m := NewMockNodeStorage(mockCtrl)
	c := NewSubtreeCache(defaultLogStrata, PopulateMapSubtreeNodes(treeID, maphasher.Default), PrepareMapSubtreeWrite())

	nodeID := storage.NewNodeIDFromHash([]byte("1234"))
	// When we loop around asking for all 0..32 bit prefix lengths of the above
	// NodeID, we should see just one "Get" request for each subtree.
	si := 0
	for b := 0; b < nodeID.PrefixLenBits; b += defaultLogStrata[si] {
		e := nodeID
		e.PrefixLenBits = b
		m.EXPECT().GetSubtree(stestonly.NodeIDEq(e)).Return(&storagepb.SubtreeProto{
			Prefix: e.Path,
		}, nil)
		si++
	}

	for nodeID.PrefixLenBits > 0 {
		_, err := c.GetNodeHash(nodeID, m.GetSubtree)
		if err != nil {
			t.Fatalf("failed to get node hash: %v", err)
		}
		nodeID.PrefixLenBits--
	}
}

func TestCacheGetNodesReadsSubtrees(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	m := NewMockNodeStorage(mockCtrl)
	c := NewSubtreeCache(defaultLogStrata, PopulateMapSubtreeNodes(treeID, maphasher.Default), PrepareMapSubtreeWrite())

	nodeIDs := []storage.NodeID{
		storage.NewNodeIDFromHash([]byte("1234")),
		storage.NewNodeIDFromHash([]byte("4567")),
		storage.NewNodeIDFromHash([]byte("89ab")),
	}

	// Set up the expected reads:
	// We expect one subtree read per entry in nodeIDs
	for _, nodeID := range nodeIDs {
		nodeID := nodeID
		// And it'll be for the prefix of the full node ID (with the default log
		// strata that'll be everything except the last byte), so modify the prefix
		// length here accoringly:
		nodeID.PrefixLenBits -= 8
		m.EXPECT().GetSubtree(stestonly.NodeIDEq(nodeID)).Return(&storagepb.SubtreeProto{
			Prefix: nodeID.Path[:len(nodeID.Path)-1],
		}, nil)
	}

	// Now request the nodes:
	_, err := c.GetNodes(
		nodeIDs,
		// Glue function to convert a call requesting multiple subtrees into a
		// sequence of calls to our mock storage:
		func(ids []storage.NodeID) ([]*storagepb.SubtreeProto, error) {
			ret := make([]*storagepb.SubtreeProto, 0)
			for _, i := range ids {
				r, err := m.GetSubtree(i)
				if err != nil {
					return nil, err
				}
				if r != nil {
					ret = append(ret, r)
				}
			}
			return ret, nil
		})
	if err != nil {
		t.Errorf("GetNodeHash(_, _) = _, %v", err)
	}
}

func noFetch(storage.NodeID) (*storagepb.SubtreeProto, error) {
	return nil, errors.New("not supposed to read anything")
}

func TestCacheFlush(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	m := NewMockNodeStorage(mockCtrl)
	c := NewSubtreeCache(defaultMapStrata, PopulateMapSubtreeNodes(treeID, maphasher.Default), PrepareMapSubtreeWrite())

	h := "0123456789abcdef0123456789abcdef"
	nodeID := storage.NewNodeIDFromHash([]byte(h))
	expectedSetIDs := make(map[string]string)
	// When we loop around asking for all 0..32 bit prefix lengths of the above
	// NodeID, we should see just one "Get" request for each subtree.
	si := -1
	for b := 0; b < nodeID.PrefixLenBits; b += defaultMapStrata[si] {
		si++
		e := storage.NewNodeIDFromHash([]byte(h))
		//e := nodeID
		e.PrefixLenBits = b
		expectedSetIDs[e.String()] = "expected"
		m.EXPECT().GetSubtree(stestonly.NodeIDEq(e)).Do(func(n storage.NodeID) {
			t.Logf("read %v", n)
		}).Return((*storagepb.SubtreeProto)(nil), nil)
	}
	m.EXPECT().SetSubtrees(gomock.Any()).Do(func(trees []*storagepb.SubtreeProto) {
		for _, s := range trees {
			subID := storage.NewNodeIDFromHash(s.Prefix)
			if got, want := s.Depth, c.stratumInfoForPrefixLength(subID.PrefixLenBits).depth; got != int32(want) {
				t.Errorf("Got subtree with depth %d, expected %d for prefixLen %d", got, want, subID.PrefixLenBits)
			}
			state, ok := expectedSetIDs[subID.String()]
			if !ok {
				t.Errorf("Unexpected write to subtree %s", subID.String())
			}
			switch state {
			case "expected":
				expectedSetIDs[subID.String()] = "met"
			case "met":
				t.Errorf("Second write to subtree %s", subID.String())
			default:
				t.Errorf("Unknown state for subtree %s: %s", subID.String(), state)
			}
			t.Logf("write %v -> (%d leaves)", subID, len(s.Leaves))
		}
	}).Return(nil)

	// Read nodes which touch the subtrees we'll write to:
	sibs := nodeID.Siblings()
	for s := range sibs {
		_, err := c.GetNodeHash(sibs[s], m.GetSubtree)
		if err != nil {
			t.Fatalf("failed to get node hash: %v", err)
		}
	}

	t.Logf("after sibs: %v", nodeID)

	// Write nodes
	for nodeID.PrefixLenBits > 0 {
		h := []byte(nodeID.String())
		err := c.SetNodeHash(nodeID, append([]byte("hash-"), h...), noFetch)
		if err != nil {
			t.Fatalf("failed to set node hash: %v", err)
		}
		nodeID.PrefixLenBits--
	}

	if err := c.Flush(m.SetSubtrees); err != nil {
		t.Fatalf("failed to flush cache: %v", err)
	}

	for k, v := range expectedSetIDs {
		switch v {
		case "expected":
			t.Errorf("Subtree %s remains unset", k)
		case "met":
			//
		default:
			t.Errorf("Unknown state for subtree %s: %s", k, v)
		}
	}
}

func TestSuffixKey(t *testing.T) {
	for _, tc := range []struct {
		depth   int
		index   int64
		want    []byte
		wantErr bool
	}{
		{depth: 0, index: 0x00, want: h2b("0000"), wantErr: false},
		{depth: 8, index: 0x00, want: h2b("0800"), wantErr: false},
		// TODO(gdbelvin): want: "0fabcd"?
		{depth: 8, index: 0xabcd, want: h2b("08cd"), wantErr: false},
		// TODO(gdbelvin): want "0240"
		{
			depth:   2,
			index:   new(big.Int).SetBytes(h2b("4000000000000000000000000000000000000000000000000000000000000000")).Int64(),
			want:    h2b("0200"),
			wantErr: false,
		},
		{depth: 15, index: 0xab, want: h2b("0fab"), wantErr: false},
		{depth: 16, index: 0x00, want: h2b("1000"), wantErr: false},
	} {
		suffixKey, err := makeSuffixKey(tc.depth, tc.index)
		if got, want := err != nil, tc.wantErr; got != want {
			t.Errorf("makeSuffixKey(%v, %v): %v, want err: %v",
				tc.depth, tc.index, err, want)
			continue
		}
		if err != nil {
			continue
		}
		b, err := base64.StdEncoding.DecodeString(suffixKey)
		if err != nil {
			t.Errorf("DecodeString(%v): %v", suffixKey, err)
			continue
		}
		if got, want := b, tc.want; !bytes.Equal(got, want) {
			t.Errorf("makeSuffixKey(%v, %x): %x, want %x",
				tc.depth, tc.index, got, want)
		}
	}
}

func TestSuffixSerializeFormat(t *testing.T) {
	s := Suffix{5, []byte{0xae}}
	if got, want := s.serialize(), "Ba4="; got != want {
		t.Fatalf("Got serialized suffix of %s, expected %s", got, want)
	}
}

func TestRepopulateLogSubtree(t *testing.T) {
	populateTheThing := PopulateLogSubtreeNodes(rfc6962.DefaultHasher)
	cmt := merkle.NewCompactMerkleTree(rfc6962.DefaultHasher)
	cmtStorage := storagepb.SubtreeProto{
		Leaves:        make(map[string][]byte),
		InternalNodes: make(map[string][]byte),
		Depth:         int32(defaultLogStrata[0]),
	}
	s := storagepb.SubtreeProto{
		Leaves: make(map[string][]byte),
		Depth:  int32(defaultLogStrata[0]),
	}
	c := NewSubtreeCache(defaultLogStrata, PopulateLogSubtreeNodes(rfc6962.DefaultHasher), PrepareLogSubtreeWrite())
	for numLeaves := int64(1); numLeaves <= 256; numLeaves++ {
		// clear internal nodes
		s.InternalNodes = make(map[string][]byte)

		leaf := []byte(fmt.Sprintf("this is leaf %d", numLeaves))
		leafHash := rfc6962.DefaultHasher.HashLeaf(leaf)
		_, err := cmt.AddLeafHash(leafHash, func(depth int, index int64, h []byte) error {
			n, err := storage.NewNodeIDForTreeCoords(int64(depth), index, 8)
			if err != nil {
				return fmt.Errorf("failed to create nodeID for cmt tree: %v", err)
			}
			// Don't store leaves or the subtree root in InternalNodes
			if depth > 0 && depth < 8 {
				_, sfx := c.splitNodeID(n)
				cmtStorage.InternalNodes[sfx.serialize()] = h
			}
			return nil
		})
		if err != nil {
			t.Fatalf("merkle tree update failed: %v", err)
		}

		sfx, err := makeSuffixKey(8, numLeaves-1)
		if err != nil {
			t.Fatalf("failed to create suffix key: %v", err)
		}
		s.Leaves[sfx] = leafHash
		if numLeaves == 1<<uint(defaultLogStrata[0]) {
			s.InternalNodeCount = uint32(len(cmtStorage.InternalNodes))
		} else {
			s.InternalNodeCount = 0
		}
		cmtStorage.Leaves[sfx] = leafHash

		if err := populateTheThing(&s); err != nil {
			t.Fatalf("failed populate subtree: %v", err)
		}
		if got, expected := s.RootHash, cmt.CurrentRoot(); !bytes.Equal(got, expected) {
			t.Fatalf("Got root %v for tree size %d, expected %v. subtree:\n%#v", got, numLeaves, expected, s.String())
		}

		// Repopulation should only have happened with a full subtree, otherwise the internal nodes map
		// should be empty
		if numLeaves != 1<<uint(defaultLogStrata[0]) {
			if len(s.InternalNodes) != 0 {
				t.Fatalf("(it %d) internal nodes should be empty but got: %v", numLeaves, s.InternalNodes)
			}
		} else if diff := pretty.Compare(cmtStorage.InternalNodes, s.InternalNodes); diff != "" {
			t.Fatalf("(it %d) CMT/sparse internal nodes diff:\n%v", numLeaves, diff)
		}
	}
}

func TestPrefixLengths(t *testing.T) {
	strata := []int{8, 8, 16, 32, 64, 128}
	stratumInfo := []stratumInfo{{0, 8}, {1, 8}, {2, 16}, {2, 16}, {4, 32}, {4, 32}, {4, 32}, {4, 32}, {8, 64}, {8, 64}, {8, 64}, {8, 64}, {8, 64}, {8, 64}, {8, 64}, {8, 64}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}}

	c := NewSubtreeCache(strata, PopulateMapSubtreeNodes(treeID, maphasher.Default), PrepareMapSubtreeWrite())

	if diff := pretty.Compare(c.stratumInfo, stratumInfo); diff != "" {
		t.Fatalf("prefixLengths diff:\n%v", diff)
	}
}

func TestGetStratumInfo(t *testing.T) {
	c := NewSubtreeCache(defaultMapStrata, PopulateMapSubtreeNodes(treeID, maphasher.Default), PrepareMapSubtreeWrite())
	testVec := []struct {
		depth int
		info  stratumInfo
	}{
		{0, stratumInfo{0, 8}},
		{1, stratumInfo{0, 8}},
		{7, stratumInfo{0, 8}},
		{8, stratumInfo{1, 8}},
		{15, stratumInfo{1, 8}},
		{79, stratumInfo{9, 8}},
		{80, stratumInfo{10, 176}},
		{81, stratumInfo{10, 176}},
		{156, stratumInfo{10, 176}},
	}
	for i, tv := range testVec {
		if diff := pretty.Compare(c.stratumInfoForPrefixLength(tv.depth), tv.info); diff != "" {
			t.Errorf("(test %d for depth %d) diff:\n%v", i, tv.depth, diff)
		}
	}
}

// h2b converts a hex string into []byte.
func h2b(h string) []byte {
	b, err := hex.DecodeString(h)
	if err != nil {
		panic("invalid hex string")
	}
	return b
}
