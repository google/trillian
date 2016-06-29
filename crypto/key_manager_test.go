package crypto

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"github.com/google/trillian"
	"github.com/stretchr/testify/assert"
	"math/big"
	"math/rand"
	"testing"
)

type ecdsaSig struct {
	R, S *big.Int
}

// demoPrivateKey must only be used for testing purposes
const demoPrivateKeyPass string = "towel"
const demoPrivateKey string = `
-----BEGIN EC PRIVATE KEY-----
Proc-Type: 4,ENCRYPTED
DEK-Info: DES-CBC,B71ECAB011EB4E8F

+6cz455aVRHFX5UsxplyGvFXMcmuMH0My/nOWNmYCL+bX2PnHdsv3dRgpgPRHTWt
IPI6kVHv0g2uV5zW8nRqacmikBFA40CIKp0SjRmi1CtfchzuqXQ3q40rFwCjeuiz
t48+aoeFsfU6NnL5sP8mbFlPze+o7lovgAWEqHEcebU=
-----END EC PRIVATE KEY-----`
const demoPublicKey string = `
-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEsAVg3YB0tOFf3DdC2YHPL2WiuCNR
1iywqGjjtu2dAdWktWqgRO4NTqPJXUggSQL3nvOupHB4WZFZ4j3QhtmWRg==
-----END PUBLIC KEY-----
`

func TestLoadDemoECDSAKey(t *testing.T) {
	km := new(KeyManager)

	// Obviously in real code we wouldn't use a fixed seed
	rand := rand.New(rand.NewSource(42))

	hasher := trillian.NewSHA256()

	err := km.LoadPrivateKey(demoPrivateKey, demoPrivateKeyPass)

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

	publicBlock, _ := pem.Decode([]byte(demoPublicKey))
	parsedKey, err := x509.ParsePKIXPublicKey(publicBlock.Bytes)

	if err != nil {
		panic(fmt.Errorf("Public test key failed to parse: %v", err))
	}

	publicKey := parsedKey.(*ecdsa.PublicKey)

	assert.True(t, ecdsa.Verify(publicKey, []byte("hello"), signature.R, signature.S),
		"Signature did not verify on round trip test")
}
