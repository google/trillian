/*
Package ct contains a usage example by providing an implementation of an RFC6962 compatible CT
log server using a Trillian log server as backend storage via its GRPC API.

IMPORTANT: Only code rooted within this part of the tree should refer to the CT
Github repository. Other parts of the system must not assume that the data they're
processing is X.509 or CT related.

The CT repository can be found at: https://github.com/google/certificate-transparency
*/
package ct
