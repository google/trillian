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

package merkle

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"

	"github.com/google/trillian/storage"
)

// For more information about how Sparse Merkle Trees work see the Revocation Transparency
// paper in the docs directory. Note that applications are not limited to X.509 certificates
// and this implementation handles arbitrary data.

// SparseMerkleTreeReader knows how to read data from a TreeStorage transaction
// to provide proofs etc.
type SparseMerkleTreeReader struct {
	tx           storage.ReadOnlyTreeTX
	hasher       MapHasher
	treeRevision int64
}

type newTXFunc func() (storage.TreeTX, error)

// SparseMerkleTreeWriter knows how to store/update a stored sparse Merkle tree
// via a TreeStorage transaction.
type SparseMerkleTreeWriter struct {
	hasher       MapHasher
	treeRevision int64
	tree         Subtree
}

type indexAndHash struct {
	index []byte
	hash  []byte
}

// rootHashOrError represents a (sub-)tree root hash, or an error which
// prevented the calculation from completing.
type rootHashOrError struct {
	hash []byte
	err  error
}

// Subtree is an interface which must be implemented by subtree workers.
// Currently there's only a locally sharded go-routine based implementation,
// the the idea is that an RPC based sharding implementation could be created
// and dropped in.
type Subtree interface {
	// SetLeaf sets a single leaf hash for integration into a sparse Merkle tree.
	SetLeaf(ctx context.Context, index []byte, hash []byte) error

	// CalculateRoot instructs the subtree worker to start calculating the root
	// hash of its tree.  It is an error to call SetLeaf() after calling this
	// method.
	CalculateRoot()

	// RootHash returns the calculated root hash for this subtree, if the root
	// hash has not yet been calculated, this method will block until it is.
	RootHash() ([]byte, error)
}

// getSubtreeFunc is essentially a factory method for getting child subtrees.
type getSubtreeFunc func(ctx context.Context, prefix []byte) (Subtree, error)

// subtreeWriter knows how to calculate and store nodes for a subtree.
type subtreeWriter struct {
	// prefix is the path to the root of this subtree in the full tree.
	// i.e. all paths/indices under this tree share the same prefix.
	prefix []byte

	// subtreeDepth is the number of levels this subtree contains.
	subtreeDepth int

	// leafQueue is the work-queue containing leaves to be integrated into the
	// subtree.
	leafQueue chan func() (*indexAndHash, error)

	// root is channel of size 1 from which the subtree root can be read once it
	// has been calculated.
	root chan rootHashOrError

	// childMutex protexts access to children.
	childMutex sync.RWMutex

	// children is a map of child-subtrees by stringified prefix.
	children map[string]Subtree

	tx           storage.TreeTX
	treeRevision int64

	treeHasher TreeHasher

	getSubtree getSubtreeFunc
}

// getOrCreateChildSubtree returns, or creates and returns, a subtree for the
// specified childPrefix.
func (s *subtreeWriter) getOrCreateChildSubtree(ctx context.Context, childPrefix []byte) (Subtree, error) {
	// TODO(al): figure out we actually need these copies and remove them if not.
	//           If we do then tidy up with a copyBytes helper.
	cp := append(make([]byte, 0, len(childPrefix)), childPrefix...)
	childPrefixStr := string(cp)
	s.childMutex.Lock()
	defer s.childMutex.Unlock()

	subtree := s.children[childPrefixStr]
	var err error
	if subtree == nil {
		subtree, err = s.getSubtree(ctx, cp)
		if err != nil {
			return nil, err
		}
		s.children[childPrefixStr] = subtree

		// Since a new subtree worker is being created we'll add a future to
		// to the leafQueue such that calculation of *this* subtree's root will
		// incorporate the newly calculated child subtree root.
		s.leafQueue <- func() (*indexAndHash, error) {
			// RootHash blocks until the root is available (or it's errored out)
			h, err := subtree.RootHash()
			if err != nil {
				return nil, err
			}
			return &indexAndHash{
				index: cp,
				hash:  h,
			}, nil
		}
	}
	return subtree, nil
}

