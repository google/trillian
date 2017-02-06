package merkle

import (
	"bytes"
	"testing"

	"github.com/google/trillian/testonly"
)

func TestEmptyRoot(t *testing.T) {
	h, err := Factory("RFC6962-SHA256")
	if err != nil {
		t.Errorf("RFC6962-SHA256 hasher not available: %v", err)
	}
	// This root was calculated with the C++/Python sparse Merkle tree code in the
	// github.com/google/certificate-transparency repo.
	emptyRoot := testonly.MustDecodeBase64("xmifEIEqCYCXbZUz2Dh1KCFmFZVn7DUVVxbBQTr1PWo=")
	if got, want := h.NullHash(-1), emptyRoot; !bytes.Equal(got, want) {
		t.Errorf("Expected empty root. Got: \n%x\nWant:\n%x", got, want)
	}
}

func TestNullHashes(t *testing.T) {
	th, err := Factory("RFC6962-SHA256")
	if err != nil {
		t.Errorf("RFC6962-SHA256 hasher not available: %v", err)
	}
	// Generate a test vector.
	numEntries := th.Size() * 8
	tests := make([][]byte, numEntries, numEntries)
	tests[numEntries-1] = th.HashLeaf([]byte{})
	for i := numEntries - 2; i >= 0; i-- {
		tests[i] = th.HashChildren(tests[i+1], tests[i+1])
	}

	if got, want := th.Size()*8, 256; got != want {
		t.Fatalf("th.Size()*8: %v, want %v", got, want)
	}
	if got, want := tests[255], th.HashEmpty(); !bytes.Equal(got, want) {
		t.Fatalf("tests[255]: \n%x, want:\n%x", got, want)
	}
	if got, want := th.NullHash(255), th.HashEmpty(); !bytes.Equal(got, want) {
		t.Fatalf("NullHash(255): \n%x, want:\n%x", got, want)
	}

	for i, test := range tests {
		if got, want := th.NullHash(i), test; !bytes.Equal(got, want) {
			t.Errorf("NullHash(%v): \n%x, want:\n%x", i, got, want)
		}
	}
}
