Directory Contents
==================

This directory holds data files for testing; **under no circumstances should these
files be used in production**.

Some of the data files are generated from other data files; the [`Makefile`](Makefile) has commands for doing this, but
the generated files are checked in for convenience.


Trillian Server Keys
--------------------

Files of the form `*-server.privkey.pem` hold private keys for Trillian servers, with the corresponding public keys
stored in `*-server.pubkey.pem`.  The following sets of files are available:

 - `log-rpc-server`: Log RPC server; password `towel`.
 - `map-rpc-server`: Map RPC server; password `towel`.
