# Map Hashers

This document describes the constraints on and requirements for hashing
strategies to be used with a Trillian Map.

## Background

A Trillian Map is a transparent key:value store based on an underlying sparse
[Merkle tree](https://en.wikipedia.org/wiki/Merkle_tree).

The leaves of the tree hold arbitrary values (`MapLeaf.LeafValue`), and the
location of each leaf is specified by its index, a fixed size bitstring
(`MapLeaf.Index`).

### Merkle Trees

A Merkle tree is then formed by arranging these leaves into a binary tree, and
assigning a cryptographic hash value to each point in the tree:
 - Leaves get a hash value that is derived from the value of the leaf (and other
   inputs).
 - Interior nodes get a hash value that is derived from hashes of the two
   children of the node (plus other inputs).

This structure means that the hash value associated with the root of the tree
cryptographically encompasses the whole tree:
 - If any leaf value changes, the root hash will also change.
 - An adversary can't 'fake up' a tree that reproduces a specific root hash
   (unless they are able to generate arbitrary hash collisions).

### Addressing

The bit pattern of the leaves index also gives us an addressing scheme for all
of the nodes in the tree:
 - The full index value for a leaf (i.e. level 0 in the tree) is its address.
 - The index value for an interior node is one bit shorter than its two
   children, truncated at the end.

For a toy example with index values that are 8 bits long:
 - Leaves (level 0) would have addresses `0b00000000`, `0b000000001`, &hellip;,
   `0b11111111` (256 of them).
 - Level 1 nodes would have addresses `0b0000000x`, `0b00000001x`, &hellip;,
   `0b1111111x` (128 of them), and (say) `0b1010110x` has children with
   addresses `0b10101100` (left) and `0b10101101` (right).
 - Level 2 nodes have addresses `0b000000xx`, &hellip;, `0x111111xx`.
 - So on&hellip;
 - Level 7 nodes have addresses `0b0xxxxxxx`, `0b1xxxxxxx` (2 of them).
 - The root node (level 8) has an address which is the empty bitstring.

### Sparseness

The example above used a toy 8-bit index, but a real example would use a much
longer hash such as SHA-256 to generate index values.  This means that the set
of all possible leaves is huge (2<sup>256</sup> for SHA-256), and the set of
leaves which actually have values is much smaller &ndash; the Merkle tree is
**sparse**.

This is essential, because calculating the hash values for every node in a
256-depth Merkle tree is quite time-consuming:
 - 2<sup>256</sup> hashes for level 0
 - 2<sup>255</sup> hashes for level 1
 - &hellip;
 - 1 hash for level 256

for a total of 2<sup>257</sup> hash calculations (less one).

The sparseness property of the tree allows us to skip most of these
calculations: if we know (in advance) what the hash value for a particular **empty
subtree** is, we can use that hash value and skip the calculation of anything
further down the tree.

As long as we have this property, then a tree that has **`N`** leaves with values
can be hashed much more quickly:
 - `N` hashes for level 0
 - At most `N/2` hashes for level 1.
 - At most `N/4` hashes for level 2.
 - &hellip;
 - 1 hash for level 256

for a total of (at most) `2 N` hashes for the whole tree.

Also, if the tree is incrementally updated with `M` new leaf values, a similar
calculation indicates that at most `2 M` hashes need to be recalculated up the
tree.


## MapHasher Interface

In the Trillian codebase, the `Tree.HashStrategy` indicates the hashing strategy
in use for a particular tree, and the implementation of the hashing strategy needs
to satisfy the [`MapHasher` interface](../merkle/hashers/tree_hasher.go).

The following sections describe the current implementations of this `interface`.

### Test Map Hasher Scheme

Trillian includes a [simple hashing strategy](../merkle/maphasher/maphasher.go)
for test trees, where:
 - The hash of a leaf is `HashLeaf(leaf) := HASH(0x00 || leaf.LeafValue)` for an arbitrary hash
   function (SHA-256 by default).
 - The hash of a non-empty interior node is `HashChildren(l, r) := HASH(0x01 || l || r)`.
 - The hash of an empty subtree at level `n` is recursively build from the leaf
   hashes of an empty leaf:
     - `n = 0`: `E0 := HashLeaf(nil)`
     - `n = 1`: `E1 := HashChildren(E0, E0)`
     - `n = 2`: `E2 := HashChildren(E1, E1)`
     - &hellip;

This hash strategy has some simple properties:
 - The hash of any node is independent of its address/location in the tree
 - The hash of a zero-length leaf value is the same as the hash of an
   empty (never set) leaf (i.e. `HashLeaf(nil) == HashEmpty(_, 0)`).
 - More generally, the hash of the top of an empty subtree (whose leaves are all
   empty/never-set) is the same as the hash of the same size subtree with leaves
   with zero-length values.

However, this means that this hash strategy is more vulnerable to attacks where
subtrees are transplanted between different locations in the tree.

### CONIKS Hasher Strategy

The CONIKS hashing strategy
([original paper](https://www.usenix.org/system/files/conference/usenixsecurity15/sec15-paper-melara.pdf))
is more secure, but is more complicated because:
 - Hash values always depend on the location / address of the node in the tree
 - Empty subtrees have a different hash value than the equivalent subtree with
   leaves that have zero-length values.  This has to be the case to preserve the
   efficiency of the sparse Merkle tree: given that all hashes are location
   dependent, if the hash of an empty subtree were the same as that of a
   zero-length-leaves subtree, the hasher would need to calculate all
   2<sup>257</sup> hashes for the whole Merkle tree.

The details of the hash strategy are that:
 - The hash of a leaf that exists is `HASH('L' || treeID || index || depth ||
   leaf.LeafValue)`.
 - The hash of a non-empty interior node is `HASH(l || r)`.
 - The hash of an empty node is `HASH('E' || treeID || index || depth)`

## Trillian Map API

As described above, the map hashing strategy includes a specific hash calculation
that applies to subtrees where all of the enclosed leaves are empty (have never
been set).  This allows hash calculations to be more efficient for a sparse
tree, but also allows for a more efficient Map API.

In particular, any values in a leaf inclusion proof that match the empty hash
value for that node need not be sent on the wire.  Instead, a `nil` value is used
to indicate that this node in the proof is the top of an empty subtree (a
subtree all of whose leaves are empty / never-been-set).

Also, the distinction between empty / never-set leaves, and leaves whose values
have been explicitly set to a zero-length value, means that two possible hash
values (`leaf.LeafHash`) apply for leaves with `len(leaf.LeafValue)==0`.

So:
 - A `trillian.MapLeaf` with `len(leaf.LeafValue) == 0` may have two potential
   `leaf.LeafHash` values:
     - `len(leaf.LeafHash) == 0` if it has never been set (a shortcut equivalent to
       `hasher.HashEmpty(leaf.Index, 0)`)
     - `leaf.LeafHash == hasher.HashLeaf(leaf.Index, nil)` otherwise.
 - The inclusion proof in a `trillian.MapLeafInclusion` may include `nil` values
   for `inc.Inclusion[level]` values, which indicates that a value of
   `hasher.Empty(leaf.Index, level)` applies.
