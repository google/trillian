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

package cache

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian/merkle/compact"
	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/storage/storagepb"
	"github.com/google/trillian/storage/tree"

	"github.com/golang/mock/gomock"
)

func ancestor(id compact.NodeID, levelsUp uint) compact.NodeID {
	return compact.NewNodeID(id.Level+levelsUp, id.Index>>levelsUp)
}

func toPrefix(t *testing.T, id compact.NodeID) []byte {
	t.Helper()
	if level := id.Level; level%8 != 0 || level > 64 {
		t.Fatalf("node %+v is not aligned", id)
	}
	var bytes [8]byte
	binary.BigEndian.PutUint64(bytes[:], id.Index<<id.Level)
	return bytes[:8-id.Level/8]
}

func TestCacheFillOnlyReadsSubtrees(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	m := NewMockNodeStorage(mockCtrl)
	c := NewLogSubtreeCache(rfc6962.DefaultHasher)

	id := compact.NewNodeID(28, 0x112233445)
	// When we loop around asking for all parents of the above NodeID, we should
	// see just one "Get" request for each tile.
	for id := ancestor(id, 4); id.Level <= 64; id = ancestor(id, 8) {
		prefix := toPrefix(t, id)
		m.EXPECT().GetSubtree(prefix).Return(&storagepb.SubtreeProto{
			Depth:  logStrataDepth,
			Prefix: prefix,
		}, nil)
	}

	for id := id; id.Level < 64; id = ancestor(id, 1) {
		if _, err := c.getNodeHash(id, m.GetSubtree); err != nil {
			t.Fatalf("failed to get node hash: %v", err)
		}
	}
}

