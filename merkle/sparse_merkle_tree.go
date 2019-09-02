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

	"github.com/golang/glog"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/storage"
)

// For more information about how Sparse Merkle Trees work see the Revocation Transparency
// paper in the docs directory. Note that applications are not limited to X.509 certificates
// and this implementation handles arbitrary data.

// SparseMerkleTreeReader knows how to read data from a TreeStorage transaction
// to provide proofs etc.
type SparseMerkleTreeReader struct {
	tx           storage.ReadOnlyTreeTX
	hasher       hashers.MapHasher
	treeRevision int64
}

// TXRunner supplies the RunTX function.
// TXRunner can be passed as the last argument to MapStorage.ReadWriteTransaction.
type TXRunner interface {
	// RunTX executes f and supplies a transaction object to operate on.
	RunTX(ctx context.Context, f func(context.Context, storage.MapTreeTX) error) error
}

// SparseMerkleTreeWriter knows how to store/update a stored sparse Merkle tree
// via a TreeStorage transaction.
type SparseMerkleTreeWriter struct {
	hasher       hashers.MapHasher
	treeRevision int64
	tree         Subtree
}

type indexAndHash struct {
	index []byte
	hash  []byte
}

type leafGenerator func() (*indexAndHash, error)

// rootHashOrError represents a (sub-)tree root hash, or an error which
// prevented the calculation from completing.
// TODO(gdbelvin): represent an empty subtree with a nil hash?
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
	CalculateRoot(ctx context.Context)

	// RootHash returns the calculated root hash for this subtree, if the root
	// hash has not yet been calculated, this method will block until it is.
	RootHash() ([]byte, error)
}

// subtreeWriter knows how to calculate and store nodes for a subtree.
type subtreeWriter struct {
	treeID int64
	// prefix is the path to the root of this subtree in the full tree.
	// i.e. all paths/indices under this tree share the same prefix.
	prefix []byte

	// subtreeDepth is the number of levels this subtree contains.
	subtreeDepth int
	// remainingDepths holds the levels below this subtree.
	remainingDepths []int

	// leafGeneratorQueue is the queue containing generators of leaves
	// to be integrated into the subtree.
	leafGeneratorQueue chan leafGenerator

	// root is channel of size 1 from which the subtree root can be read once it
	// has been calculated.
	root chan rootHashOrError

	// childMutex protects access to children.
	childMutex sync.RWMutex

	// children is a map of child-subtrees by stringified prefix.
	children map[string]Subtree

	txRunner     TXRunner
	treeRevision int64

	hasher hashers.MapHasher
}

func (s *subtreeWriter) newChildSubtreeWriter(ctx context.Context, p []byte) (Subtree, error) {
	myPrefix := bytes.Join([][]byte{s.prefix, p}, []byte{})
	return newLocalSubtreeWriter(ctx, s.treeID, s.treeRevision, myPrefix, s.remainingDepths, s.txRunner, s.hasher)
}

// getOrCreateChildSubtree returns, or creates and returns, a subtree for the
// specified childPrefix.
func (s *subtreeWriter) getOrCreateChildSubtree(ctx context.Context, childPrefix []byte) (Subtree, error) {
	childPrefixStr := string(childPrefix)
	s.childMutex.Lock()
	defer s.childMutex.Unlock()

	subtree := s.children[childPrefixStr]
	var err error
	if subtree == nil {
		subtree, err = s.newChildSubtreeWriter(ctx, childPrefix)
		if err != nil {
			return nil, err
		}
		s.children[childPrefixStr] = subtree

		// Since a new subtree worker is being created we'll add a future to
		// to the leafGeneratorQueue such that calculation of *this* subtree's root will
		// incorporate the newly calculated child subtree root.
		s.leafGeneratorQueue <- func() (*indexAndHash, error) {
			// RootHash blocks until the root is available (or it's errored out)
			h, err := subtree.RootHash()
			if err != nil {
				return nil, err
			}
			return &indexAndHash{
				index: childPrefix,
				hash:  h,
			}, nil
		}
	}
	return subtree, nil
}

