package merkle

import (
	"bytes"
	"testing"

	"github.com/google/trillian"
)

const (
	// echo -n | sha256sum
	rfc6962EmptyHashHex = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	// echo -n 004C313233343536 | xxd -r -p | sha256sum
	rfc6962LeafL123456HashHex = "395aa064aa4c29f7010acfe3f25db9485bbd4b91897b6ad7ad547639252b4d56"
	// echo -n 014E3132334E343536 | xxd -r -p | sha256sum
	rfc6962NodeN123N456HashHex = "aa217fe888e47007fa15edab33c2b492a722cb106c64667fc2b044444de66bbb"
)

func ensureHashMatches(expected, actual []byte, testCase string, t *testing.T) {
	if bytes.Compare(expected, actual) != 0 {
		t.Fatalf("Hash mismatch testing %s expected %v, got: %v", testCase, expected, actual)
	}
}

func TestRfc6962Hasher(t *testing.T) {
	hasher := NewRFC6962TreeHasher(trillian.NewSHA256())

	ensureHashMatches(mustHexDecode(rfc6962EmptyHashHex), hasher.HashEmpty(), "RFC962 Empty", t)
	ensureHashMatches(mustHexDecode(rfc6962LeafL123456HashHex), hasher.HashLeaf([]byte("L123456")), "RFC6962 Leaf", t)
	ensureHashMatches(mustHexDecode(rfc6962NodeN123N456HashHex), hasher.HashChildren([]byte("N123"), []byte("N456")), "RFC6962 Node", t)
}
