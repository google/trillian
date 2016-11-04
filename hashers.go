package trillian

import (
	"crypto"
	_ "crypto/sha256" // Register the SHA256 algorithm
	"fmt"
)

// Hasher is the interface which must be implemented by hashers.
type Hasher struct {
	crypto.Hash
	alg HashAlgorithm
}

// NewHasher creates a Hasher instance for the specified algorithm.
func NewHasher(alg HashAlgorithm) (Hasher, error) {
	switch alg {
	case HashAlgorithm_SHA256:
		return Hasher{crypto.SHA256, alg}, nil
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
	h, err := NewHasher(HashAlgorithm_SHA256)
	if err != nil {
		// the only error we could get would be "unsupported hash algo", and since we know we support SHA256 that "can't" happen, right?
		panic(err)
	}
	return h
}

// HashAlgorithm returns an identifier for the underlying hash algorithm for a Hasher instance.
func (h Hasher) HashAlgorithm() HashAlgorithm {
	return h.alg
}
