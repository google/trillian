/*
Package testonly contains code and data that should only be used by tests.
Production code MUST NOT depend on anything in this package. This will be enforced
by tools where possible.

As an example PEM encoded test certificates and helper functions to decode them are
suitable candidates for being placed in testonly. However, nothing specific to a
particular application should be added at this level. Do not add CT specific test
data for example.
*/
package testonly
