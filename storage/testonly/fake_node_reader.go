// Copyright 2017 Google Inc. All Rights Reserved.
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

package testonly

import (
	"bytes"
	"context"
	"fmt"

	"github.com/golang/glog"
	"github.com/google/trillian/merkle/compact"
	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/storage"
)

// This is a fake implementation of a NodeReader intended for use in testing Merkle path code.
// Building node sets for tests by hand is onerous and error prone, especially when trying
// to test code reading from multiple tree revisions. It cannot live in the main testonly
// package as this creates import cycles.

// FakeNodeReader is an implementation of storage.NodeReader that's preloaded with a set of
// NodeID -> Node mappings and will return only those. Requesting any other nodes results in
// an error. For use in tests only, does not implement any other storage APIs.
type FakeNodeReader struct {
	treeSize     int64
	treeRevision int64
	nodeMap      map[string]storage.Node
}

// NewFakeNodeReader creates and returns a FakeNodeReader with the supplied nodes
// assuming that all the nodes are at a specified tree revision. All the node IDs
// must be distinct.
func NewFakeNodeReader(nodes []storage.Node, treeSize, treeRevision int64) *FakeNodeReader {
	nodeMap := make(map[string]storage.Node)

	for _, node := range nodes {
		id := node.NodeID.String()
		if _, ok := nodeMap[id]; ok {
			// Duplicate mapping - the test data is invalid so don't continue.
			glog.Fatalf("NewFakeNodeReader duplicate mapping for: %s in:\n%v", id, nodes)
		}
		nodeMap[id] = node
	}

	return &FakeNodeReader{nodeMap: nodeMap, treeSize: treeSize, treeRevision: treeRevision}
}

// GetTreeRevisionIncludingSize implements the corresponding NodeReader API.
func (f FakeNodeReader) GetTreeRevisionIncludingSize(treeSize int64) (int64, error) {
	if f.treeSize < treeSize {
		return int64(0), fmt.Errorf("GetTreeRevisionIncludingSize() got treeSize:%d, want: >= %d", treeSize, f.treeSize)
	}

	return f.treeRevision, nil
}

// GetMerkleNodes implements the corresponding NodeReader API.
func (f FakeNodeReader) GetMerkleNodes(treeRevision int64, NodeIDs []storage.NodeID) ([]storage.Node, error) {
	if f.treeRevision > treeRevision {
		return nil, fmt.Errorf("GetMerkleNodes() got treeRevision:%d, want up to: %d", treeRevision, f.treeRevision)
	}

	nodes := make([]storage.Node, 0, len(NodeIDs))
	for _, nodeID := range NodeIDs {
		node, ok := f.nodeMap[nodeID.String()]

		if !ok {
			return nil, fmt.Errorf("GetMerkleNodes() unknown node ID: %v", nodeID)
		}

		nodes = append(nodes, node)
	}

	return nodes, nil
}

func (f FakeNodeReader) hasID(nodeID storage.NodeID) bool {
	_, ok := f.nodeMap[nodeID.String()]
	return ok
}

// MultiFakeNodeReader can provide nodes at multiple revisions. It delegates to a number of
// FakeNodeReaders, each set up to handle one revision.
type MultiFakeNodeReader struct {
	readers []FakeNodeReader
}

// LeafBatch describes a set of leaves to be loaded into a MultiFakeNodeReader via a compact
// merkle tree. As each batch is added to the tree a set of node updates are collected
// and recorded in a FakeNodeReader for that revision. The expected root should be the
// result of calling CurrentRoot() on the compact Merkle tree encoded by hex.EncodeToString().
type LeafBatch struct {
	TreeRevision int64
	Leaves       []string
	ExpectedRoot []byte
}

// NewMultiFakeNodeReader creates a MultiFakeNodeReader delegating to a number of FakeNodeReaders
func NewMultiFakeNodeReader(readers []FakeNodeReader) *MultiFakeNodeReader {
	return &MultiFakeNodeReader{readers: readers}
}

