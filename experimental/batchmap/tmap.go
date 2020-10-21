// Package batchmap is a library to be used within Beam pipelines to construct
// verifiable data structures.
package batchmap

//go:generate go install github.com/apache/beam/sdks/go/cmd/starcgen
//go:generate starcgen --package=batchmap --identifiers=entryToNodeHashFn,partitionByPrefixLenFn,tileHashFn,leafShardFn,tileToNodeHashFn,tileUpdateFn

import (
	"context"
	"crypto"
	"fmt"

	"github.com/apache/beam/sdks/go/pkg/beam"

	"github.com/google/trillian/experimental/batchmap/tilepb"

	"github.com/google/trillian/storage/tree"

	"github.com/google/trillian/merkle/coniks"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/merkle/smt"
)

var (
	cntTilesHashed  = beam.NewCounter("batchmap", "tiles-hashed")
	cntTilesCopied  = beam.NewCounter("batchmap", "tiles-copied")
	cntTilesCreated = beam.NewCounter("batchmap", "tiles-created")
	cntTilesUpdated = beam.NewCounter("batchmap", "tiles-updated")
)

// Create builds a new map from the given PCollection of *tilepb.Entry. Outputs
// the resulting Merkle tree tiles as a PCollection of *tilepb.Tile.
//
// The keys in the input PCollection must be 256-bit, uniformly distributed,
// and unique within the input.
// The values in the input PCollection must be 256-bit.
// treeID should be a unique ID for the lifetime of this map. This is used as
// part of the hashing algorithm to provide preimage resistance. If the tiles
// are to be imported into Trillian for serving, this must match the tree ID
// within Trillian.
// The internal hash algorithm can be picked between SHA256 and SHA512_256.
// The internal nodes will use this algorithm via the CONIKS strategy.
// prefixStrata is the number of 8-bit prefix strata. Any path from root to leaf
// will have prefixStrata+1 tiles.
func Create(s beam.Scope, entries beam.PCollection, treeID int64, hash crypto.Hash, prefixStrata int) (beam.PCollection, error) {
	s = s.Scope("batchmap.Create")
	if prefixStrata < 0 || prefixStrata >= 32 {
		return beam.PCollection{}, fmt.Errorf("prefixStrata must be in [0, 32), got %d", prefixStrata)
	}

	// Construct the map pipeline starting with the leaf tiles.
	nodeHashes := beam.ParDo(s, entryToNodeHashFn, entries)
	lastStratum := createStratum(s, nodeHashes, treeID, hash, prefixStrata)
	allTiles := make([]beam.PCollection, 0, prefixStrata+1)
	allTiles = append(allTiles, lastStratum)
	for d := prefixStrata - 1; d >= 0; d-- {
		nodeHashes = beam.ParDo(s, tileToNodeHashFn, lastStratum)
		lastStratum = createStratum(s, nodeHashes, treeID, hash, d)
		allTiles = append(allTiles, lastStratum)
	}

	// Collate all of the strata together and return them.
	return beam.Flatten(s, allTiles...), nil
}

