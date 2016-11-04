package crypto

import (
	gocrypto "crypto"
	_ "crypto/sha256" // Register the SHA256 algorithm
	"fmt"

	"github.com/google/trillian"
)

// Hasher is the interface which must be implemented by hashers.
type Hasher struct {
	gocrypto.Hash
	alg trillian.HashAlgorithm
}

// NewHasher creates a Hasher instance for the specified algorithm.
func NewHasher(alg trillian.HashAlgorithm) (Hasher, error) {
	switch alg {
	case trillian.HashAlgorithm_SHA256:
		return Hasher{gocrypto.SHA256, alg}, nil
	}
	return Hasher{}, fmt.Errorf("unsupported hash algorithm %v", alg)
}

// Digest calculates the digest of b according to the underlying algorithm.
func (h Hasher) Digest(b []byte) []byte {
	hr := h.New()
	hr.Write(b)
	return hr.Sum([]byte{})
}

// NewSHA256 creates a Hasher instance for a stateless SHA256 hashing function.
func NewSHA256() Hasher {
	h, err := NewHasher(trillian.HashAlgorithm_SHA256)
	if err != nil {
		// the only error we could get would be "unsupported hash algo", and since we know we support SHA256 that "can't" happen, right?
		panic(err)
	}
	return h
}

// HashAlgorithm returns an identifier for the underlying hash algorithm for a Hasher instance.
func (h Hasher) HashAlgorithm() trillian.HashAlgorithm {
	return h.alg
}
