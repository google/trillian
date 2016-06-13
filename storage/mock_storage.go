package storage

// This is a mock implementation of LogStorage for testing.

import (
	"sort"

	"github.com/google/trillian"
	"github.com/stretchr/testify/mock"
)

// MockLogStorage is a mock version of LogStorage
type MockLogStorage struct {
	mock.Mock
}

type MockTreeTX struct {
	mock.Mock
}

// MockLogTX is a mock version of LogTX
type MockLogTX struct {
	MockTreeTX
}

// MockReadOnlyLogTX is a mock version of ReadOnlyLogTX
type MockReadOnlyLogTX struct {
	MockTreeTX
}

// Begin is a mock
func (s MockLogStorage) Begin() (LogTX, error) {
	args := s.Called()
	return args.Get(0).(LogTX), args.Error(1)
}

// Snapshot is a mock
func (s MockLogStorage) Snapshot() (ReadOnlyLogTX, error) {
	args := s.Called()

	return args.Get(0).(ReadOnlyLogTX), args.Error(1)
}

// Commit is a mock
func (t *MockLogTX) Commit() error {
	args := t.Called()

	return args.Error(0)
}

// Rollback is a mock
func (t *MockLogTX) Rollback() error {
	args := t.Called()

	return args.Error(0)
}

// GetMerkleNodes is a mock
func (t *MockTreeTX) GetMerkleNodes(treeRevision int64, ids []NodeID) ([]Node, error) {
	args := t.Called(treeRevision, ids)

	return args.Get(0).([]Node), args.Error(1)
}

type by func(n1, n2 *Node) bool

func (by by) sort(nodes []Node) {
	ns := &nodeSorter{nodes: nodes, by: by}
	sort.Sort(ns)
}

type nodeSorter struct {
	nodes []Node
	by    func(n1, n2 *Node) bool
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

// SetMerkleNodes is a mock
func (t *MockTreeTX) SetMerkleNodes(treeRevision int64, nodes []Node) error {
	// We need a stable order to match the mock expectations so we sort them by
	// prefix len before passing them to the mock library. Might need extending
	// if we have more complex tests.
	prefixLen := func(n1, n2 *Node) bool {
		return n1.NodeID.PrefixLenBits < n2.NodeID.PrefixLenBits
	}

	by(prefixLen).sort(nodes)

	args := t.Called(nodes, treeRevision)

	return args.Error(0)
}

// QueueLeaves is a mock
func (t *MockLogTX) QueueLeaves(leaves []trillian.LogLeaf) error {
	args := t.Called(leaves)

	return args.Error(0)
}

// DequeueLeaves is a mock
func (t *MockLogTX) DequeueLeaves(limit int) ([]trillian.LogLeaf, error) {
	args := t.Called(limit)

	return args.Get(0).([]trillian.LogLeaf), args.Error(1)
}

// UpdateSequencedLeaves is a mock
func (t *MockLogTX) UpdateSequencedLeaves(leaves []trillian.LogLeaf) error {
	args := t.Called(leaves)

	return args.Error(0)
}

// GetSequencedLeafCount is a mock
func (t *MockLogTX) GetSequencedLeafCount() (int64, error) {
	args := t.Called()

	return args.Get(0).(int64), args.Error(1)
}

// GetLeavesByIndex is a mock
func (t *MockLogTX) GetLeavesByIndex(leaves []int64) ([]trillian.LogLeaf, error) {
	args := t.Called(leaves)

	return args.Get(0).([]trillian.LogLeaf), args.Error(1)
}

// GetLeavesByHash is a mock
func (t *MockTreeTX) GetLeavesByHash(leafHashes []trillian.Hash) ([]trillian.LogLeaf, error) {
	args := t.Called(leafHashes)

	return args.Get(0).([]trillian.LogLeaf), args.Error(1)
}

// LatestSignedLogRoot is a mock
func (t *MockLogTX) LatestSignedLogRoot() (trillian.SignedLogRoot, error) {
	args := t.Called()

	return args.Get(0).(trillian.SignedLogRoot), args.Error(1)
}

// StoreSignedLogRoot is a mock
func (t *MockLogTX) StoreSignedLogRoot(root trillian.SignedLogRoot) error {
	args := t.Called(root)

	return args.Error(0)
}