// Update takes an existing base map (PCollection of *tilepb.Tile), applies the
// delta (PCollection of *tilepb.Entry) and returns the resulting map as a
// PCollection of *tilepb.Tile.
// The deltas can add new keys to the map or overwrite existing keys. Keys
// cannot be deleted (though their value can be set to a sentinel value).
//
// treeID, hash, and prefixStrata must match the values passed into the
// original call to Create that started the base map.
func Update(s beam.Scope, base, delta beam.PCollection, treeID int64, hash crypto.Hash, prefixStrata int) (beam.PCollection, error) {
	s = s.Scope("batchmap.Update")
	if prefixStrata < 0 || prefixStrata >= 32 {
		return beam.PCollection{}, fmt.Errorf("prefixStrata must be in [0, 32), got %d", prefixStrata)
	}

	// Tile sets returned from this library have tiles present at all byte
	// lengths from [0..prefixStrata]. This makes this a perfect partition fn.
	baseStrata := beam.Partition(s, prefixStrata+1, partitionByPrefixLenFn, base)
	// Construct the map pipeline starting with the leaf tiles.
	nodeHashes := beam.ParDo(s, entryToNodeHashFn, delta)
	lastStratum := updateStratum(s, baseStrata[prefixStrata], nodeHashes, treeID, hash, prefixStrata)

	allTiles := make([]beam.PCollection, 0, prefixStrata+1)
	allTiles = append(allTiles, lastStratum)
	for d := prefixStrata - 1; d >= 0; d-- {
		nodeHashes = beam.ParDo(s, tileToNodeHashFn, lastStratum)
		lastStratum = updateStratum(s, baseStrata[d], nodeHashes, treeID, hash, d)
		allTiles = append(allTiles, lastStratum)
	}

	// Collate all of the strata together and return them.
	return beam.Flatten(s, allTiles...), nil
}

// createStratum creates the tiles for the stratum at the given rootDepth bytes.
// leaves is a PCollection of nodeHash that are the leaves of this layer.
// output is a PCollection of *tilepb.Tile.
func createStratum(s beam.Scope, leaves beam.PCollection, treeID int64, hash crypto.Hash, rootDepth int) beam.PCollection {
	s = s.Scope(fmt.Sprintf("createStratum-%d", rootDepth))
	shardedLeaves := beam.ParDo(s, &leafShardFn{RootDepthBytes: rootDepth}, leaves)
	return beam.ParDo(s, &tileHashFn{TreeID: treeID, Hash: hash}, beam.GroupByKey(s, shardedLeaves))
}

// updateStratum updates the tiles for the stratum at the given bytes depth.
// base is a PCollection of *tilepb.Tile which is the tiles in the stratum
// to be updated.
// deltas is a PCollection of nodeHash that are the updated leaves of this layer.
// output is a PCollection of *tilepb.Tile.
func updateStratum(s beam.Scope, base, deltas beam.PCollection, treeID int64, hash crypto.Hash, rootDepth int) beam.PCollection {
	s = s.Scope(fmt.Sprintf("updateStratum-%d", rootDepth))
	shardedBase := beam.ParDo(s, func(t *tilepb.Tile) ([]byte, *tilepb.Tile) { return t.GetPath(), t }, base)
	shardedDelta := beam.ParDo(s, &leafShardFn{RootDepthBytes: rootDepth}, deltas)
	return beam.ParDo(s, &tileUpdateFn{TreeID: treeID, Hash: hash}, beam.CoGroupByKey(s, shardedBase, shardedDelta))
}

// nodeHash describes a leaf to be included in a tile.
// This is logically the same as smt.Node however it has public fields so is
// serializable by the default Beam coder. Also, it allows changes to be made
// to smt.Node without affecting this, which improves decoupling.
type nodeHash struct {
	// Path from root of the map to this node. Equivalant to NodeID2, but with the
	// significant benefit that it will be serialized properly without writing a
	// custom coder for nodeHash.
	Path []byte
	Hash []byte
}

func partitionByPrefixLenFn(t *tilepb.Tile) int {
	return len(t.GetPath())
}

func tileToNodeHashFn(t *tilepb.Tile) nodeHash {
	return nodeHash{Path: t.GetPath(), Hash: t.GetRootHash()}
}

func entryToNodeHashFn(e *tilepb.Entry) nodeHash {
	return nodeHash{Path: e.GetHashKey(), Hash: e.GetHashValue()}
}

// leafShardFn groups nodeHashs together based on the first RootDepthBytes
// bytes of their path. This groups all leaves from the same tile together.
type leafShardFn struct {
	RootDepthBytes int
}

func (fn *leafShardFn) ProcessElement(leaf nodeHash) ([]byte, nodeHash) {
	return leaf.Path[:fn.RootDepthBytes], leaf
}

