package crypto

import (
	"crypto"
	"crypto/rand"
	"encoding/json"
	"fmt"

	"github.com/benlaurie/objecthash/go/objecthash"
	"github.com/google/trillian"
	"github.com/golang/glog"
)

// rootForSerialization is used when creating the JSON to be fed to the hasher for signed roots
// Using this may look like copying protos to other protos but we want to ensure that only
// expected fields are included in the hash as this cannot change without impacting clients and
// the SignedLogRoot proto could evolve.
type rootForSerialization struct {
	TimestampNanos int64
	TreeSize       int64
	RootHash       []byte
}

// Signer is responsible for signing log-related data and producing the appropriate
// application specific signature objects.
type Signer struct {
	hasher       trillian.Hasher
	signer       crypto.Signer
	sigAlgorithm trillian.SignatureAlgorithm
}

// NewSigner creates a new LogSigner wrapping up a hasher and a signer. For the moment
// we only support SHA256 hashing and either ECDSA or RSA signing but this is not enforced
// here.
func NewSigner(hasher trillian.Hasher, signatureAlgorithm trillian.SignatureAlgorithm, signer crypto.Signer) *Signer {
	return &Signer{hasher, signer, signatureAlgorithm}
}

// Sign obtains a signature after first hashing the input data.
func (s Signer) Sign(data []byte) (trillian.DigitallySigned, error) {
	digest := s.hasher.Digest(data)

	if len(digest) != s.hasher.Size() {
		return trillian.DigitallySigned{}, fmt.Errorf("hasher returned unexpected digest length: %d, %d",
			len(digest), s.hasher.Size())
	}

	sig, err := s.signer.Sign(rand.Reader, digest, s.hasher)

	if err != nil {
		return trillian.DigitallySigned{}, err
	}

	return trillian.DigitallySigned{
		SignatureAlgorithm: s.sigAlgorithm,
		HashAlgorithm:      s.hasher.HashAlgorithm(),
		Signature:          sig}, nil
}

// LogRootToJson returns the JSON representation of a signed log root that should be fed to
// objecthash when verifying a signed root.
// TODO(Martin2112): Decouple code to verify roots from Signer as you need a private
// key to create these objects.
func (s Signer) LogRootToJson(root trillian.SignedLogRoot) (string, error) {
	rootToSerialize := rootForSerialization{TimestampNanos: root.TimestampNanos, TreeSize: root.TreeSize, RootHash: root.RootHash}

	serializedJson, err := json.Marshal(&rootToSerialize)

	if err != nil {
		glog.Warningf("Failed to create json root for signing: %v", err)
		return "", err
	}

	return string(serializedJson), nil
}

// SignLogRoot updates a log root to include a signature from the crypto signer this object
// was created with. Signatures use objecthash on a fixed JSON format of the root.
func (s Signer) SignLogRoot(root *trillian.SignedLogRoot) error {
	serializedJson, err := s.LogRootToJson(*root)

	if err != nil {
		// already logged, just return
		return err
	}

	objectHash := objecthash.CommonJSONHash(serializedJson)
	signature, err := s.Sign(objectHash[:])

	if err != nil {
		glog.Warningf("Signer failed to sign root: %v", err)
		return err
	}

	(*root).Signature = &signature
	return nil
}