# Experimental Beam Map Generation

Generates [Verifiable Maps](../../docs/papers/VerifiableDataStructures.pdf)
using [Beam Go](https://beam.apache.org/get-started/quickstart-go/).
Generating a map in batch scales better than incremental for large numbers of
key/values.

> :warning: **This code is experimental!** This code is free to change outside
> of semantic versioning in the trillian repository.

The resulting map is output as tiles, in which the tree is divided from the
root in a configurable number of prefix strata.
Each strata contains a single byte of the 256-bit path.
Tiles are only output if they are non-empty.

* TODO(mhutchinson): Include demo for generating map and performing
  inclusion proofs.