package merkle

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"math/big"
	"testing"

	"github.com/google/trillian"
)

// This root was calculated with the C++/Python sparse merkle tree code in the
// github.com/google/certificate-transparency repo.
const sparseEmptyRootHashB64 = "xmifEIEqCYCXbZUz2Dh1KCFmFZVn7DUVVxbBQTr1PWo="

// TODO(al): collect these test helpers together somewhere.
func mustDecode(b64 string) trillian.Hash {
	r, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		panic(r)
	}
	return r
}

// createHStar2Values builds a list of HStar2Value structs suitable for
// passing into a the HStar2 sparse merkle tree implementation.
// The map keys will be SHA256 hashed before being added to the returned
// structs.
func createHStar2Values(vs map[string]string) []HStar2Value {
	r := []HStar2Value{}
	for k := range vs {
		h := sha256.Sum256([]byte(k))
		r = append(r, HStar2Value{
			Index: new(big.Int).SetBytes(h[:]),
			Value: []byte(vs[k]),
		})
	}
	return r
}

func TestHStar2EmptyRootKAT(t *testing.T) {
	s := NewHStar2(NewRFC6962TreeHasher(trillian.NewSHA256()))
	root, err := s.HStar2(s.hasher.Size()*8, []HStar2Value{})
	if err != nil {
		t.Fatalf("Failed to calculate root: %v", err)
	}
	if expected, got := mustDecode(sparseEmptyRootHashB64), root; !bytes.Equal(expected, got) {
		t.Fatalf("Expected empty root:\n%v\nGot:\n%v", expected, got)
	}
}

func TestHStar2SimpleDataSetKAT(t *testing.T) {
	s := NewHStar2(NewRFC6962TreeHasher(trillian.NewSHA256()))
	vector := []struct{ k, v, rootB64 string }{
		{"a", "0", "nP1psZp1bu3jrY5Yv89rI+w5ywe9lLqI2qZi5ibTSF0="},
		{"b", "1", "EJ1Rw6DQT9bDn2Zbn7u+9/j799PSdqT9gfBymS9MBZY="},
		{"a", "2", "2rAZz4HJAMJqJ5c8ClS4wEzTP71GTdjMZMe1rKWPA5o="},
	}

	m := make(map[string]string)
	for i, x := range vector {
		m[x.k] = x.v
		values := createHStar2Values(m)
		root, err := s.HStar2(s.hasher.Size()*8, values)
		if err != nil {
			t.Fatalf("Failed to calculate root at iteration %d: %v", err, i)
		}
		if expected, got := mustDecode(x.rootB64), root; !bytes.Equal(expected, got) {
			t.Fatalf("Expected root:\n%v\nGot:\n%v", base64.StdEncoding.EncodeToString(expected), base64.StdEncoding.EncodeToString(got))
		}
	}
}