// NewMultiFakeNodeReaderFromLeaves uses a compact Merkle tree to set up the nodes at various
// revisions. It collates all node updates from a batch of leaf data into one FakeNodeReader.
// This has the advantage of not needing to manually create all the data structures but the
// disadvantage is that a bug in the compact tree could be reflected in test using this
// code. To help guard against this we check the tree root hash after each batch has been
// processed. The supplied batches should be in ascending order of tree revision.
func NewMultiFakeNodeReaderFromLeaves(batches []LeafBatch) *MultiFakeNodeReader {
	tree := compact.NewTree(rfc6962.DefaultHasher)
	readers := make([]FakeNodeReader, 0, len(batches))

	lastBatchRevision := int64(0)
	for _, batch := range batches {
		if batch.TreeRevision <= lastBatchRevision {
			glog.Fatalf("Batches out of order revision: %d, last: %d in:\n%v", batch.TreeRevision,
				lastBatchRevision, batches)
		}

		lastBatchRevision = batch.TreeRevision
		nodeMap := make(map[string]storage.Node)
		for _, leaf := range batch.Leaves {
			// We're only interested in the side effects of adding leaves - the node updates
			tree.AddLeaf([]byte(leaf), func(depth int, index int64, hash []byte) error {
				nID, err := storage.NewNodeIDForTreeCoords(int64(depth), index, 64)

				if err != nil {
					return fmt.Errorf("failed to create a nodeID for tree - should not happen d:%d i:%d",
						depth, index)
				}

				nodeMap[nID.String()] = storage.Node{NodeID: nID, NodeRevision: batch.TreeRevision, Hash: hash}
				return nil
			})
		}

		// Sanity check the tree root hash against the one we expect to see.
		if got, want := tree.CurrentRoot(), batch.ExpectedRoot; !bytes.Equal(got, want) {
			panic(fmt.Errorf("NewMultiFakeNodeReaderFromLeaves() got root: %x, want: %x (%v)", got, want, batch))
		}

		// Unroll the update map to []storage.Node to retain the most recent node update within
		// the batch for each ID. Use that to create a new FakeNodeReader.
		nodes := make([]storage.Node, 0, len(nodeMap))
		for _, node := range nodeMap {
			nodes = append(nodes, node)
		}

		readers = append(readers, *NewFakeNodeReader(nodes, tree.Size(), batch.TreeRevision))
	}

	return NewMultiFakeNodeReader(readers)
}

func (m MultiFakeNodeReader) readerForNodeID(nodeID storage.NodeID, revision int64) *FakeNodeReader {
	// Work backwards and use the first reader where the node is present and the revision is in range
	for i := len(m.readers) - 1; i >= 0; i-- {
		if m.readers[i].treeRevision <= revision && m.readers[i].hasID(nodeID) {
			return &m.readers[i]
		}
	}

	return nil
}

// GetTreeRevisionIncludingSize implements the corresponding NodeReader API.
func (m MultiFakeNodeReader) GetTreeRevisionIncludingSize(treeSize int64) (int64, int64, error) {
	for i := len(m.readers) - 1; i >= 0; i-- {
		if m.readers[i].treeSize >= treeSize {
			return m.readers[i].treeRevision, m.readers[i].treeSize, nil
		}
	}

	return int64(0), int64(0), fmt.Errorf("want revision for tree size: %d but it doesn't exist", treeSize)
}

// GetMerkleNodes implements the corresponding NodeReader API.
func (m MultiFakeNodeReader) GetMerkleNodes(ctx context.Context, treeRevision int64, NodeIDs []storage.NodeID) ([]storage.Node, error) {
	// Find the correct reader for the supplied tree revision. This must be done for each node
	// as earlier revisions may still be relevant
	nodes := make([]storage.Node, 0, len(NodeIDs))
	for _, nID := range NodeIDs {
		reader := m.readerForNodeID(nID, treeRevision)

		if reader == nil {
			return nil,
				fmt.Errorf("want nodeID: %v with revision <= %d but no reader has it\n%v", nID, treeRevision, m)
		}

		node, err := reader.GetMerkleNodes(treeRevision, []storage.NodeID{nID})
		if err != nil {
			return nil, err
		}

		nodes = append(nodes, node[0])
	}

	return nodes, nil
}