func TestCacheGetNodesReadsSubtrees(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	m := NewMockNodeStorage(mockCtrl)
	c := NewLogSubtreeCache(rfc6962.DefaultHasher)

	ids := []compact.NodeID{
		compact.NewNodeID(0, 0x1234),
		compact.NewNodeID(0, 0x1235),
		compact.NewNodeID(0, 0x4567),
		compact.NewNodeID(0, 0x89ab),
		compact.NewNodeID(0, 0x89ac),
		compact.NewNodeID(0, 0x89ad),
	}

	// Test that node IDs from one subtree are collapsed into one stratum read.
	skips := map[int]bool{1: true, 4: true, 5: true}

	// Set up the expected reads. We expect one subtree read per entry in ids,
	// except for the ones in the skips map.
	for i, id := range ids {
		if skips[i] {
			continue
		}
		// And it'll be for the prefix of the full node ID (with the default log
		// strata that'll be everything except the last byte).
		prefix := toPrefix(t, ancestor(id, 8))
		m.EXPECT().GetSubtree(prefix).Return(&storagepb.SubtreeProto{
			Prefix: prefix,
		}, nil)
	}

	// Now request the nodes:
	_, err := c.GetNodes(
		ids,
		// Glue function to convert a call requesting multiple subtrees into a
		// sequence of calls to our mock storage:
		func(ids [][]byte) ([]*storagepb.SubtreeProto, error) {
			ret := make([]*storagepb.SubtreeProto, 0)
			for _, id := range ids {
				r, err := m.GetSubtree(id)
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

func noFetch(_ []byte) (*storagepb.SubtreeProto, error) {
	return nil, errors.New("not supposed to read anything")
}

func TestCacheFlush(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	m := NewMockNodeStorage(mockCtrl)
	c := NewLogSubtreeCache(rfc6962.DefaultHasher)

	id := compact.NewNodeID(0, 12345)
	expectedSetIDs := make(map[string]string)
	// When we loop around asking for all 0..64 bit prefix lengths of the above
	// NodeID, we should see just one "Get" request for each subtree.
	for id := ancestor(id, 8); id.Level <= 64; id = ancestor(id, 8) {
		prefix := toPrefix(t, id)
		expectedSetIDs[string(prefix)] = "expected"
		m.EXPECT().GetSubtree(prefix).Do(func(id []byte) {
			t.Logf("read %x", id)
		}).Return((*storagepb.SubtreeProto)(nil), nil)
	}
	m.EXPECT().SetSubtrees(gomock.Any(), gomock.Any()).Do(func(ctx context.Context, trees []*storagepb.SubtreeProto) {
		for _, s := range trees {
			if got, want := s.Depth, int32(8); got != want {
				t.Errorf("Got subtree with depth %d, expected %d for prefix %x", got, want, s.Prefix)
			}
			state, ok := expectedSetIDs[string(s.Prefix)]
			if !ok {
				t.Errorf("Unexpected write to subtree %x", s.Prefix)
			}
			switch state {
			case "expected":
				expectedSetIDs[string(s.Prefix)] = "met"
			case "met":
				t.Errorf("Second write to subtree %x", s.Prefix)
			default:
				t.Errorf("Unknown state for subtree %x: %s", s.Prefix, state)
			}
			t.Logf("write %x -> (%d leaves)", s.Prefix, len(s.Leaves))
		}
	}).Return(nil)

	// Write nodes.
	for id := id; id.Level < 64; id = ancestor(id, 1) {
		err := c.SetNodeHash(id, []byte(fmt.Sprintf("hash-%v", id)), m.GetSubtree)
		if err != nil {
			t.Fatalf("failed to set node hash: %v", err)
		}
	}

	if err := c.Flush(ctx, m.SetSubtrees); err != nil {
		t.Fatalf("failed to flush cache: %v", err)
	}

	for k, v := range expectedSetIDs {
		switch v {
		case "expected":
			t.Errorf("Subtree %x remains unset", k)
		case "met":
			//
		default:
			t.Errorf("Unknown state for subtree %x: %s", k, v)
		}
	}
}

func TestRepopulateLogSubtree(t *testing.T) {
	fact := compact.RangeFactory{Hash: rfc6962.DefaultHasher.HashChildren}
	cr := fact.NewEmptyRange(0)
	cmtStorage := storagepb.SubtreeProto{
		Leaves:        make(map[string][]byte),
		InternalNodes: make(map[string][]byte),
		Depth:         8,
	}
	s := storagepb.SubtreeProto{
		Leaves: make(map[string][]byte),
		Depth:  8,
	}
	for numLeaves := int64(1); numLeaves <= 256; numLeaves++ {
		// clear internal nodes
		s.InternalNodes = make(map[string][]byte)

		leaf := []byte(fmt.Sprintf("this is leaf %d", numLeaves))
		leafHash := rfc6962.DefaultHasher.HashLeaf(leaf)
		store := func(id compact.NodeID, hash []byte) {
			// Don't store leaves or the subtree root in InternalNodes
			if id.Level > 0 && id.Level < 8 {
				_, sfx := tree.Split(id)
				cmtStorage.InternalNodes[sfx.String()] = hash
			}
		}
		if err := cr.Append(leafHash, store); err != nil {
			t.Fatalf("merkle tree update failed: %v", err)
		}

		sfxKey := toSuffix(compact.NewNodeID(0, uint64(numLeaves)-1))
		s.Leaves[sfxKey] = leafHash
		if numLeaves == 256 {
			s.InternalNodeCount = uint32(len(cmtStorage.InternalNodes))
		} else {
			s.InternalNodeCount = 0
		}
		cmtStorage.Leaves[sfxKey] = leafHash

		if err := PopulateLogTile(&s, rfc6962.DefaultHasher); err != nil {
			t.Fatalf("failed populating tile: %v", err)
		}

		// Repopulation should only have happened with a full subtree, otherwise the internal nodes map
		// should be empty
		if numLeaves != 256 {
			if len(s.InternalNodes) != 0 {
				t.Fatalf("(it %d) internal nodes should be empty but got: %v", numLeaves, s.InternalNodes)
			}
		} else if diff := cmp.Diff(cmtStorage.InternalNodes, s.InternalNodes); diff != "" {
			t.Fatalf("(it %d) CMT/sparse internal nodes diff:\n%v", numLeaves, diff)
		}
	}
}

func BenchmarkRepopulateLogSubtree(b *testing.B) {
	hasher := rfc6962.DefaultHasher
	s := storagepb.SubtreeProto{
		Leaves:            make(map[string][]byte),
		Depth:             8,
		InternalNodeCount: 254,
	}
	for i := 0; i < 256; i++ {
		leaf := []byte(fmt.Sprintf("leaf %d", i))
		hash := hasher.HashLeaf(leaf)
		s.Leaves[toSuffix(compact.NewNodeID(0, uint64(i)))] = hash
	}

	for n := 0; n < b.N; n++ {
		if err := PopulateLogTile(&s, hasher); err != nil {
			b.Fatalf("failed populating tile: %v", err)
		}
	}
}

func TestIdempotentWrites(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	m := NewMockNodeStorage(mockCtrl)

	// Note: The ID must end with a zero byte, to be the first leaf in the tile.
	id := compact.NewNodeID(24, 0x12300)
	subtreePrefix := toPrefix(t, ancestor(id, 8))

	expectedSetIDs := make(map[string]string)
	expectedSetIDs[string(subtreePrefix)] = "expected"

	// The first time we read the subtree we'll emulate an empty subtree:
	m.EXPECT().GetSubtree(subtreePrefix).Do(func(id []byte) {
		t.Logf("read %x", id)
	}).Return((*storagepb.SubtreeProto)(nil), nil)

	// We should only see a single write attempt
	m.EXPECT().SetSubtrees(gomock.Any(), gomock.Any()).Times(1).Do(func(ctx context.Context, trees []*storagepb.SubtreeProto) {
		for _, s := range trees {
			state, ok := expectedSetIDs[string(s.Prefix)]
			if !ok {
				t.Errorf("Unexpected write to subtree %x", s.Prefix)
			}
			switch state {
			case "expected":
				expectedSetIDs[string(s.Prefix)] = "met"
			case "met":
				t.Errorf("Second write to subtree %x", s.Prefix)
			default:
				t.Errorf("Unknown state for subtree %x: %s", s.Prefix, state)
			}

			// After this write completes, subsequent reads will see the subtree
			// being written now:
			m.EXPECT().GetSubtree(s.Prefix).AnyTimes().Do(func(id []byte) {
				t.Logf("read again %x", id)
			}).Return(s, nil)

			t.Logf("write %x -> %#v", s.Prefix, s)
		}
	}).Return(nil)

	// Now write the same value to the same node multiple times.
	// We should see many reads, but only the first call to SetNodeHash should
	// result in an actual write being flushed through to storage.
	for i := 0; i < 10; i++ {
		c := NewLogSubtreeCache(rfc6962.DefaultHasher)
		if _, err := c.getNodeHash(id, m.GetSubtree); err != nil {
			t.Fatalf("%d: failed to get node hash: %v", i, err)
		}
		if err := c.SetNodeHash(id, []byte("noodled"), noFetch); err != nil {
			t.Fatalf("%d: failed to set node hash: %v", i, err)
		}
		if err := c.Flush(ctx, m.SetSubtrees); err != nil {
			t.Fatalf("%d: failed to flush cache: %v", i, err)
		}
	}

	for k, v := range expectedSetIDs {
		switch v {
		case "expected":
			t.Errorf("Subtree %x remains unset", k)
		case "met":
			//
		default:
			t.Errorf("Unknown state for subtree %x: %s", k, v)
		}
	}
}
