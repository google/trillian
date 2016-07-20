package ct

import (
	"testing"

	"github.com/google/trillian/examples/ct/testonly"
	"github.com/stretchr/testify/assert"
)

func TestLoadSingleCertFromPEMs(t *testing.T) {
	for _, pem := range []string{testonly.CACertPEM, testonly.CACertPEMWithOtherStuff, testonly.CACertPEMDuplicated} {
		pool := NewPEMCertPool()

		ok := pool.AppendCertsFromPEM([]byte(pem))
		assert.True(t, ok, "Expected to append a certificate ok")
		assert.Equal(t, 1, len(pool.Subjects()), "Expected one cert in pool")
	}
}

func TestBadOrEmptyCertificateRejected(t *testing.T) {
	for _, pem := range []string{testonly.UnknownBlockTypePEM, testonly.CACertPEMBad} {
		pool := NewPEMCertPool()

		ok := pool.AppendCertsFromPEM([]byte(pem))
		assert.False(t, ok, "Accepted appending no certs")
		assert.Equal(t, 0, len(pool.Subjects()), "Expected no certs in pool")
	}
}

func TestLoadMultipleCertsFromPEM(t *testing.T) {
	pool := NewPEMCertPool()

	ok := pool.AppendCertsFromPEM([]byte(testonly.CACertMultiplePEM))
	assert.True(t, ok, "Rejected valid multiple certs")
	assert.Equal(t, 2, len(pool.Subjects()), "Expected two certs in pool")
}
