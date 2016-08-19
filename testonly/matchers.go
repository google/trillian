package testonly

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/golang/mock/gomock"
	"github.com/google/trillian/storage"
)

// We need a stable order to match the mock expectations so we sort them by
// prefix len before passing them to the mock library. Might need extending
// if we have more complex tests.
func prefixLen(n1, n2 *storage.Node) bool {
	return n1.NodeID.PrefixLenBits < n2.NodeID.PrefixLenBits
}

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
