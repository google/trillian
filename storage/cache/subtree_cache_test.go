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
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/trillian/merkle/compact"
	"github.com/google/trillian/merkle/maphasher"
	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/storage/storagepb"
	"github.com/google/trillian/storage/tree"

	"github.com/golang/mock/gomock"
	"github.com/kylelemons/godebug/pretty"

	stestonly "github.com/google/trillian/storage/testonly"
)

var (
	defaultLogStrata = []int{8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8}
	defaultMapStrata = []int{8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 176}
)

const treeID = int64(0)

func TestCacheFillOnlyReadsSubtrees(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	m := NewMockNodeStorage(mockCtrl)
	c := NewSubtreeCache(defaultLogStrata, populateMapSubtreeNodes(treeID, maphasher.Default), prepareMapSubtreeWrite())

	nodeID := tree.NewNodeIDFromHash([]byte("1234"))
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
		_, err := c.getNodeHash(nodeID, m.GetSubtree)
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
	c := NewSubtreeCache(defaultLogStrata, populateMapSubtreeNodes(treeID, maphasher.Default), prepareMapSubtreeWrite())

	nodeIDs := []tree.NodeID{
		tree.NewNodeIDFromHash([]byte("1234")),
		tree.NewNodeIDFromHash([]byte("1235")),
		tree.NewNodeIDFromHash([]byte("4567")),
		tree.NewNodeIDFromHash([]byte("89ab")),
		tree.NewNodeIDFromHash([]byte("89ac")),
		tree.NewNodeIDFromHash([]byte("89ad")),
	}
	// Test that node IDs from one subtree are collapsed into one stratum read.
	skips := map[int]bool{1: true, 4: true, 5: true}

	// Set up the expected reads. We expect one subtree read per entry in
	// nodeIDs, except for the ones in the skips map.
	for i, nodeID := range nodeIDs {
		if skips[i] {
			continue
		}
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
		func(ids []tree.NodeID) ([]*storagepb.SubtreeProto, error) {
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
		t.Errorf("getNodeHash(_, _) = _, %v", err)
	}
}

func noFetch(_ tree.NodeID) (*storagepb.SubtreeProto, error) {
	return nil, errors.New("not supposed to read anything")
}

func TestCacheFlush(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	m := NewMockNodeStorage(mockCtrl)
	c := NewSubtreeCache(defaultMapStrata, populateMapSubtreeNodes(treeID, maphasher.Default), prepareMapSubtreeWrite())

	h := "0123456789abcdef0123456789abcdef"
	nodeID := tree.NewNodeIDFromHash([]byte(h))
	expectedSetIDs := make(map[string]string)
	// When we loop around asking for all 0..32 bit prefix lengths of the above
	// NodeID, we should see just one "Get" request for each subtree.
	si := -1
	for b := 0; b < nodeID.PrefixLenBits; b += defaultMapStrata[si] {
		si++
		e := nodeID
		e.PrefixLenBits = b
		expectedSetIDs[e.String()] = "expected"
		m.EXPECT().GetSubtree(stestonly.NodeIDEq(e)).Do(func(n tree.NodeID) {
			t.Logf("read %v", n)
		}).Return((*storagepb.SubtreeProto)(nil), nil)
	}
	m.EXPECT().SetSubtrees(gomock.Any(), gomock.Any()).Do(func(ctx context.Context, trees []*storagepb.SubtreeProto) {
		for _, s := range trees {
			rootID := tree.NewNodeIDFromHash(s.Prefix)
			if got, want := s.Depth, c.layout.TileHeight(rootID.PrefixLenBits); got != int32(want) {
				t.Errorf("Got subtree with depth %d, expected %d for prefixLen %d", got, want, rootID.PrefixLenBits)
			}
			state, ok := expectedSetIDs[rootID.String()]
			if !ok {
				t.Errorf("Unexpected write to subtree %s", rootID.String())
			}
			switch state {
			case "expected":
				expectedSetIDs[rootID.String()] = "met"
			case "met":
				t.Errorf("Second write to subtree %s", rootID.String())
			default:
				t.Errorf("Unknown state for subtree %s: %s", rootID.String(), state)
			}
			t.Logf("write %v -> (%d leaves)", rootID, len(s.Leaves))
		}
	}).Return(nil)

	// Read nodes which touch the subtrees we'll write to:
	sibs := nodeID.Siblings()
	for s := range sibs {
		_, err := c.getNodeHash(sibs[s], m.GetSubtree)
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

	if err := c.Flush(ctx, m.SetSubtrees); err != nil {
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

func TestRepopulateLogSubtree(t *testing.T) {
	populateTheThing := populateLogSubtreeNodes(rfc6962.DefaultHasher)
	fact := compact.RangeFactory{Hash: rfc6962.DefaultHasher.HashChildren}
	cr := fact.NewEmptyRange(0)
	cmtStorage := storagepb.SubtreeProto{
		Leaves:        make(map[string][]byte),
		InternalNodes: make(map[string][]byte),
		Depth:         int32(defaultLogStrata[0]),
	}
	s := storagepb.SubtreeProto{
		Leaves: make(map[string][]byte),
		Depth:  int32(defaultLogStrata[0]),
	}
	c := NewSubtreeCache(defaultLogStrata, populateLogSubtreeNodes(rfc6962.DefaultHasher), prepareLogSubtreeWrite())
	for numLeaves := int64(1); numLeaves <= 256; numLeaves++ {
		// clear internal nodes
		s.InternalNodes = make(map[string][]byte)

		leaf := []byte(fmt.Sprintf("this is leaf %d", numLeaves))
		leafHash := rfc6962.DefaultHasher.HashLeaf(leaf)
		store := func(id compact.NodeID, hash []byte) {
			n := stestonly.MustCreateNodeIDForTreeCoords(int64(id.Level), int64(id.Index), 8)
			// Don't store leaves or the subtree root in InternalNodes
			if id.Level > 0 && id.Level < 8 {
				_, sfx := c.layout.Split(n)
				cmtStorage.InternalNodes[sfx.String()] = hash
			}
		}
		if err := cr.Append(leafHash, store); err != nil {
			t.Fatalf("merkle tree update failed: %v", err)
		}

		nodeID := tree.NewNodeIDFromPrefix(s.Prefix, logStrataDepth, numLeaves-1, logStrataDepth, maxLogDepth)
		_, sfx := nodeID.Split(len(s.Prefix), int(s.Depth))
		sfxKey := sfx.String()
		s.Leaves[sfxKey] = leafHash
		if numLeaves == 1<<uint(defaultLogStrata[0]) {
			s.InternalNodeCount = uint32(len(cmtStorage.InternalNodes))
		} else {
			s.InternalNodeCount = 0
		}
		cmtStorage.Leaves[sfxKey] = leafHash

		if err := populateTheThing(&s); err != nil {
			t.Fatalf("failed populate subtree: %v", err)
		}
		root, err := cr.GetRootHash(nil)
		if err != nil {
			t.Fatalf("GetRootHash: %v", err)
		}
		if got, expected := s.RootHash, root; !bytes.Equal(got, expected) {
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

func BenchmarkRepopulateLogSubtree(b *testing.B) {
	hasher := rfc6962.DefaultHasher
	s := storagepb.SubtreeProto{
		Leaves:            make(map[string][]byte),
		Depth:             int32(defaultLogStrata[0]),
		InternalNodeCount: 254,
	}
	for i := 0; i < 256; i++ {
		leaf := []byte(fmt.Sprintf("leaf %d", i))
		hash := hasher.HashLeaf(leaf)
		nodeID := tree.NewNodeIDFromPrefix(s.Prefix, logStrataDepth, int64(i), logStrataDepth, maxLogDepth)
		_, sfx := nodeID.Split(len(s.Prefix), int(s.Depth))
		s.Leaves[sfx.String()] = hash
	}

	populate := populateLogSubtreeNodes(hasher)
	for n := 0; n < b.N; n++ {
		if err := populate(&s); err != nil {
			b.Fatalf("failed populate subtree: %v", err)
		}
	}
}

func TestIdempotentWrites(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	m := NewMockNodeStorage(mockCtrl)

	h := "0123456789abcdef0123456789abcdef"
	nodeID := tree.NewNodeIDFromHash([]byte(h))
	nodeID.PrefixLenBits = 40
	subtreeID := nodeID
	subtreeID.PrefixLenBits = 32

	expectedSetIDs := make(map[string]string)
	expectedSetIDs[subtreeID.String()] = "expected"

	// The first time we read the subtree we'll emulate an empty subtree:
	m.EXPECT().GetSubtree(stestonly.NodeIDEq(subtreeID)).Do(func(n tree.NodeID) {
		t.Logf("read %v", n.String())
	}).Return((*storagepb.SubtreeProto)(nil), nil)

	// We should only see a single write attempt
	m.EXPECT().SetSubtrees(gomock.Any(), gomock.Any()).Times(1).Do(func(ctx context.Context, trees []*storagepb.SubtreeProto) {
		for _, s := range trees {
			subID := tree.NewNodeIDFromHash(s.Prefix)
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

			// After this write completes, subsequent reads will see the subtree
			// being written now:
			m.EXPECT().GetSubtree(stestonly.NodeIDEq(subID)).AnyTimes().Do(func(n tree.NodeID) {
				t.Logf("read again %v", n.String())
			}).Return(s, nil)

			t.Logf("write %v -> %#v", subID.String(), s)
		}
	}).Return(nil)

	// Now write the same value to the same node multiple times.
	// We should see many reads, but only the first call to SetNodeHash should
	// result in an actual write being flushed through to storage.
	for i := 0; i < 10; i++ {
		c := NewSubtreeCache(defaultMapStrata, populateMapSubtreeNodes(treeID, maphasher.Default), prepareMapSubtreeWrite())
		_, err := c.getNodeHash(nodeID, m.GetSubtree)
		if err != nil {
			t.Fatalf("%d: failed to get node hash: %v", i, err)
		}

		err = c.SetNodeHash(nodeID, []byte("noodled"), noFetch)
		if err != nil {
			t.Fatalf("%d: failed to set node hash: %v", i, err)
		}

		if err := c.Flush(ctx, m.SetSubtrees); err != nil {
			t.Fatalf("%d: failed to flush cache: %v", i, err)
		}
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
