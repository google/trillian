package storage

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/mock"
)

var splitTestVector = []struct {
	inPath        []byte
	inPathLenBits int
	outPrefix     []byte
	outSuffix     []byte
}{
	{[]byte{0x12, 0x34, 0x56, 0x7f}, 32, []byte{0x12, 0x34, 0x56}, []byte{0x7f}},
	{[]byte{0x12, 0x34, 0x56, 0xff}, 29, []byte{0x12, 0x34, 0x56}, []byte{0xf8}},
	{[]byte{0x12, 0x34, 0x56, 0xff}, 25, []byte{0x12, 0x34, 0x56}, []byte{0x80}},
	{[]byte{0x12, 0x34, 0x56, 0x78}, 16, []byte{0x12}, []byte{0x34}},
	{[]byte{0x12, 0x34, 0x56, 0x78}, 9, []byte{0x12}, []byte{0x00}},
	{[]byte{0x12, 0x34, 0x56, 0x78}, 8, []byte{}, []byte{0x12}},
	{[]byte{0x12, 0x34, 0x56, 0x78}, 7, []byte{}, []byte{0x12}},
	{[]byte{0x12, 0x34, 0x56, 0x78}, 0, []byte{}, []byte{}},
}

func TestSplitNodeID(t *testing.T) {
	for i, v := range splitTestVector {
		n := NewNodeIDFromHash(v.inPath)
		n.PrefixLenBits = v.inPathLenBits

		p, s := splitNodeID(n)
		if expected, got := v.outPrefix, p; !bytes.Equal(expected, got) {
			t.Fatalf("(test %d) Expected prefix %v, got %v", i, expected, got)
		}

		if expected, got := v.inPathLenBits-len(v.outPrefix)*8, int(s.bits); expected != got {
			t.Fatalf("(test %d) Expected suffix num bits %d, got %d", i, expected, got)
		}

		if expected, got := v.outSuffix, s.path; !bytes.Equal(expected, got) {
			t.Fatalf("(test %d) Expected suffix path of %v, got %v", i, expected, got)
		}
	}
}

type mockNodeStorage struct {
	mock.Mock
}

func (m *mockNodeStorage) GetSubtree(n NodeID) (*SubtreeProto, error) {
	ret := m.Called(n)
	return ret.Get(0).(*SubtreeProto), ret.Error(1)
}

func (m *mockNodeStorage) SetSubtree(s *SubtreeProto) error {
	ret := m.Called(s)
	return ret.Error(0)
}

func TestCacheFillOnlyReadsSubtrees(t *testing.T) {
	m := mockNodeStorage{}
	c := NewSubtreeCache()

	nodeID := NewNodeIDFromHash([]byte("1234"))
	// When we loop around asking for all 0..32 bit prefix lengths of the above
	// NodeID, we should see just one "Get" request for each subtree.
	for b := 0; b < nodeID.PrefixLenBits; b += strataDepth {
		e := nodeID
		e.PrefixLenBits = b
		m.On("GetSubtree", mock.MatchedBy(func(n NodeID) bool {
			r := n.Equivalent(e)
			if r {
				t.Logf("saw %v", n)
			}
			return r
		})).Return(&SubtreeProto{
			Prefix: e.Path,
		}, nil)
	}

	for nodeID.PrefixLenBits > 0 {
		_, _, err := c.GetNodeHash(nodeID, m.GetSubtree)
		if err != nil {
			t.Fatalf("failed to get node hash: %v", err)
		}
		nodeID.PrefixLenBits--
	}
}

func TestCacheFlush(t *testing.T) {
	m := mockNodeStorage{}
	c := NewSubtreeCache()

	nodeID := NewNodeIDFromHash([]byte("1234"))
	// When we loop around asking for all 0..32 bit prefix lengths of the above
	// NodeID, we should see just one "Get" request for each subtree.
	for b := 0; b < nodeID.PrefixLenBits; b += strataDepth {
		e := NewNodeIDFromHash([]byte("1234"))
		//e := nodeID
		e.PrefixLenBits = b
		m.On("GetSubtree", mock.MatchedBy(func(n NodeID) bool {
			r := n.Equivalent(e)
			if r {
				t.Logf("read %v", n)
			}
			return r
		})).Return((*SubtreeProto)(nil), nil)
		m.On("SetSubtree", mock.MatchedBy(func(s *SubtreeProto) bool {
			e := e
			subID := NewNodeIDFromHash(s.Prefix)
			r := subID.Equivalent(e)
			if r {
				t.Logf("write %v -> (%d hashes)", subID, len(s.Nodes))
			}
			return r
		})).Return(nil)
	}

	// Read nodes which touch the subtrees we'll write to:
	sibs := nodeID.Siblings()
	for s := range sibs {
		_, _, err := c.GetNodeHash(sibs[s], m.GetSubtree)
		if err != nil {
			t.Fatalf("failed to get node hash: %v", err)
		}
	}

	t.Logf("after sibs: %v", nodeID)

	// Write nodes
	for nodeID.PrefixLenBits > 0 {
		h := []byte(nodeID.String())
		err := c.SetNodeHash(nodeID, 100, append([]byte("hash-"), h...))
		if err != nil {
			t.Fatalf("failed to set node hash: %v", err)
		}
		nodeID.PrefixLenBits--
	}

	if err := c.Flush(m.SetSubtree); err != nil {
		t.Fatalf("failed to flush cache: %v", err)
	}
}
