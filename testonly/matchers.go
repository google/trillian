package testonly

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/golang/mock/gomock"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/storagepb"
)

type subtreeHasPrefix struct {
	expectedID storage.NodeID
}

func (s subtreeHasPrefix) Matches(x interface{}) bool {
	st, ok := x.(*storagepb.SubtreeProto)
	if !ok {
		return false
	}
	subID := storage.NewNodeIDFromHash(st.Prefix)
	return subID.Equivalent(s.expectedID)
}

func (s subtreeHasPrefix) String() string {
	return fmt.Sprintf("has prefix %v", s.expectedID)
}

// SubtreeHasPrefix returns a gomock matcher which returns true when it finds
// a subtree whose prefix matches the one passed in.
func SubtreeHasPrefix(n storage.NodeID) gomock.Matcher {
	return subtreeHasPrefix{n}
}

type nodeIDEq struct {
	expectedID storage.NodeID
}

func (m nodeIDEq) Matches(x interface{}) bool {
	n, ok := x.(storage.NodeID)
	if !ok {
		return false
	}
	return n.Equivalent(m.expectedID)
}

func (m nodeIDEq) String() string {
	return fmt.Sprintf("is %s", m.expectedID.String())
}

// NodeIDEq returns a matcher that expects the specified NodeID.
func NodeIDEq(n storage.NodeID) gomock.Matcher {
	return nodeIDEq{n}
}

// We need a stable order to match the mock expectations so we sort them by
// prefix len before passing them to the mock library. Might need extending
// if we have more complex tests.
func prefixLen(n1, n2 *storage.Node) bool {
	return n1.NodeID.PrefixLenBits < n2.NodeID.PrefixLenBits
}

// NodeSet returns a matcher that expects the given set of Nodes.
func NodeSet(nodes []storage.Node) gomock.Matcher {
	by(prefixLen).sort(nodes)
	return nodeSet{nodes}
}

type nodeSet struct {
	other []storage.Node
}

func (n nodeSet) Matches(x interface{}) bool {
	nodes, ok := x.([]storage.Node)
	if !ok {
		return false
	}
	by(prefixLen).sort(nodes)
	return reflect.DeepEqual(nodes, n.other)
}

func (n nodeSet) String() string {
	return fmt.Sprintf("equivalent to %v", n.other)
}

// Node sorting boilerplate below.

type by func(n1, n2 *storage.Node) bool

func (by by) sort(nodes []storage.Node) {
	ns := &nodeSorter{nodes: nodes, by: by}
	sort.Sort(ns)
}

type nodeSorter struct {
	nodes []storage.Node
	by    func(n1, n2 *storage.Node) bool
}

func (n *nodeSorter) Len() int {
	return len(n.nodes)
}

func (n *nodeSorter) Swap(i, j int) {
	n.nodes[i], n.nodes[j] = n.nodes[j], n.nodes[i]
}

func (n *nodeSorter) Less(i, j int) bool {
	return n.by(&n.nodes[i], &n.nodes[j])
}

// End sorting boilerplate.