type tileHashFn struct {
	TreeID int64
	Hash   crypto.Hash
	th     *tileHasher
}

func (fn *tileHashFn) Setup() {
	fn.th = &tileHasher{fn.TreeID, coniks.New(fn.Hash)}
}

func (fn *tileHashFn) ProcessElement(ctx context.Context, rootPath []byte, leaves func(*nodeHash) bool) (*tilepb.Tile, error) {
	nodes, err := convertNodes(leaves)
	if err != nil {
		return nil, err
	}
	cntTilesHashed.Inc(ctx, 1)
	return fn.th.construct(rootPath, nodes)
}

// convertNodes consumes the Beam-style iterator of nodeHash and returns the
// corresponding slice of smt.Node. Nothing clever is attempted to ensure that
// the data structure will fit in memory. If the iterator has too many elements
// then this will cause an out of memory panic. It is up to the library client
// to configure the map with an appropriate number of prefix strata such that
// this does not occur.
func convertNodes(leaves func(*nodeHash) bool) ([]smt.Node, error) {
	nodes := []smt.Node{}
	var leaf nodeHash
	for leaves(&leaf) {
		lid, err := nodeID2Decode(leaf.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to decode leaf ID: %v", err)
		}
		nodes = append(nodes, smt.Node{ID: lid, Hash: leaf.Hash})
	}
	return nodes, nil
}

// tileUpdateFn merges the base tile from the original map with the deltas that
// represent the changes to the map. Note this only support additions or
// overwrites. There is no ability to delete a leaf.
type tileUpdateFn struct {
	TreeID int64
	Hash   crypto.Hash
	th     *tileHasher
}

func (fn *tileUpdateFn) Setup() {
	fn.th = &tileHasher{fn.TreeID, coniks.New(fn.Hash)}
}

func (fn *tileUpdateFn) ProcessElement(ctx context.Context, rootPath []byte, bases func(**tilepb.Tile) bool, deltas func(*nodeHash) bool) (*tilepb.Tile, error) {
	base, err := getOptionalTile(bases)
	if err != nil {
		return nil, fmt.Errorf("failed precondition getOptionalTile at %x: %v", rootPath, err)
	}

	nodes, err := convertNodes(deltas)
	if err != nil {
		return nil, err
	}

	if len(nodes) == 0 {
		// If there are no deltas, then the base tile is unchanged.
		cntTilesCopied.Inc(ctx, 1)
		return base, nil
	}
	if base == nil {
		cntTilesCreated.Inc(ctx, 1)
		return fn.th.construct(rootPath, nodes)
	}

	cntTilesUpdated.Inc(ctx, 1)
	return fn.updateTile(rootPath, base, nodes)
}

func (fn *tileUpdateFn) updateTile(rootPath []byte, base *tilepb.Tile, deltas []smt.Node) (*tilepb.Tile, error) {
	baseNodes := make([]smt.Node, 0, len(base.Leaves))
	for _, l := range base.Leaves {
		leafPath := append(rootPath, l.Path...)
		lidx, err := nodeID2Decode(leafPath)
		if err != nil {
			return nil, fmt.Errorf("failed to decode leaf ID: %v", err)
		}
		baseNodes = append(baseNodes, smt.Node{ID: lidx, Hash: l.GetHash()})
	}

	return fn.th.update(rootPath, baseNodes, deltas)
}

// tileHasher is an smt.NodeAccessor used for computing node hashes of a tile.
// This is not serializable and must be constructed within each worker stage.
type tileHasher struct {
	treeID int64
	h      hashers.MapHasher
}