// SetLeaf sets a single leaf hash for incorporation into the sparse Merkle tree.
// index is the full path of the leaf, starting from the root (not the subtree's root).
func (s *subtreeWriter) SetLeaf(ctx context.Context, index []byte, hash []byte) error {
	depth := len(index) * 8
	absSubtreeDepth := len(s.prefix)*8 + s.subtreeDepth

	switch {
	case depth < absSubtreeDepth:
		return fmt.Errorf("depth: %d, want >= %d", depth, absSubtreeDepth)

	case depth > absSubtreeDepth:
		childPrefix := index[:absSubtreeDepth/8]
		subtree, err := s.getOrCreateChildSubtree(ctx, childPrefix)
		if err != nil {
			return err
		}

		return subtree.SetLeaf(ctx, index, hash)

	default: // depth == absSubtreeDepth:
		s.leafGeneratorQueue <- func() (*indexAndHash, error) {
			return &indexAndHash{index: index, hash: hash}, nil
		}
		return nil
	}
}

// CalculateRoot initiates the process of calculating the subtree root.
// The leafGeneratorQueue is closed.
func (s *subtreeWriter) CalculateRoot(ctx context.Context) {
	close(s.leafGeneratorQueue)

	for _, v := range s.children {
		v.CalculateRoot(ctx)
	}
}

// RootHash returns the calculated subtree root hash, blocking if necessary.
func (s *subtreeWriter) RootHash() ([]byte, error) {
	r := <-s.root
	return r.hash, r.err
}

