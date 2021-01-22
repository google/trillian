# Integration tests

This directory contains some integration tests which are intended to serve as a
way of checking that various top-level binaries work as intended, as well as
providing a simple example of how to run and use the various servers.


## Running the tests

### Log integration test
To run the Log integration test, ensure that you have a mysql database
configured and running, with the Trillian schema loaded (see the
[main README](../README.md) for details), and then run
`log_integration_test.sh`.