// SetLeaf sets a single leaf hash for incorporation into the sparse Merkle
// tree.
func (s *subtreeWriter) SetLeaf(ctx context.Context, index []byte, hash []byte) error {
	indexLen := len(index) * 8

	switch {
	case indexLen < s.subtreeDepth:
		return fmt.Errorf("index length %d is < our depth %d", indexLen, s.subtreeDepth)

	case indexLen > s.subtreeDepth:
		childPrefix := index[:s.subtreeDepth/8]
		subtree, err := s.getOrCreateChildSubtree(ctx, childPrefix)
		if err != nil {
			return err
		}

		return subtree.SetLeaf(ctx, index[s.subtreeDepth/8:], hash)

	case indexLen == s.subtreeDepth:
		s.leafQueue <- func() (*indexAndHash, error) { return &indexAndHash{index: index, hash: hash}, nil }
		return nil
	}

	return fmt.Errorf("internal logic error in SetLeaf. index length: %d, subtreeDepth: %d", indexLen, s.subtreeDepth)
}

// CalculateRoot initiates the process of calculating the subtree root.
// The leafQueue is closed.
func (s *subtreeWriter) CalculateRoot() {
	close(s.leafQueue)

	for _, v := range s.children {
		v.CalculateRoot()
	}
}

// RootHash returns the calculated subtree root hash, blocking if necessary.
func (s *subtreeWriter) RootHash() ([]byte, error) {
	r := <-s.root
	return r.hash, r.err
}

func nodeIDFromAddress(size int, prefix []byte, index *big.Int, depth int) storage.NodeID {
	switch {
	case depth < 0:
		panic(fmt.Errorf("logic error: cannot have depth (%d) < 0", depth))
	case depth == 0:
		return storage.NewEmptyNodeID(size * 8)
	}
	ib := index.Bytes()
	t := make([]byte, size)

	copy(t, prefix)
	copy(t[size-len(ib):], ib)
	n := storage.NewNodeIDFromHash(t)
	n.PrefixLenBits = len(prefix)*8 + depth
	return n
}

// buildSubtree is the worker function which calculates the root hash.
// The root chan will have had exactly one entry placed in it, and have been
// subsequently closed when this method exits.
func (s *subtreeWriter) buildSubtree(ctx context.Context) {
	defer close(s.root)
	defer s.tx.Close()

	leaves := make([]HStar2LeafHash, 0, len(s.leafQueue))
	nodesToStore := make([]storage.Node, 0, len(s.leafQueue)*2)

	for leaf := range s.leafQueue {
		ih, err := leaf()
		if err != nil {
			s.root <- rootHashOrError{hash: nil, err: err}
			return
		}
		leaves = append(leaves, HStar2LeafHash{Index: new(big.Int).SetBytes(ih.index), LeafHash: ih.hash})
		nodesToStore = append(nodesToStore,
			storage.Node{
				NodeID:       storage.NewNodeIDFromHash(bytes.Join([][]byte{s.prefix, ih.index}, []byte{})),
				Hash:         ih.hash,
				NodeRevision: s.treeRevision,
			})

	}

	// calculate new root, and intermediate nodes:
	hs2 := NewHStar2(s.treeHasher)
	treeDepthOffset := (s.treeHasher.Size()-len(s.prefix))*8 - s.subtreeDepth
	addressSize := len(s.prefix) + s.subtreeDepth/8
	root, err := hs2.HStar2Nodes(s.subtreeDepth, treeDepthOffset, leaves,
		func(depth int, index *big.Int) ([]byte, error) {
			nodeID := nodeIDFromAddress(addressSize, s.prefix, index, depth)
			nodes, err := s.tx.GetMerkleNodes(ctx, s.treeRevision, []storage.NodeID{nodeID})
			if err != nil {
				return nil, err
			}
			if len(nodes) == 0 {
				return nil, nil
			}
			if expected, got := nodeID, nodes[0].NodeID; !expected.Equivalent(got) {
				return nil, fmt.Errorf("expected node ID %s from storage, but got %s", expected.String(), got.String())
			}
			if expected, got := s.treeRevision, nodes[0].NodeRevision; got > expected {
				return nil, fmt.Errorf("expected node revision <= %d, but got %d", expected, got)
			}
			return nodes[0].Hash, nil
		},
		func(depth int, index *big.Int, h []byte) error {
			// Don't store the root node of the subtree - that's part of the parent
			// tree.
			if depth == 0 && len(s.prefix) > 0 {
				return nil
			}
			nID := nodeIDFromAddress(addressSize, s.prefix, index, depth)
			nodesToStore = append(nodesToStore,
				storage.Node{
					NodeID:       nID,
					Hash:         h,
					NodeRevision: s.treeRevision,
				})
			return nil
		})
	if err != nil {
		s.root <- rootHashOrError{nil, err}
		return
	}

	// write nodes back to storage
	if err := s.tx.SetMerkleNodes(ctx, nodesToStore); err != nil {
		s.root <- rootHashOrError{nil, err}
		return
	}
	if err := s.tx.Commit(); err != nil {
		s.root <- rootHashOrError{nil, err}
		return
	}

	// send calculated root hash
	s.root <- rootHashOrError{root, nil}
}

