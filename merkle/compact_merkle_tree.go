package merkle

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"

	log "github.com/golang/glog"
	"github.com/google/trillian"
)

// CompactMerkleTree is a compact merkle tree representation.
// Uses log(n) nodes to represent the current on-disk tree.
type CompactMerkleTree struct {
	hasher TreeHasher
	root   trillian.Hash
	// the list of "dangling" left-hand nodes, NOTE: index 0 is the leaf, not the root.
	nodes []trillian.Hash
	size  int64
}

func isPerfectTree(x int64) bool {
	return x != 0 && (x&(x-1) == 0)
}

func bitLen(x int64) int {
	r := 0
	for x > 0 {
		r++
		x >>= 1
	}
	return r
}

// GetNodeFunc is a function prototype which can look up particular nodes within a non-compact Merkle Tree.
// Used by the CompactMerkleTree to populate itself with correct state when starting up with a non-empty tree.
type GetNodeFunc func(depth int, index int64) (trillian.Hash, error)

// NewCompactMerkleTreeWithState creates a new CompactMerkleTree for the passed in |size|.
// |f| will be called a number of times with the co-ordinates of internal MerkleTree nodes whose hash values are
// required to initialise the internal state of the CompactMerkleTree.  |expectedRoot| is the known-good tree root
// of the tree at |size|, and is used to verify the correct initial state of the CompactMerkleTree after initialisation.
func NewCompactMerkleTreeWithState(hasher trillian.Hasher, size int64, f GetNodeFunc, expectedRoot trillian.Hash) *CompactMerkleTree {

	r := CompactMerkleTree{
		hasher: NewTreeHasher(hasher),
		nodes:  make([]trillian.Hash, bitLen(size)),
		size:   size,
	}

	if isPerfectTree(size) {
		log.V(1).Info("Is perfect tree.")
		// just have to trust it - the compact tree is empty for a perfect tree.
		copy(r.root[:], expectedRoot[:])
	} else {
		// Pull in the nodes we need to repopulate our compact tree and verify the root
		numBits := bitLen(size)
		for depth := 0; depth < numBits; depth++ {
			if size&1 == 1 {
				index := size - 1
				log.V(1).Infof("fetching d: %d i: %d, leaving size %d", depth, index, size)
				h, err := f(depth, index)
				if err != nil {
					// TODO(al): return an error, don't Fatal
					log.Fatalf("Failed to fetch node depth %d index %d: %s", depth, index, err)
				}
				r.nodes[depth] = h
			}
			size >>= 1
		}
		r.recalculateRoot(func(depth int, index int64, hash trillian.Hash) {})
	}
	if !bytes.Equal(r.root, expectedRoot) {
		// TODO(al): return an error, don't Fatal
		log.Fatalf("Corrupt state, expected root %s, got %s", hex.EncodeToString(expectedRoot[:]), hex.EncodeToString(r.root[:]))
	}
	log.V(1).Infof("Resuming at size %d, with root: %s", r.size, base64.StdEncoding.EncodeToString(r.root[:]))
	return &r
}

// NewCompactMerkleTree creates a new CompactMerkleTree with size zero.
func NewCompactMerkleTree(hasher trillian.Hasher) *CompactMerkleTree {
	emptyHash := hasher.Digest([]byte{})
	r := CompactMerkleTree{
		hasher: NewTreeHasher(hasher),
		root:   trillian.Hash(emptyHash[:]),
		nodes:  make([]trillian.Hash, 0),
		size:   0,
	}
	return &r
}

// CurrentRoot returns the current root hash.
func (c CompactMerkleTree) CurrentRoot() trillian.Hash {
	return c.root
}

// DumpNodes logs the internal state of the CompactMerkleTree, and is used for debugging.
func (c CompactMerkleTree) DumpNodes() {
	log.Infof("Tree Nodes @ %s", c.size)
	mask := int64(1)
	numBits := bitLen(c.size)
	for bit := 0; bit < numBits; bit++ {
		if c.size&mask != 0 {
			log.Infof("  %s", base64.StdEncoding.EncodeToString(c.nodes[bit][:]))
		} else {
			log.Infof("  -")
		}
		mask <<= 1
	}
}

type setNodeFunc func(depth int, index int64, hash trillian.Hash)

func (c *CompactMerkleTree) recalculateRoot(f setNodeFunc) {
	index := c.size

	var newRoot trillian.Hash
	first := true
	mask := int64(1)
	numBits := bitLen(c.size)
	for bit := 0; bit < numBits; bit++ {
		index >>= 1
		if c.size&mask != 0 {
			if first {
				newRoot = c.nodes[bit]
				first = false
			} else {
				newRoot = c.hasher.HashChildren(c.nodes[bit], newRoot)
				f(bit+1, index, newRoot)
			}
		}
		mask <<= 1
	}
	c.root = newRoot
}

// AddLeaf calculates the leafhash of |data| and appends it to the tree.
// |f| is a callback which will be called multiple times with the full MerkleTree coordinates of nodes whose hash should be updated.
func (c *CompactMerkleTree) AddLeaf(data []byte, f setNodeFunc) (int64, trillian.Hash) {
	h := c.hasher.HashLeaf(data)
	return c.AddLeafHash(h, f), h
}

// AddLeafHash adds the specified |leafHash| to the tree.
// |f| is a callback which will be called multiple times with the full MerkleTree coordinates of nodes whose hash should be updated.
func (c *CompactMerkleTree) AddLeafHash(leafHash trillian.Hash, f setNodeFunc) (assignedSeq int64) {
	defer func() {
		c.size++
		// TODO(al): do this lazily
		c.recalculateRoot(f)
	}()

	assignedSeq = c.size
	index := assignedSeq

	f(0, index, leafHash)

	if c.size == 0 {
		// new tree
		c.nodes = append(c.nodes, leafHash)
		return
	}

	// Initialize our running hash value to the leaf hash
	hash := leafHash
	bit := 0
	// Iterate over the bits in our tree size
	for t := c.size; t > 0; t >>= 1 {
		if t&1 == 0 {
			// Just store the running hash here; we're done.
			c.nodes[bit] = hash
			// Don't re-write the leaf hash node (we've done it above already)
			if bit > 0 {
				// Store the leaf hash node
				f(bit, index, hash)
			}
			return
		}
		// The bit is set so we have a node at that position in the nodes list so hash it with our running hash:
		hash = c.hasher.HashChildren(c.nodes[bit], hash)
		// Store the resulting parent hash.
		f(bit+1, index>>1, hash)
		// Figure out if we're done:
		if bit+1 >= len(c.nodes) {
			// If we're extending the node list then add a new entry with our
			// running hash, and we're done.
			c.nodes = append(c.nodes, hash)
			return
		} else if t&0x02 == 0 {
			// If the node above us is unused at this tree size, then store our
			// running hash there, and we're done.
			c.nodes[bit+1] = hash
			return
		}
		// Otherwise, go around again.
		bit++
	}
	// We should never get here, because that'd mean we had a running hash which
	// we've not stored somewhere.
	log.Fatal("AddLeaf failed.")
	return
}

// Size returns the current size of the tree, that is, the number of leaves ever added to the tree.
func (c CompactMerkleTree) Size() int64 {
	return c.size
}
