# Storage layer

The interface, various concrete implementations, and any associated components live here.
Currently, there is only one storage implementation:
   * MySQL/MariaDB, which lives in [mysql/](mysql).


The design is such that both `LogStorage` and `MapStorage` models reuse a
shared `TreeStorage` model which can store arbitrary nodes in a tree.

Anyone poking around in here should be aware that there are some subtle
wrinkles introduced by the fact that Log trees grow upwards (i.e. the Log
considers nodes at level 0 to be the leaves), and in contrast the Map considers
the leaves to be at level 255 (and the root at 0), this is based on the [HStar2
algorithm](https://www.links.org/files/RevocationTransparency.pdf).

## TreeStorage

### Nodes

Nodes within the tree are each given a unique `NodeID`
([see storage/types.go](storage/types.go)), this ID can be thought of as the
binary path (0 for left, 1 for right) from the root of the tree down to the
node in question (or, equivalently, as the binary representation of the node's
horizonal index into the tree layer at the depth of the node.)

*TODO(al): pictures!*


### Subtrees

The `TreeStorage` model does not, in fact, store all the internal nodes of the
tree; it divides the tree into subtrees of depth 8 and stores the data for each
subtree as a single unit.  Within these subtrees, only the (subtree-relative)
"leaf" nodes are actually written to disk, the internal structure of the
subtrees is re-calculated when the subtree is read from disk.

Doing this compaction saves a considerable amout of on-disk space, and at least
for the MySQL storage implementation, results in a ~20% speed increase.

### History

Updates to the tree storage are performed in a batched fashion (i.e. some unit
of update which provides a self-consistent view of the tree - e.g.:
  * *n* `append leaf` operations along with internal node updates for the
    LogStorage, and tagged with their sequence number.
  * *n* `set value` operations along with internal node updates for the
    MapStorage.

These batched updates are termed `treeRevision`s, and nodes updated within each
revision are tagged with a monotonically incrementing sequence number.

In this fashion, the storage model records all historical revisions of the tree.

To perform lookups at a particular treeRevision, the `TreeStorage` simply
requests nodes from disk which are associated with the given `NodeID` and whose
`treeRevsion`s are `<=` the desired revision.

Currently there's no mechanism to safely garbage collect obsolete nodes so
storage grows without bound. This will be addressed at some point in the
future.

### Updates to the tree

The *current* treeRevision is defined to be the one referenced by the latest
Signed [Map|Log] Head (if there is no SignedHead, then the current treeRevision
is -1.)

Updates to the tree are performed *in the future* (i.e. at
`currentTreeRevision + 1`), and, to allow for reasonable performance, are not
required to be atomic (i.e. a tree update may partially fail), **however** for
this to work, higher layers using the storage layer must guarantee that failed
tree updates are later re-tried with either precisely the same set of node
chages, or a superset there-of, in order to ensure integrity of the tree.

We intend to enforce this contract within the `treeStorage` layer at some point
in the future.


## LogStorage

*TODO(al): flesh this out*

`LogStorage` builds upon `TreeStorage` and additionally provides a means of
storing log leaves, and `SignedTreeHead`s, and an API for sequencing new
leaves into the tree.

## MapStorage

*TODO(al): flesh this out*

`MapStorage` builds upon `TreeStorage` and additionally provides a means of
storing map values, and `SignedMapHead`s.


