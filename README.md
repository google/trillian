# Trillian
## General Transparency

Trillian is an implementation of the concepts described in the
[Verifiable Data Structures](docs/VerifiableDataStructures.pdf)
white paper, which in turn could be thought of as an extension and
generalisation of the ideas which underpin
[Certificate Transparency](https://certificate-transparency.org).

## Build

If you're not with the Go program of working within its own directory
tree, then:

    % cd <your favourite directory for git repos>
    % git clone https://github.com/google/trillian.git
    % ln -s `pwd`/trillian $GOPATH/src/github.com/google  # you may have to make this directory first
    % cd trillian
    % go get -d -v ./...

If you are with the Go program, then you know what to do.

## Test

To run the tests, you need to have an instance of MySQL running, and
configured like so:

    % mysql -u root -e 'DROP DATABASE IF EXISTS test;'
    % mysql -u root -e 'CREATE DATABASE test;'
    % mysql -u root -e "GRANT ALL ON test.* TO 'test'@'localhost' IDENTIFIED BY 'zaphod';"
    % mysql -u root -D test < storage/mysql/storage.sql

Then:

    % go test -v ./...

Mechanisms
----------

Trillian provides a Verifiable Log-Backed Map.
This data structure provides the efficient k/v queries of a map,
and is constructed from auditable, tamper-evident data structures.

A Verifiable Log-Backed Map ensures that clients can verify that the map they have
been shown has also been seen by anyone auditing the log for correct operation, which in turn
allows the client to trust the key/value pairs returned by the map.

### Data structure complexity

The following table summarizes properties of data structures laid in the
[Verifiable Data Structures](docs/VerifiableDataStructures.pdf) white paper.
“Efficiently” means that a client can and should perform this validation themselves.
“Full audit” means that to validate correctly, a client would need to download the entire dataset,
and is something that in practices we expect a small number of dedicated auditors to perform,
rather than being done by each client.

Verifiable Log-Backed Maps use a sparse Merkle Tree implementation. There is more
details on how these work in the [Revocation Transparency](docs/RevocationTransparency.pdf)
paper. Note that they are not specific to handling X.509 certificates and, as implemented
in this project, can store any type of data.

                                         |  Verifiable Log      |  Verifiable Map      |  Verifiable Log-Backed Map
-----------------------------------------|----------------------|----------------------|----------------------------
Prove inclusion of value                 |  Yes, efficiently    |  Yes, efficiently    |  Yes, efficiently
Prove non-inclusion of value             |  Impractical         |  Yes, efficiently    |  Yes, efficiently
Retrieve provable value for key          |  Impractical         |  Yes, efficiently    |  Yes, efficiently
Retrieve provable current value for key  |  Impractical         |  No                  |  Yes, efficiently
Prove append-only                        |  Yes, efficiently    |  No                  |  Yes, efficiently [1].
Enumerate all entries                    |  Yes, by full audit  |  Yes, by full audit  |  Yes, by full audit
Prove correct operation                  |  Yes, efficiently    |  No                  |  Yes, by full audit
Enable detection of split-view           |  Yes, efficiently    |  Yes, efficiently    |  Yes, efficiently

- [1] -- although full audit is required to verify complete correct operation

### What can be inserted?

In short: Anything.

This data structure does not describe the format of a log entry, nor specifically how it affects the map.
Importantly, the Verifiable Map may in fact be operated by an entirely different party than the backing log,
and in turn the log that it writes its Signed Map Heads to may in fact be operated by another party again.
