package trillian

import (
	"crypto"
	_ "crypto/sha256"
	"fmt"
)

// Hasher is the interface which must be implemented by hashers.
type Hasher struct {
	crypto.Hash
	alg HashAlgorithm
}

func NewHasher(alg HashAlgorithm) (Hasher, error) {
	switch alg {
	case HashAlgorithm_SHA256:
		return Hasher{crypto.SHA256, alg}, nil
	}
	return Hasher{}, fmt.Errorf("unsupported hash algorithm %v", alg)
}

// Calculates the digest of b according to the underlying algorithm.
func (h Hasher) Digest(b []byte) Hash {
	hr := h.New()
	hr.Write(b)
	return hr.Sum([]byte{})
}

// SHA256 is a stateless SHA-256 hashing function which conforms to the Hasher prototype.
func NewSHA256() Hasher {
	h, err := NewHasher(HashAlgorithm_SHA256)
	if err != nil {
		// the only error we could get would be "unsupported hash algo", and since we know we support SHA256 that "can't" happen, right?
		panic(err)
	}
	return h
}

func (h Hasher) HashAlgorithm() HashAlgorithm {
	return h.alg
}
