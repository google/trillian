package merkle

import (
	"bytes"
	"encoding/base64"
	"testing"

	"github.com/google/trillian"
)

const emptyMapRootB64 = "xmifEIEqCYCXbZUz2Dh1KCFmFZVn7DUVVxbBQTr1PWo="

func emptyMapRoot() []byte {
	r, err := base64.StdEncoding.DecodeString(emptyMapRootB64)
	if err != nil {
		panic("couldn't decode empty root base64 constant.")
	}
	return r
}

func TestNullHashes(t *testing.T) {
	mh := NewMapHasher(trillian.NewSHA256())
	emptyRoot := mh.HashChildren(mh.nullHashes[0], mh.nullHashes[0])
	if got, want := emptyRoot, emptyMapRoot(); !bytes.Equal(got, want) {
		t.Fatalf("Expected empty root of %v, got %v", want, got)
	}
}
