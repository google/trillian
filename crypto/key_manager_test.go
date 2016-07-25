package crypto

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"math/big"
	"math/rand"
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/testonly"
	"github.com/stretchr/testify/assert"
)

type ecdsaSig struct {
	R, S *big.Int
}

func TestLoadDemoECDSAKeyAndSign(t *testing.T) {
	km := new(PEMKeyManager)

	// Obviously in real code we wouldn't use a fixed seed
	rand := rand.New(rand.NewSource(42))

	hasher := trillian.NewSHA256()

	err := km.LoadPrivateKey(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass)

	if err != nil {
		t.Fatalf("Failed to load key: %v", err)
	}

	signer, err := km.Signer()

	if err != nil {
		t.Fatalf("Failed to create signer: %v", err)
	}

	signed, err := signer.Sign(rand, []byte("hello"), hasher)

	if err != nil {
		t.Fatalf("Failed to sign: %v", err)
	}

	// Do a round trip by verifying the signature using the public key
	var signature ecdsaSig
	_, err = asn1.Unmarshal(signed, &signature)

	if err != nil {
		t.Fatalf("Failed to unmarshal signature as asn.1")
	}

	publicBlock, rest := pem.Decode([]byte(testonly.DemoPublicKey))
	parsedKey, err := x509.ParsePKIXPublicKey(publicBlock.Bytes)

	if len(rest) > 0 {
		t.Fatal("Extra data after key in PEM string")
	}

	if err != nil {
		panic(fmt.Errorf("Public test key failed to parse: %v", err))
	}

	publicKey := parsedKey.(*ecdsa.PublicKey)

	assert.True(t, ecdsa.Verify(publicKey, []byte("hello"), signature.R, signature.S),
		"Signature did not verify on round trip test")
}

func TestLoadDemoECDSAPublicKey(t *testing.T) {
	km := new(PEMKeyManager)

	if err := km.LoadPublicKey(testonly.DemoPublicKey); err != nil {
		t.Fatal("Failed to load public key")
	}

	key, err := km.GetPublicKey()

	assert.NoError(t, err, "unexpected error getting public key")

	if key == nil {
		t.Fatal("Key manager did not return public key after loading it")
	}

	// Additional sanity check on type as we know it must be an ECDSA key

	if _, ok := key.(*ecdsa.PublicKey); !ok {
		t.Fatalf("Expected to have loaded an ECDSA key but got: %v", key)
	}
}
