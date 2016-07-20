/*
Package testonly contains code and data that should only be used by tests.
Production code MUST NOT depend on anything in this package. This will be enforced
by tools where possible.

As an example PEM encoded test certificates and helper functions to decode them are
suitable candidates for being placed in testonly.
*/
package testonly

// TODO(Martin2112): When all current work has landed split this up so that all the CT
// specific test data is moved to an appropriate testonly directory under the CT frontend.
