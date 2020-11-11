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

The demo intends only to show the usage of the API and provide a simple way to test locally running the pipeline.
It is not intended to demonstrate where data would be sourced from, or how the output Tiles should be used.
See the comments in the demo script for more details.

* TODO(mhutchinson): Upgrade demo to store tiles and provide code to generate and confirm inclusion proofs.