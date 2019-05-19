# Trillian: General Transparency

[![Build Status](https://travis-ci.org/google/trillian.svg?branch=master)](https://travis-ci.org/google/trillian)
[![Go Report Card](https://goreportcard.com/badge/github.com/google/trillian)](https://goreportcard.com/report/github.com/google/trillian)
[![GoDoc](https://godoc.org/github.com/google/trillian?status.svg)](https://godoc.org/github.com/google/trillian)
[![Slack Status](https://img.shields.io/badge/Slack-Chat-blue.svg)](https://gtrillian.slack.com/)

 - [Overview](#overview)
 - [Support](#support)
 - [Using the Code](#using-the-code)
     - [MySQL Setup](#mysql-setup)
     - [Integration Tests](#integration-tests)
 - [Working on the Code](#working-on-the-code)
     - [Rebuilding Generated Code](#rebuilding-generated-code)
     - [Updating Vendor Code](#updating-vendor-code)
     - [Running Codebase Checks](#running-codebase-checks)
 - [Design](#design)
     - [Design Overview](#design-overview)
     - [Personalities](#personalities)
     - [Log Mode](#log-mode)
     - [Map Mode](#map-mode)
 - [Use Cases](#use-cases)
     - [Certificate Transparency Log](#certificate-transparency-log)
     - [Verifiable Log-Backed Map](#verifiable-log-backed-map)


## Overview

Trillian is an implementation of the concepts described in the
[Verifiable Data Structures](docs/papers/VerifiableDataStructures.pdf) white paper,
which in turn is an extension and generalisation of the ideas which underpin
[Certificate Transparency](https://certificate-transparency.org).

Trillian implements a [Merkle tree](https://en.wikipedia.org/wiki/Merkle_tree)
whose contents are served from a data storage layer, to allow scalability to
extremely large trees.  On top of this Merkle tree, Trillian provides two
modes:

 - An append-only **Log** mode, analogous to the original
   [Certificate Transparency](https://certificate-transparency.org) logs.  In
   this mode, the Merkle tree is effectively filled up from the left, giving a
   *dense* Merkle tree.
 - An experimental **Map** mode that allows transparent storage of arbitrary
   key:value pairs derived from the contents of a source Log; this is also known
   as a **log-backed map**.  In this mode, the key's hash is used to designate a
   particular leaf of a deep Merkle tree – sufficiently deep that filled
   leaves are vastly outnumbered by unfilled leaves, giving a *sparse* Merkle
   tree.  (A Trillian Map is an *unordered* map; it does not allow enumeration
   of the Map's keys.)

Note that Trillian requires particular applications to provide their own
[personalities](#personalities) on top of the core transparent data store
functionality.

[Certificate Transparency (CT)](https://tools.ietf.org/html/rfc6962)
is the most well-known and widely deployed transparency application, and a
implementation of CT as a Trillian personality is available in the
[certificate-transparency-go repo](https://github.com/google/certificate-transparency-go/blob/master/trillian).

Other examples of Trillian personalities are available in the
[trillian-examples](https://github.com/google/trillian-examples) repo.


## Support

- Mailing list: https://groups.google.com/forum/#!forum/trillian-transparency
- Slack: https://gtrillian.slack.com/ ([invitation](https://join.slack.com/t/gtrillian/shared_invite/enQtNDM3NTE3NjA4NDcwLWQ1ZjU3NzVkYmVlMDU0MDVhNWI4NDEwNzQ4MDE4NGFmMThmM2U5YTU2OWVjOGNmOWYxYTAzOWE4MDRhNzJkNDI))


## Using the Code

**WARNING**: The Trillian codebase is still under development, but the Log mode
is now being used in production by several organizations. We will try to avoid
any further incompatible code and schema changes but cannot guarantee that they
will never be necessary.

To build and test Trillian you need:

 - Go 1.9 or later.

To run many of the tests (and production deployment) you need:

 - [MySQL](https://www.mysql.com/) or [MariaDB](https://mariadb.org/) to provide
   the data storage layer; see the [MySQL Setup](#mysql-setup) section.

Use the standard Go tools to install other dependencies.

```bash
go get github.com/google/trillian
cd $GOPATH/src/github.com/google/trillian
go get -t -u -v ./...
```

To build and run tests, use:

```bash
go test ./...
```

Note that go seems to sometimes fail to fetch or update all dependencies (as of
v1.10.2), so you may need to manually fetch missing ones, or update all Go
source with:

```bash
go get -u -v all
```

The repository also includes multi-process integration tests, described in the
[Integration Tests](#integration-tests) section below.


### MySQL Setup

To run Trillian's integration tests you need to have an instance of MySQL
running and configured to:

 - listen on the standard MySQL port 3306 (so `mysql --host=127.0.0.1
   --port=3306` connects OK)
 - not require a password for the `root` user

You can then set up the [expected tables](storage/mysql/schema/storage.sql) in a
`test` database like so:

```bash
./scripts/resetdb.sh
Warning: about to destroy and reset database 'test'
Are you sure? y
> Resetting DB...
> Reset Complete
```

If you are working with the Trillian Map, you will probably need to increase
the
[MySQL maximum connection count](https://dev.mysql.com/doc/refman/5.5/en/server-system-variables.html#sysvar_max_connections):

```bash
% mysql -u root
MySQL> SET GLOBAL max_connections = 1000;
```

### Integration Tests

Trillian includes an integration test suite to confirm basic end-to-end
functionality, which can be run with:

```bash
./integration/integration_test.sh
```

This runs two multi-process tests:

 - A [test](integration/log_integration_test.go) that starts a Trillian server
   in Log mode, together with a signer, logs many leaves, and checks they are
   integrated correctly.
 - A [test](integration/map_integration_test.go) that starts a Trillian server
   in Map mode, sets various key:value pairs and checks they can be retrieved.


## Working on the Code

Developers who want to make changes to the Trillian codebase need some
additional dependencies and tools, described in the following sections.  The
[Travis configuration](.travis.yml) for the codebase is also useful reference
for the required tools and scripts, as it may be more up-to-date than this
document.

### Rebuilding Generated Code

Some of the Trillian Go code is autogenerated from other files:

 - [gRPC](http://www.grpc.io/) message structures are originally provided as
   [protocol buffer](https://developers.google.com/protocol-buffers/) message
   definitions.
 - Some unit tests use mock implementations of interfaces; these are created
   from the real implementations by [GoMock](https://github.com/golang/mock).
 - Some enums have string-conversion methods (satisfying the `fmt.Stringer`
   interface) created using the
   [stringer](https://godoc.org/golang.org/x/tools/cmd/stringer) tool (`go get
   golang.org/x/tools/cmd/stringer`).

Re-generating mock or protobuffer files is only needed if you're changing
the original files; if you do, you'll need to install the prerequisites:

  - `mockgen` tool from https://github.com/golang/mock
  - `stringer` tool from https://golang.org/x/tools/cmd/stringer
  - `protoc`, [Go support for protoc](https://github.com/golang/protobuf),
     [grpc-gateway](https://github.com/grpc-ecosystem/grpc-gateway) and
     [protoc-gen-doc](https://github.com/pseudomuto/protoc-gen-doc).
  - protocol buffer definitions for standard Google APIs:

    ```bash
    git clone https://github.com/googleapis/googleapis.git $GOPATH/src/github.com/googleapis/googleapis
    ```

and run the following:

```bash
go generate -x ./...  # hunts for //go:generate comments and runs them
```

### Updating Vendor Code

The Trillian codebase includes a couple of external projects under the `vendor/`
subdirectory, to ensure that builds use a fixed version (typically because the
upstream repository does not guarantee back-compatibility between the tip
`master` branch and the current stable release).  These external codebases are
included as Git
[subtrees](https://github.com/git/git/blob/master/contrib/subtree/git-subtree.txt).

To update the code in one of these subtrees, perform steps like:

```bash
# Add master repo for upstream code as a Git remote.
git remote add vendor-xyzzy https://github.com/orgname/xyzzy
# Pull the updated code for the desired version tag from the remote, dropping history.
# Trailing / in prefix is needed.
git subtree pull --squash --prefix=vendor/github.com/orgname/xyzzy/ vendor-xyzzy vX.Y.Z
```

If new `vendor/` subtree is required, perform steps similar to:

```bash
# Add master repo for upstream code as a Git remote.
git remote add vendor-xyzzy https://github.com/orgname/xyzzy
# Pull the desired version of the code in, dropping history.
# Trailing / in --prefix is needed.
git subtree add --squash --prefix=vendor/github.com/orgname/xyzzy/ vendor-xyzzy vX.Y.Z
```

### Running Codebase Checks

The [`scripts/presubmit.sh`](scripts/presubmit.sh) script runs various tools
and tests over the codebase.

#### Install [golangci-lint](https://github.com/golangci/golangci-lint#local-installation).
```bash
go get -u github.com/golangci/golangci-lint/cmd/golangci-lint
cd $GOPATH/src/github.com/golangci/golangci-lint/cmd/golangci-lint
go install -ldflags "-X 'main.version=$(git describe --tags)' -X 'main.commit=$(git rev-parse --short HEAD)' -X 'main.date=$(date)'"
cd -
```

#### Install [prototool](https://github.com/uber/prototool#installation)
```bash
go get -u github.com/uber/prototool/cmd/prototool
```

#### Run code generation, build, test and linters
```bash
./scripts/presubmit.sh
```

#### Or just run the linters alone
```bash
golangci-lint run
prototool lint
```


## Design

### Design Overview

Trillian is primarily implemented as a
[gRPC service](http://www.grpc.io/docs/guides/concepts.html#service-definition);
this service receives get/set requests over gRPC and retrieves the corresponding
Merkle tree data from a separate storage layer (currently using MySQL), ensuring
that the cryptographic properties of the tree are preserved along the way.

The Trillian service is multi-tenanted – a single Trillian installation can
support multiple Merkle trees in parallel, distinguished by their `TreeId` – and
each tree operates in one of two modes:

 - **Log** mode: an append-only collection of items; this has two sub-modes:
   - normal Log mode, where the Trillian service assigns sequence numbers to
     new tree entries as they arrive
   - 'preordered' Log mode, where the unique sequence number for entries in
     the Merkle tree is externally specified
 - **Map** mode: a collection of key:value pairs.

In either case, Trillian's key transparency property is that cryptographic
proofs of inclusion/consistency are available for data items added to the
service.


### Personalities

To build a complete transparent application, the Trillian core service needs
to be paired with additional code, known as a *personality*, that provides
functionality that is specific to the particular application.

In particular, the personality is responsible for:

 * **Admission Criteria** – ensuring that submissions comply with the
   overall purpose of the application.
 * **Canonicalization** – ensuring that equivalent versions of the same
   data get the same canonical identifier, so they can be de-duplicated by
   the Trillian core service.
 * **External Interface** – providing an API for external users,
   including any practical constraints (ACLs, load-balancing, DoS protection,
   etc.)

This is
[described in more detail in a separate document](docs/Personalities.md).
General
[design considerations for transparent Log applications](docs/TransparentLogging.md)
are also discussed seperately.

### Log Mode

When running in Log mode, Trillian provides a gRPC API whose operations are
similar to those available for Certificate Transparency logs
(cf. [RFC 6962](https://tools.ietf.org/html/6962)). These include:

 - `GetLatestSignedLogRoot` returns information about the current root of the
   Merkle tree for the log, including the tree size, hash value, timestamp and
   signature.
 - `GetLeavesByHash`, `GetLeavesByIndex` and `GetLeavesByRange` return leaf
   information for particular leaves, specified either by their hash value or
   index in the log.
 - `QueueLeaves` requests inclusion of specified items into the log.
     - For a pre-ordered log, `AddSequencedLeaves` requests the inclusion of
       specified items into the log at specified places in the tree.
 - `GetInclusionProof`, `GetInclusionProofByHash` and `GetConsistencyProof`
    return inclusion and consistency proof data.

In Log mode (whether normal or pre-ordered), Trillian includes an additional
Signer component; this component periodically processes pending items and
adds them to the Merkle tree, creating a new signed tree head as a result.

![Log components](docs/images/LogDesign.png)

(Note that each of the components in this diagram can be
[distributed](https://github.com/google/certificate-transparency-go/blob/master/trillian/docs/ManualDeployment.md#distribution),
for scalability and resilience.)


### Map Mode

**WARNING**: Trillian Map mode is experimental and under development; it should
not be relied on for a production service (yet).

Trillian in Map mode can be thought of as providing a key:value store for
values derived from a data source (normally a Trillian Log), together with
cryptographic transparency guarantees for that data.

When running in Map mode, Trillian provides a straightforward gRPC API with the
following available operations:

 - `SetLeaves` requests inclusion of specified key:value pairs into the Map;
   these will appear as the next **revision** of the Map, with a new tree head
   for that revision.
 - `GetSignedMapRoot` returns information about the current root of the Merkle
   tree representing the Map, including a revision , hash value, timestamp and
   signature.
     - A variant allows queries of the tree root at a specified historical
       revision.
 - `GetLeaves` returns leaf information for a specified set of key values,
   optionally as of a particular revision.  The returned leaf information also
   includes inclusion proof data.

![Map components](docs/images/MapDesign.png)


### Logged Map

As a stand-alone component, it is not possible to reliably monitor or audit a
Trillian Map instance; key:value pairs can be modified and subsequently reset
without anyone noticing.

A future plan to deal with this is to create a *Logged Map*, which combines a
Trillian Map with a Trillian Log so that all published revisions of the Map
have their signed tree head data appended to the corresponding Map.

The mapping between the source Log data and the key:value data stored in the
Map is application-specific, and so is implemented as a Trillian personality.
This allows for wide flexibility in the mapping function:

 - The simplest example is a Log that holds a journal of pending mutations to
   the key:value data; the mapping function here simply applies a batch of
   mutations.
 - A more sophisticated example might log entries that are independently of
   interest (e.g. Web PKI certificates) and apply a more complex mapping
   function (e.g. map from domain name to public key for the domains covered by
   a certificate).

![Log-Backed Map](docs/images/LogBackedMapDesign.png)


Use Cases
---------

### Certificate Transparency Log

The most obvious application for Trillian in Log mode is to provide a
Certificate Transparency (RFC 6962) Log.  To do this, the CT Log personality
needs to include all of the certificate-specific processing – in particular,
checking that an item that has been suggested for inclusion is indeed a valid
certificate that chains to an accepted root.

### Verifiable Log-Backed Map

One useful application for Trillian in Map mode is to provide a verifiable
log-backed map, as described in the
[Verifiable Data Structures](docs/papers/VerifiableDataStructures.pdf) white
paper (which uses the term 'log-backed map').  To do this, a mapper personality
would monitor the additions of entries to a Log, potentially external, and would
write some kind of corresponding key:value data to a Trillian Map.

Clients of the log-backed map are then able to verify that the entries in the
Map they are shown are also seen by anyone auditing the Log for correct
operation, which in turn allows the client to trust the key/value pairs
returned by the Map.

A concrete example of this might be a log-backed map that monitors a
Certificate Transparency Log and builds a corresponding Map from domain names
to the set of certificates associated with that domain.

The following table summarizes properties of data structures laid in the
[Verifiable Data Structures](docs/papers/VerifiableDataStructures.pdf) white
paper. "Efficiently" means that a client can and should perform this validation
themselves.  "Full audit" means that to validate correctly, a client would need
to download the entire dataset, and is something that in practice we expect a
small number of dedicated auditors to perform, rather than being done by each
client.


|                                          |  Verifiable Log        |  Verifiable Map        |  Verifiable Log-Backed Map |
| ---------------------------------------- | ---------------------- | ---------------------- |--------------------------- |
| Prove inclusion of value                 |  Yes, efficiently      |  Yes, efficiently      |  Yes, efficiently          |
| Prove non-inclusion of value             |  Impractical           |  Yes, efficiently      |  Yes, efficiently          |
| Retrieve provable value for key          |  Impractical           |  Yes, efficiently      |  Yes, efficiently          |
| Retrieve provable current value for key  |  Impractical           |  No                    |  Yes, efficiently          |
| Prove append-only                        |  Yes, efficiently      |  No                    |  Yes, efficiently [1].     |
| Enumerate all entries                    |  Yes, by full audit    |  Yes, by full audit    |  Yes, by full audit        |
| Prove correct operation                  |  Yes, efficiently      |  No                    |  Yes, by full audit        |
| Enable detection of split-view           |  Yes, efficiently      |  Yes, efficiently      |  Yes, efficiently          |

- [1] -- although full audit is required to verify complete correct operation
