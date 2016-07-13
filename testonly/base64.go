package testonly

import (
	"encoding/base64"

	"github.com/google/trillian"
)

// MustDecodeBase64 expects a base 64 encoded string input and panics if it cannot be decoded
func MustDecodeBase64(b64 string) trillian.Hash {
	r, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		panic(r)
	}
	return r
}