func (th *tileHasher) construct(rootPath []byte, nodes []smt.Node) (*tilepb.Tile, error) {
	rootDepthBytes := len(rootPath)
	if err := smt.Prepare(nodes, nodes[0].ID.BitLen()); err != nil {
		return nil, fmt.Errorf("smt.Prepare: %v", err)
	}

	// N.B. This needs to be done after Prepare but BEFORE HStar3 because it
	// fiddles around with the nodes and makes their IDs invalid afterwards.
	tls := make([]*tilepb.TileLeaf, len(nodes))
	for i, n := range nodes {
		nPath, err := nodeID2Encode(n.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to encode leaf ID: %v", err)
		}
		tls[i] = &tilepb.TileLeaf{
			Path: nPath[rootDepthBytes:],
			Hash: n.Hash,
		}
	}

	rootHash, err := th.hashTile(uint(8*rootDepthBytes), nodes)
	if err != nil {
		return nil, fmt.Errorf("failed to hash tile: %v", err)
	}

	return &tilepb.Tile{
		Path:     rootPath,
		Leaves:   tls,
		RootHash: rootHash,
	}, nil
}

func (th *tileHasher) update(rootPath []byte, baseNodes, deltaNodes []smt.Node) (*tilepb.Tile, error) {
	// We add new values first and then update with base to easily check for duplicates in deltas.
	m := make(map[tree.NodeID2]smt.Node)
	for _, leaf := range deltaNodes {
		if v, found := m[leaf.ID]; found {
			return nil, fmt.Errorf("found duplicate values at leaf tile position %s: {%x, %x}", leaf.ID, v.Hash, leaf.Hash)
		}
		m[leaf.ID] = leaf
	}

	for _, leaf := range baseNodes {
		if _, found := m[leaf.ID]; !found {
			// Only add base values if they haven't been updated.
			m[leaf.ID] = leaf
		}
	}

	nodes := make([]smt.Node, 0, len(m))
	for _, v := range m {
		nodes = append(nodes, v)
	}
	return th.construct(rootPath, nodes)
}

// hashTile computes the root hash of the root given the prepared leaves.
// The leaves slice MUST NOT be used after calling this method.
func (th *tileHasher) hashTile(depthBits uint, leaves []smt.Node) ([]byte, error) {
	h, err := smt.NewHStar3(leaves, th.h.HashChildren, uint(leaves[0].ID.BitLen()), depthBits)
	if err != nil {
		return nil, err
	}
	r, err := h.Update(th)
	if err != nil {
		return nil, err
	}
	if len(r) != 1 {
		return nil, fmt.Errorf("expected single root but got %d", len(r))
	}
	return r[0].Hash, nil
}

// Get returns hash of an empty subtree for the given root node ID.
func (th tileHasher) Get(id tree.NodeID2) ([]byte, error) {
	oldID := tree.NewNodeIDFromID2(id)
	height := th.h.BitLen() - oldID.PrefixLenBits
	return th.h.HashEmpty(th.treeID, oldID.Path, height), nil
}

func (th tileHasher) Set(id tree.NodeID2, hash []byte) {}

func nodeID2Encode(n tree.NodeID2) ([]byte, error) {
	b, c := n.LastByte()
	if c == 0 {
		return []byte{}, nil
	}
	if c == 8 {
		return append([]byte(n.FullBytes()), b), nil
	}
	return nil, fmt.Errorf("node ID bit length is not aligned to bytes: %d", n.BitLen())
}

func nodeID2Decode(bs []byte) (tree.NodeID2, error) {
	return tree.NewNodeID2(string(bs), 8*uint(len(bs))), nil
}

// getOptionalTile consumes the Beam-style iterator and returns:
// - nil if there were no entries
// - the single tile if there was only one entry
// - an error if there were multiple entries
func getOptionalTile(iter func(**tilepb.Tile) bool) (*tilepb.Tile, error) {
	var t1, t2 *tilepb.Tile
	if !iter(&t1) || !iter(&t2) { // Only at most one entry is found.
		return t1, nil // Note: Returns nil if found nothing.
	}
	return nil, fmt.Errorf("unexpectedly found multiple tiles at %x", t1.GetPath())
}