// buildSubtree is the worker function which calculates the root hash.
// The root chan will have had exactly one entry placed in it, and have been
// subsequently closed when this method exits.
func (s *subtreeWriter) buildSubtree(ctx context.Context, queueSize int) {
	defer close(s.root)
	var root []byte
	err := s.txRunner.RunTX(ctx, func(ctx context.Context, tx storage.MapTreeTX) error {
		root = []byte{}
		leaves := make([]*HStar2LeafHash, 0, queueSize)
		nodesToStore := make([]storage.Node, 0, queueSize*2)

		// sibs will hold the list of sibling node IDs for all nodes we'll end up
		// wanting to write - we'll use this to prewarm the subtree cache.
		var sibs []storage.NodeID

		// The go-routine will block here until the channel is closed via
		// CalculateRoot, at which point we can proceed with completing the
		// subtree building and calculation
		for leafGenerator := range s.leafGeneratorQueue {
			ih, err := leafGenerator()
			if err != nil {
				return err
			}
			nodeID := storage.NewNodeIDFromPrefixSuffix(ih.index, storage.EmptySuffix, s.hasher.BitLen())
			sibs = append(sibs, nodeID.Siblings()...)

			leaves = append(leaves, &HStar2LeafHash{
				Index:    nodeID.BigInt(),
				LeafHash: ih.hash,
			})
			nodesToStore = append(nodesToStore,
				storage.Node{
					NodeID:       nodeID,
					Hash:         ih.hash,
					NodeRevision: s.treeRevision,
				})
		}
		ctx, postQueueCloseEnd := spanFor(ctx, "buildSubtree.runTX.postQueueClose")
		defer postQueueCloseEnd()

		// Prewarm the cache:
		if _, err := tx.GetMerkleNodes(ctx, s.treeRevision, sibs); err != nil {
			return fmt.Errorf("failed to preload node hash cache: %s", err)
		}

		hsCtx, hstar2SpanEnd := spanFor(ctx, "buildSubtree.runTX.hstar2")
		// calculate new root, and intermediate nodes:
		hs2 := NewHStar2(s.treeID, s.hasher)
		var err error
		root, err = hs2.HStar2Nodes(s.prefix, s.subtreeDepth, leaves,
			func(depth int, index *big.Int) ([]byte, error) {
				nodeID := storage.NewNodeIDFromBigInt(depth, index, s.hasher.BitLen())
				if glog.V(4) {
					glog.Infof("buildSubtree.get(%x, %d) nid: %x, %v",
						index.Bytes(), depth, nodeID.Path, nodeID.PrefixLenBits)
				}
				nodes, err := tx.GetMerkleNodes(hsCtx, s.treeRevision, []storage.NodeID{nodeID})
				if err != nil {
					return nil, err
				}
				if len(nodes) == 0 {
					return nil, nil
				}
				if got, want := nodes[0].NodeID, nodeID; !got.Equivalent(want) {
					return nil, fmt.Errorf("got node %v from storage, want %v", got, want)
				}
				if got, want := nodes[0].NodeRevision, s.treeRevision; got > want {
					return nil, fmt.Errorf("got node revision %d, want <= %d", got, want)
				}
				return nodes[0].Hash, nil
			},
			func(depth int, index *big.Int, h []byte) error {
				// Don't store the root node of the subtree - that's part of the parent
				// tree.
				if depth == len(s.prefix)*8 && len(s.prefix) > 0 {
					return nil
				}
				nodeID := storage.NewNodeIDFromBigInt(depth, index, s.hasher.BitLen())
				if glog.V(4) {
					glog.Infof("buildSubtree.set(%x, %v) nid: %x, %v : %x",
						index.Bytes(), depth, nodeID.Path, nodeID.PrefixLenBits, h)
				}
				nodesToStore = append(nodesToStore,
					storage.Node{
						NodeID:       nodeID,
						Hash:         h,
						NodeRevision: s.treeRevision,
					})
				return nil
			})
		hstar2SpanEnd() // hstar2
		if err != nil {
			return err
		}

		// write nodes back to storage
		return tx.SetMerkleNodes(ctx, nodesToStore)
	})
	if err != nil {
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
func NewSparseMerkleTreeReader(rev int64, h hashers.MapHasher, tx storage.ReadOnlyTreeTX) *SparseMerkleTreeReader {
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
func newLocalSubtreeWriter(ctx context.Context, treeID, rev int64, prefix []byte, depths []int, txRunner TXRunner, h hashers.MapHasher) (Subtree, error) {
	tree := subtreeWriter{
		treeID:             treeID,
		treeRevision:       rev,
		prefix:             prefix,
		subtreeDepth:       depths[0],
		remainingDepths:    depths[1:],
		leafGeneratorQueue: make(chan leafGenerator, leafQueueSize(depths)),
		root:               make(chan rootHashOrError, 1),
		children:           make(map[string]Subtree),
		txRunner:           txRunner,
		hasher:             h,
	}

	// TODO(al): probably shouldn't be spawning go routines willy-nilly like
	// this, but it'll do for now.
	go tree.buildSubtree(ctx, leafQueueSize(depths))
	return &tree, nil
}

// NewSparseMerkleTreeWriter returns a new SparseMerkleTreeWriter, which will
// write data back into the tree at the specified revision, using the passed
// in MapHasher to calculate/verify tree hashes, storing via tx.
func NewSparseMerkleTreeWriter(ctx context.Context, treeID, rev int64, h hashers.MapHasher, txRunner TXRunner) (*SparseMerkleTreeWriter, error) {
	// TODO(al): allow the tree layering sizes to be customisable somehow.
	const topSubtreeSize = 8 // must be a multiple of 8 for now.
	tree, err := newLocalSubtreeWriter(ctx, treeID, rev, []byte{}, []int{topSubtreeSize, h.Size()*8 - topSubtreeSize}, txRunner, h)
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
	ctx, spanEnd := spanFor(ctx, "RootAtRevision")
	defer spanEnd()

	rootNodeID := storage.NewEmptyNodeID(256)
	nodes, err := s.tx.GetMerkleNodes(ctx, rev, []storage.NodeID{*rootNodeID})
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
	if !nodes[0].NodeID.Equivalent(*rootNodeID) {
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
	ctx, spanEnd := spanFor(ctx, "InclusionProof")
	defer spanEnd()

	proofs, err := s.BatchInclusionProof(ctx, rev, [][]byte{index})
	if err != nil {
		return nil, err
	}
	return proofs[string(index)], nil
}

// BatchInclusionProof returns an inclusion (or non-inclusion) proof for each of the specified keys
// at the specified revision. The return value is a map of the string form of the key to the
// inclusion proof for that key.
func (s SparseMerkleTreeReader) BatchInclusionProof(ctx context.Context, rev int64, indices [][]byte) (map[string][][]byte, error) {
	ctx, spanEnd := spanFor(ctx, "BatchInclusionProof")
	defer spanEnd()

	_, calculateSpanEnd := spanFor(ctx, "binc.calculateNodes")
	indexToSibs := make(map[string][]storage.NodeID)
	allSibs := make([]storage.NodeID, 0, len(indices)*s.hasher.BitLen())
	includedNodes := map[string]bool{}
	for _, index := range indices {
		nid := storage.NewNodeIDFromHash(index)
		sibs := nid.Siblings()
		indexToSibs[string(index)] = sibs
		for _, sib := range sibs {
			if sibID := sib.AsKey(); !includedNodes[sibID] {
				includedNodes[sibID] = true
				allSibs = append(allSibs, sib)
			}
		}
	}
	calculateSpanEnd()

	gnCtx, getNodesSpanEnd := spanFor(ctx, "binc.getMerkleNodes")
	nodes, err := s.tx.GetMerkleNodes(gnCtx, rev, allSibs)
	if err != nil {
		return nil, err
	}
	getNodesSpanEnd()

	_, postprocessSpanEnd := spanFor(ctx, "binc.postprocess")
	nodeMap := make(map[string]*storage.Node)
	for i, n := range nodes {
		if glog.V(2) {
			glog.Infof("   %x, %d: %x", n.NodeID.Path, len(n.NodeID.AsKey()), n.Hash)
		}
		nodeMap[n.NodeID.AsKey()] = &nodes[i]
	}

	r := map[string][][]byte{}
	for _, index := range indices {
		// We're building a full proof from a combination of whichever nodes we got
		// back from the storage layer, and the set of "null" hashes.
		sibs := indexToSibs[string(index)]
		ri := make([][]byte, len(sibs))
		// For each proof element:
		for i := range ri {
			proofID := sibs[i]
			pNode := nodeMap[proofID.AsKey()]
			if pNode == nil {
				// No node for this level from storage, so use the nil hash.
				continue
			}
			ri[i] = pNode.Hash
		}
		r[string(index)] = ri
	}
	postprocessSpanEnd()

	return r, nil
}

// SetLeaves adds a batch of leaves to the in-flight tree update.
func (s *SparseMerkleTreeWriter) SetLeaves(ctx context.Context, leaves []HashKeyValue) error {
	ctx, spanEnd := spanFor(ctx, "SetLeaves")
	defer spanEnd()

	for _, l := range leaves {
		if err := s.tree.SetLeaf(ctx, l.HashedKey, l.HashedValue); err != nil {
			return err
		}
	}
	return nil
}

// CalculateRoot calculates the new root hash including the newly added leaves.
func (s *SparseMerkleTreeWriter) CalculateRoot(ctx context.Context) ([]byte, error) {
	ctx, spanEnd := spanFor(ctx, "writer.CalculateRoot")
	defer spanEnd()

	s.tree.CalculateRoot(ctx)
	return s.tree.RootHash()
}

// HashKeyValue represents a Hash(key)-Hash(value) pair.
type HashKeyValue struct {
	// HashedKey is the hash of the key data
	HashedKey []byte

	// HashedValue is the hash of the value data.
	HashedValue []byte
}

func spanFor(ctx context.Context, name string) (context.Context, func()) {
	return monitoring.StartSpan(ctx, fmt.Sprintf("/trillian/m_sparse.%s", name))
}
