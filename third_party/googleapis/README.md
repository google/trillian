## Purpose

This directory is manually vendored in order to take the required proto definitions from googleapis.
The principle is that the dependency should be hermetic and lightweight.

Previously, the instructions were to manually clone the whole googleapis repository at head, into
a known location.

## Updating

The required files were determined by manual inspection of the codebase and closing the
transitive dependencies by checking the imports of the files.
Should the upstream repository be significantly refactored then it is possible that further files
may need to be imported if an update is performed.

The files here were cloned at @c81bb70.
The workflow is simple and documented here for future maintainers:
```sh
export GA_VERSION=c81bb701eb53991d6faf74b2656eaf539261a122
mkdir -p google/api
wget https://raw.githubusercontent.com/googleapis/googleapis/$GA_VERSION/google/api/annotations.proto -O google/api/annotations.proto
wget https://raw.githubusercontent.com/googleapis/googleapis/$GA_VERSION/google/api/http.proto -O google/api/http.proto
mkdir -p google/rpc
wget https://raw.githubusercontent.com/googleapis/googleapis/$GA_VERSION/google/rpc/code.proto -O google/rpc/code.proto
wget https://raw.githubusercontent.com/googleapis/googleapis/$GA_VERSION/google/rpc/status.proto -O google/rpc/status.proto
```
