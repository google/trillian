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
    % go test -v ./...

If you are with the Go program, then you know what to do.
