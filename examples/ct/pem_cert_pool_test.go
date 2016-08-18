package ct

import (
	"testing"

	"github.com/google/trillian/examples/ct/testonly"
)

func TestLoadSingleCertFromPEMs(t *testing.T) {
	for _, pem := range []string{testonly.CACertPEM, testonly.CACertPEMWithOtherStuff, testonly.CACertPEMDuplicated} {
		pool := NewPEMCertPool()

		ok := pool.AppendCertsFromPEM([]byte(pem))
		if !ok {
			t.Fatal("Expected to append a certificate ok")
		}
		if got, want := len(pool.Subjects()), 1; got != want {
			t.Fatalf("Got %d cert(s) in the pool, expected %d", got, want)
		}
	}
}

func TestBadOrEmptyCertificateRejected(t *testing.T) {
	for _, pem := range []string{testonly.UnknownBlockTypePEM, testonly.CACertPEMBad} {
		pool := NewPEMCertPool()

		ok := pool.AppendCertsFromPEM([]byte(pem))
		if ok {
			t.Fatal("Expected appending no certs")
		}
		if got, want := len(pool.Subjects()), 0; got != want {
			t.Fatalf("Got %d cert(s) in pool, expected %d", got, want)
		}
	}
}

func TestLoadMultipleCertsFromPEM(t *testing.T) {
	pool := NewPEMCertPool()

	ok := pool.AppendCertsFromPEM([]byte(testonly.CACertMultiplePEM))
	if !ok {
		t.Fatal("Rejected valid multiple certs")
	}
	if got, want := len(pool.Subjects()), 2; got != want {
		t.Fatalf("Got %d certs in pool, expected %d", got, want)
	}
}
