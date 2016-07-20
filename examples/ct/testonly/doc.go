/*
Package testonly contains code and data that should only be used by tests.
Production code MUST NOT depend on anything in this package. This will be enforced
by tools where possible.

As an example PEM encoded test certificates and helper functions to decode them are
suitable candidates for being placed in testonly.

This package should only contain CT specific code and certificate data.
*/
package testonly
