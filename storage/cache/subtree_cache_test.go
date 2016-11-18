package cache

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io/ioutil"
	"reflect"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/storagepb"
	"github.com/google/trillian/testonly"
)

var splitTestVector = []struct {
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
}

var defaultLogStrata = []int{8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8}
var defaultMapStrata = []int{8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 176}

func TestSplitNodeID(t *testing.T) {
	c := NewSubtreeCache(defaultMapStrata, PopulateMapSubtreeNodes(merkle.NewRFC6962TreeHasher(crypto.NewSHA256())))
	for i, v := range splitTestVector {
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
	c := NewSubtreeCache(defaultLogStrata, PopulateMapSubtreeNodes(merkle.NewRFC6962TreeHasher(crypto.NewSHA256())))

	nodeID := storage.NewNodeIDFromHash([]byte("1234"))
	// When we loop around asking for all 0..32 bit prefix lengths of the above
	// NodeID, we should see just one "Get" request for each subtree.
	si := 0
	for b := 0; b < nodeID.PrefixLenBits; b += defaultLogStrata[si] {
		e := nodeID
		e.PrefixLenBits = b
		m.EXPECT().GetSubtree(testonly.NodeIDEq(e)).Return(&storagepb.SubtreeProto{
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
	c := NewSubtreeCache(defaultLogStrata, PopulateMapSubtreeNodes(merkle.NewRFC6962TreeHasher(crypto.NewSHA256())))

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
		m.EXPECT().GetSubtree(testonly.NodeIDEq(nodeID)).Return(&storagepb.SubtreeProto{
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

func noFetch(id storage.NodeID) (*storagepb.SubtreeProto, error) {
	return nil, errors.New("not supposed to read anything")
}

func TestCacheFlush(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	m := NewMockNodeStorage(mockCtrl)
	c := NewSubtreeCache(defaultMapStrata, PopulateMapSubtreeNodes(merkle.NewRFC6962TreeHasher(crypto.NewSHA256())))

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
		m.EXPECT().GetSubtree(testonly.NodeIDEq(e)).Do(func(n storage.NodeID) {
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

func TestSuffixSerializeFormat(t *testing.T) {
	s := Suffix{5, []byte{0xae}}
	if got, want := s.serialize(), "Ba4="; got != want {
		t.Fatalf("Got serialized suffix of %s, expected %s", got, want)
	}
}

func TestRepopulateMapSubtreeKAT(t *testing.T) {
	hasher := merkle.NewRFC6962TreeHasher(crypto.NewSHA256())
	populateTheThing := PopulateMapSubtreeNodes(hasher)
	pb, err := ioutil.ReadFile("../../testdata/map_good_subtree.pb")
	if err != nil {
		t.Fatalf("failed to read test data: %v", err)
	}
	goodSubtree := storagepb.SubtreeProto{}
	if err := proto.UnmarshalText(string(pb), &goodSubtree); err != nil {
		t.Fatalf("failed to unmarshal SubtreeProto: %v", err)
	}

	leavesOnly := storagepb.SubtreeProto{}
	if err := proto.UnmarshalText(string(pb), &leavesOnly); err != nil {
		t.Fatalf("failed to unmarshal SubtreeProto: %v", err)
	}
	// erase the internal nodes
	leavesOnly.InternalNodes = make(map[string][]byte)

	if err := populateTheThing(&leavesOnly); err != nil {
		t.Fatalf("failed to repopulate subtree: %v", err)
	}
	if got, want := []byte(leavesOnly.RootHash), []byte(goodSubtree.RootHash); !bytes.Equal(got, want) {
		t.Errorf("recalculated incorrect root: got %v, wanted %v", got, want)
	}
	if got, want := len(leavesOnly.InternalNodes), len(goodSubtree.InternalNodes); got != want {
		t.Errorf("recalculated tree has %d internal nodes, expected %d", got, want)
	}

	for k, v := range goodSubtree.InternalNodes {
		h, ok := leavesOnly.InternalNodes[k]
		if !ok {
			t.Errorf("Reconstructed tree missing internal node for %v", k)
			continue
		}
		if got, want := h, v; !bytes.Equal(got, want) {
			t.Errorf("Recalculated incorrect hash for node %v, got %v expected %v", k, got, want)
		}
		delete(leavesOnly.InternalNodes, k)
	}
	if numExtraNodes := len(leavesOnly.InternalNodes); numExtraNodes > 0 {
		t.Errorf("Reconstructed tree has %d unexpected extra nodes:", numExtraNodes)
		for k := range leavesOnly.InternalNodes {
			rk, err := base64.StdEncoding.DecodeString(k)
			if err != nil {
				t.Errorf("  invalid base64: %v", err)
				continue
			}
			t.Errorf("  %v (%v)", k, rk)
		}
	}
}

func TestRepopulateLogSubtree(t *testing.T) {
	hasher := merkle.NewRFC6962TreeHasher(crypto.NewSHA256())
	populateTheThing := PopulateLogSubtreeNodes(hasher)
	cmt := merkle.NewCompactMerkleTree(hasher)
	cmtStorage := storagepb.SubtreeProto{
		Leaves:        make(map[string][]byte),
		InternalNodes: make(map[string][]byte),
	}
	s := storagepb.SubtreeProto{
		Leaves: make(map[string][]byte),
	}
	c := NewSubtreeCache(defaultLogStrata, PopulateMapSubtreeNodes(merkle.NewRFC6962TreeHasher(crypto.NewSHA256())))
	for numLeaves := int64(1); numLeaves < 255; numLeaves++ {
		// clear internal nodes
		s.InternalNodes = make(map[string][]byte)

		leaf := []byte(fmt.Sprintf("this is leaf %d", numLeaves))
		leafHash := hasher.Digest(leaf)
		cmt.AddLeafHash(leafHash, func(depth int, index int64, h []byte) {
			n, err := storage.NewNodeIDForTreeCoords(int64(depth), index, 8)
			if err != nil {
				t.Fatalf("failed to create nodeID for cmt tree: %v", err)
			}
			// Don't store leaves or the subtree root in InternalNodes
			if depth > 0 && depth < 8 {
				_, sfx := c.splitNodeID(n)
				cmtStorage.InternalNodes[sfx.serialize()] = h
			}
		})

		sfx, err := makeSuffixKey(8, numLeaves-1)
		if err != nil {
			t.Fatalf("failed to create suffix key: %v", err)
		}
		s.Leaves[sfx] = leafHash
		cmtStorage.Leaves[sfx] = leafHash

		if err := populateTheThing(&s); err != nil {
			t.Fatalf("failed populate subtree: %v", err)
		}

		if got, expected := s.RootHash, cmt.CurrentRoot(); !bytes.Equal(got, expected) {
			t.Fatalf("Got root %v for tree size %d, expected %v. subtree:\n%#v", got, numLeaves, expected, s.String())
		}

		if !reflect.DeepEqual(cmtStorage.InternalNodes, s.InternalNodes) {
			t.Fatalf("(it %d) CMT internal nodes are\n%v, but sparse internal nodes are\n%v", numLeaves, cmtStorage.InternalNodes, s.InternalNodes)
		}
	}
}

type logKATData struct {
	File      string
	NumLeaves int
}

func TestRepopulateLogSubtreeKAT(t *testing.T) {
	testVector := []logKATData{
		{"log_good_subtree_5.pb", 5},
		{"log_good_subtree_55.pb", 55},
	}

	for _, k := range testVector {
		runLogSubtreeKAT(t, k)
	}
}

func runLogSubtreeKAT(t *testing.T, data logKATData) {
	hasher := merkle.NewRFC6962TreeHasher(crypto.NewSHA256())
	populateTheThing := PopulateLogSubtreeNodes(hasher)
	pb, err := ioutil.ReadFile("../../testdata/" + data.File)
	if err != nil {
		t.Fatalf("failed to read test data: %v", err)
	}
	goodSubtree := storagepb.SubtreeProto{}
	if err := proto.UnmarshalText(string(pb), &goodSubtree); err != nil {
		t.Fatalf("failed to unmarshal SubtreeProto: %v", err)
	}
	t.Logf("good root %v", goodSubtree.RootHash)

	leavesOnly := storagepb.SubtreeProto{}
	if err := proto.UnmarshalText(string(pb), &leavesOnly); err != nil {
		t.Fatalf("failed to unmarshal SubtreeProto: %v", err)
	}
	// erase the internal nodes
	leavesOnly.InternalNodes = make(map[string][]byte)

	if err := populateTheThing(&leavesOnly); err != nil {
		t.Fatalf("failed to repopulate subtree: %v", err)
	}
	if got, want := leavesOnly.RootHash, goodSubtree.RootHash; !bytes.Equal(got, want) {
		t.Errorf("recalculated incorrect root: got %v, wanted %v", got, want)
	}
	if got, want := len(leavesOnly.InternalNodes), len(goodSubtree.InternalNodes); got != want {
		t.Errorf("recalculated tree has %d internal nodes, expected %d", got, want)
	}

	for k, v := range goodSubtree.InternalNodes {
		h, ok := leavesOnly.InternalNodes[k]
		if !ok {
			t.Errorf("Reconstructed tree missing internal node for %v", k)
			continue
		}
		if got, want := h, v; !bytes.Equal(got, want) {
			t.Errorf("Recalculated incorrect hash for node %v, got %v expected %v", k, got, want)
		}
		delete(leavesOnly.InternalNodes, k)
	}
	if numExtraNodes := len(leavesOnly.InternalNodes); numExtraNodes > 0 {
		t.Errorf("Reconstructed tree has %d unexpected extra nodes:", numExtraNodes)
		for k := range leavesOnly.InternalNodes {
			rk, err := base64.StdEncoding.DecodeString(k)
			if err != nil {
				t.Errorf("  invalid base64: %v", err)
				continue
			}
			t.Errorf("  %v (%v)", k, rk)
		}
	}
}

func TestPrefixLengths(t *testing.T) {
	strata := []int{8, 8, 16, 32, 64, 128}
	stratumInfo := []stratumInfo{{0, 8}, {1, 8}, {2, 16}, {2, 16}, {4, 32}, {4, 32}, {4, 32}, {4, 32}, {8, 64}, {8, 64}, {8, 64}, {8, 64}, {8, 64}, {8, 64}, {8, 64}, {8, 64}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}, {16, 128}}

	c := NewSubtreeCache(strata, PopulateMapSubtreeNodes(merkle.NewRFC6962TreeHasher(crypto.NewSHA256())))

	if got, want := c.stratumInfo, stratumInfo; !reflect.DeepEqual(got, want) {
		t.Fatalf("Got prefixLengths of %v, expected %v", got, want)
	}
}

func TestGetStratumInfo(t *testing.T) {
	c := NewSubtreeCache(defaultMapStrata, PopulateMapSubtreeNodes(merkle.NewRFC6962TreeHasher(crypto.NewSHA256())))
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
		if got, want := c.stratumInfoForPrefixLength(tv.depth), tv.info; !reflect.DeepEqual(got, want) {
			t.Errorf("(test %d for depth %d) got %#v, expected %#v", i, tv.depth, got, want)
		}
	}
}
