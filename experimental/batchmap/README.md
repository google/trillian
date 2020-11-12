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

## Running the demo

### Building the map

The following instructions show how to run this code locally using the Python
portable runner. Setting up a Python environment is beyond the scope of this
demo, but instructions can be found at [Beam Python Tips](https://cwiki.apache.org/confluence/display/BEAM/Python+Tips).
These instructions are for Linux/MacOS but can likely be adapted for Windows.

Ensure that the Python runner is running:
1. Check out `github.com/apache/beam` (tested at branch `release-2.24.0`)
2. `cd sdks/python` within that repository
3. `python -m apache_beam.runners.portability.local_job_service_main --port=8099`

In another terminal:
1. Check out this repository and `cd` to the folder containing this README
2. `go run ./cmd/build/mapdemo.go --output=/tmp/mapv1 --runner=universal --endpoint=localhost:8099 --environment_type=LOOPBACK`

The pipeline should run and generate files under the ouput directory, each of which contains a tile from the map.
Note that a new file will be constructed for each tile output, which can get very large
if the `key_count` or `prefix_strata` parameters are changed from their default values!
If these parameters are set too high, one could run out of inodes on your file system.
You have been warned.

The demo intends only to show the usage of the API and provide a simple way to test locally running the pipeline.
It is not intended to demonstrate where data would be sourced from, or how the output Tiles should be used.
See the comments in the demo script for more details.

### Verifying the map

This requires a map to have been constructed using the previous instructions.
Verifying that a particular key/value is set correctly within the tiles can be done with the command:
* `go run cmd/verify/verify.go --logtostderr --map_dir=/tmp/mapv1 --key=5`

The `map_dir` must match the directory provided as `output` in the previous stage.
The parameters for `value_salt` and `tree_id` must also match those used in the map
construction as they are used during the value construction/hashing.

If the expected value is committed to correctly by the tiles, then you will see an output line similar to:

```
key 5 found at path 11cd1b2203ad4a3a11ff479d1ee75a59c9f33a73c5f5cf45bda87b656237e9ed, with value '[v1]5' (1e27e661ca57f2231fb41b7ef861ab702ce7412921e4df9eb106db0d8b442227) committed to by map root 4365e3c65742fdfeb60079b677ccf4a264405c0d18fc7db1706690a1b06db73c
```

Setting the `key` parameter to a key outside the range generated in the map will show non-inclusion, as will
changing the `tree_id` or `value_salt` parameters.