var (
	// ErrNoSuchRevision is returned when a request is made for information about
	// a tree revision which does not exist.
	ErrNoSuchRevision = errors.New("no such revision")
)

// NewSparseMerkleTreeReader returns a new SparseMerkleTreeReader, reading at
// the specified tree revision, using the passed in MapHasher for calculating
// and verifying tree hashes read via tx.
func NewSparseMerkleTreeReader(rev int64, h MapHasher, tx storage.ReadOnlyTreeTX) *SparseMerkleTreeReader {
	return &SparseMerkleTreeReader{
		tx:           tx,
		hasher:       h,
		treeRevision: rev,
	}
}

func leafQueueSize(depths []int) int {
	if len(depths) == 1 {
		return 1024
	}
	// for higher levels make sure we've got enough space if all leaves turn out
	// to be sub-tree futures...
	return 1 << uint(depths[0])
}

// newLocalSubtreeWriter creates a new local go-routine based subtree worker.
func newLocalSubtreeWriter(ctx context.Context, rev int64, prefix []byte, depths []int, newTX newTXFunc, h TreeHasher) (Subtree, error) {
	tx, err := newTX()
	if err != nil {
		return nil, err
	}
	tree := subtreeWriter{
		treeRevision: rev,
		// TODO(al): figure out if we actually need these copies and remove it not.
		prefix:       append(make([]byte, 0, len(prefix)), prefix...),
		subtreeDepth: depths[0],
		leafQueue:    make(chan func() (*indexAndHash, error), leafQueueSize(depths)),
		root:         make(chan rootHashOrError, 1),
		children:     make(map[string]Subtree),
		tx:           tx,
		treeHasher:   h,
		getSubtree: func(ctx context.Context, p []byte) (Subtree, error) {
			myPrefix := bytes.Join([][]byte{prefix, p}, []byte{})
			return newLocalSubtreeWriter(ctx, rev, myPrefix, depths[1:], newTX, h)
		},
	}

	// TODO(al): probably shouldn't be spawning go routines willy-nilly like
	// this, but it'll do for now.
	go tree.buildSubtree(ctx)
	return &tree, nil
}

// NewSparseMerkleTreeWriter returns a new SparseMerkleTreeWriter, which will
// write data back into the tree at the specified revision, using the passed
// in MapHasher to calculate/verify tree hashes, storing via tx.
func NewSparseMerkleTreeWriter(ctx context.Context, rev int64, h MapHasher, newTX newTXFunc) (*SparseMerkleTreeWriter, error) {
	// TODO(al): allow the tree layering sizes to be customisable somehow.
	const topSubtreeSize = 8 // must be a multiple of 8 for now.
	tree, err := newLocalSubtreeWriter(ctx, rev, []byte{}, []int{topSubtreeSize, h.Size()*8 - topSubtreeSize}, newTX, h.TreeHasher)
	if err != nil {
		return nil, err
	}
	return &SparseMerkleTreeWriter{
		hasher:       h,
		tree:         tree,
		treeRevision: rev,
	}, nil
}

