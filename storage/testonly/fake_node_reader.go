// Copyright 2017 Google LLC. All Rights Reserved.
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
	"github.com/google/trillian/storage/tree"
	"github.com/transparency-dev/merkle/compact"
	"github.com/transparency-dev/merkle/rfc6962"
)

// This is a fake implementation of a NodeReader intended for use in testing Merkle path code.
// Building node sets for tests by hand is onerous and error prone, especially when trying
// to test code reading from multiple tree revisions. It cannot live in the main testonly
// package as this creates import cycles.

// FakeNodeReader is an implementation of storage.NodeReader that's preloaded with a set of
// NodeID -> Node mappings and will return only those. Requesting any other nodes results in
// an error. For use in tests only, does not implement any other storage APIs.
type FakeNodeReader struct {
	nodeMap map[compact.NodeID]tree.Node
}

// NewFakeNodeReader creates and returns a FakeNodeReader with the supplied nodes
// assuming that all the nodes are at a specified tree revision. All the node IDs
// must be distinct.
func NewFakeNodeReader(nodes []tree.Node) *FakeNodeReader {
	nodeMap := make(map[compact.NodeID]tree.Node)

	for _, node := range nodes {
		id := node.ID
		if _, ok := nodeMap[id]; ok {
			// Duplicate mapping - the test data is invalid so don't continue.
			glog.Fatalf("NewFakeNodeReader duplicate mapping for: %+v in:\n%v", id, nodes)
		}
		nodeMap[id] = node
	}

	return &FakeNodeReader{nodeMap: nodeMap}
}

// GetMerkleNodes implements the corresponding NodeReader API.
func (f FakeNodeReader) GetMerkleNodes(ids []compact.NodeID) ([]tree.Node, error) {
	nodes := make([]tree.Node, 0, len(ids))
	for _, id := range ids {
		node, ok := f.nodeMap[id]
		if !ok {
			return nil, fmt.Errorf("GetMerkleNodes() unknown node ID: %v", id)
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func (f FakeNodeReader) hasID(id compact.NodeID) bool {
	_, ok := f.nodeMap[id]
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
	hasher := rfc6962.DefaultHasher
	fact := compact.RangeFactory{Hash: hasher.HashChildren}
	cr := fact.NewEmptyRange(0)

	readers := make([]FakeNodeReader, 0, len(batches))

	lastBatchRevision := int64(0)
	for _, batch := range batches {
		if batch.TreeRevision <= lastBatchRevision {
			glog.Fatalf("Batches out of order revision: %d, last: %d in:\n%v", batch.TreeRevision,
				lastBatchRevision, batches)
		}

		lastBatchRevision = batch.TreeRevision
		nodeMap := make(map[compact.NodeID][]byte)
		store := func(id compact.NodeID, hash []byte) { nodeMap[id] = hash }
		for _, leaf := range batch.Leaves {
			hash := hasher.HashLeaf([]byte(leaf))
			// Store the new leaf node, and all new perfect nodes.
			store(compact.NewNodeID(0, cr.End()), hash)
			if err := cr.Append(hash, store); err != nil {
				panic(fmt.Errorf("Append: %v", err))
			}
		}
		// TODO(pavelkalinnikov): Use testing.T.Fatalf instead of panics.
		root, err := cr.GetRootHash(nil)
		if err != nil {
			panic(fmt.Errorf("GetRootHash: %v", err))
		}
		if cr.End() == 0 {
			root = hasher.EmptyRoot()
		}
		// Sanity check the tree root hash against the one we expect to see.
		if got, want := root, batch.ExpectedRoot; !bytes.Equal(got, want) {
			panic(fmt.Errorf("NewMultiFakeNodeReaderFromLeaves() got root: %x, want: %x (%v)", got, want, batch))
		}

		// Unroll the update map to []tree.Node to retain the most recent node update within
		// the batch for each ID. Use that to create a new FakeNodeReader.
		nodes := make([]tree.Node, 0, len(nodeMap))
		for id, hash := range nodeMap {
			nodes = append(nodes, tree.Node{ID: id, Hash: hash})
		}

		readers = append(readers, *NewFakeNodeReader(nodes))
	}

	return NewMultiFakeNodeReader(readers)
}

func (m MultiFakeNodeReader) readerForNodeID(id compact.NodeID) *FakeNodeReader {
	// Work backwards and use the first reader where the node is present.
	for i := len(m.readers) - 1; i >= 0; i-- {
		if m.readers[i].hasID(id) {
			return &m.readers[i]
		}
	}
	return nil
}

// GetMerkleNodes implements the corresponding NodeReader API.
func (m MultiFakeNodeReader) GetMerkleNodes(ctx context.Context, ids []compact.NodeID) ([]tree.Node, error) {
	// Find the correct reader for the supplied tree revision. This must be done for each node
	// as earlier revisions may still be relevant
	nodes := make([]tree.Node, 0, len(ids))
	for _, id := range ids {
		reader := m.readerForNodeID(id)

		if reader == nil {
			return nil,
				fmt.Errorf("want nodeID %v, but no reader has it\n%v", id, m)
		}

		node, err := reader.GetMerkleNodes([]compact.NodeID{id})
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node[0])
	}
	return nodes, nil
}