// RootAtRevision returns the sparse Merkle tree root hash at the specified
// revision, or ErrNoSuchRevision if the requested revision doesn't exist.
func (s SparseMerkleTreeReader) RootAtRevision(ctx context.Context, rev int64) ([]byte, error) {
	rootNodeID := storage.NewEmptyNodeID(256)
	nodes, err := s.tx.GetMerkleNodes(ctx, rev, []storage.NodeID{rootNodeID})
	if err != nil {
		return nil, err
	}
	switch {
	case len(nodes) == 0:
		return nil, ErrNoSuchRevision
	case len(nodes) > 1:
		return nil, fmt.Errorf("expected 1 node, but got %d", len(nodes))
	}
	// Sanity check the nodeID
	if !nodes[0].NodeID.Equivalent(rootNodeID) {
		return nil, fmt.Errorf("unexpected node returned with ID: %v", nodes[0].NodeID)
	}
	// Sanity check the revision
	if nodes[0].NodeRevision > rev {
		return nil, fmt.Errorf("unexpected node revision returned: %d > %d", nodes[0].NodeRevision, rev)
	}
	return nodes[0].Hash, nil
}

// InclusionProof returns an inclusion (or non-inclusion) proof for the
// specified key at the specified revision.
// If the revision does not exist it will return ErrNoSuchRevision error.
func (s SparseMerkleTreeReader) InclusionProof(ctx context.Context, rev int64, index []byte) ([][]byte, error) {
	nid := storage.NewNodeIDFromHash(index)
	sibs := nid.Siblings()
	nodes, err := s.tx.GetMerkleNodes(ctx, rev, sibs)
	if err != nil {
		return nil, err
	}

	nodeMap := make(map[string]*storage.Node)
	for _, n := range nodes {
		n := n // need this or we'll end up with the same node hash repeated in the map
		nodeMap[n.NodeID.String()] = &n
	}

	// We're building a full proof from a combination of whichever nodes we got
	// back from the storage layer, and the set of "null" hashes.
	r := make([][]byte, len(sibs), len(sibs))
	// For each proof element:
	for i := 0; i < len(r); i++ {
		proofID := sibs[i]
		pNode := nodeMap[proofID.String()]
		if pNode == nil {
			// we have no node for this level from storage, so the client will use
			// the null hash.
			continue
		}
		r[i] = pNode.Hash
		delete(nodeMap, proofID.String())
	}

	// Make sure we used up all the returned nodes, otherwise something's gone wrong.
	if remaining := len(nodeMap); remaining != 0 {
		return nil, fmt.Errorf("failed to consume all returned nodes; got %d nodes, but %d remain(s) unused", len(nodes), remaining)
	}
	return r, nil
}

// SetLeaves adds a batch of leaves to the in-flight tree update.
func (s *SparseMerkleTreeWriter) SetLeaves(ctx context.Context, leaves []HashKeyValue) error {
	for _, l := range leaves {
		if err := s.tree.SetLeaf(ctx, l.HashedKey, l.HashedValue); err != nil {
			return err
		}
	}
	return nil
}

// CalculateRoot calculates the new root hash including the newly added leaves.
func (s *SparseMerkleTreeWriter) CalculateRoot() ([]byte, error) {
	s.tree.CalculateRoot()
	return s.tree.RootHash()
}

// HashKeyValue represents a Hash(key)-Hash(value) pair.
type HashKeyValue struct {
	// HashedKey is the hash of the key data
	HashedKey []byte

	// HashedValue is the hash of the value data.
	HashedValue []byte
}
