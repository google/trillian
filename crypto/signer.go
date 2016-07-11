package crypto

import (
	"crypto"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strconv"

	"github.com/benlaurie/objecthash/go/objecthash"
	"github.com/golang/glog"
	"github.com/google/trillian"
)

// Constants used as map keys when building input for ObjectHash. They must not be changed
// as this will change the output of hashRoot()
const (
	mapKeyRootHash string = "RootHash"
	mapKeyTimestampNanos string = "TimestampNanos"
	mapKeyTreeSize string = "TreeSize"
)

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

func (s Signer) hashRoot(root trillian.SignedLogRoot) []byte {
	rootMap := make(map[string]interface{})

	// Pull out the fields we want to hash. Caution: use string format for int64 values as they
	// can overflow when JSON encoded otherwise (it uses floats). We want to be sure that people
	// using JSON to verify hashes can build the exact same input to ObjectHash.
	rootMap[mapKeyRootHash] = base64.StdEncoding.EncodeToString(root.RootHash)
	rootMap[mapKeyTimestampNanos] = strconv.FormatInt(root.TimestampNanos, 10)
	rootMap[mapKeyTreeSize] = strconv.FormatInt(root.TreeSize, 10)

	hash := objecthash.ObjectHash(rootMap)

	return hash[:]
}

// SignLogRoot updates a log root to include a signature from the crypto signer this object
// was created with. Signatures use objecthash on a fixed JSON format of the root.
func (s Signer) SignLogRoot(root *trillian.SignedLogRoot) error {
	objectHash := s.hashRoot(*root)
	signature, err := s.Sign(objectHash[:])

	if err != nil {
		glog.Warningf("Signer failed to sign root: %v", err)
		return err
	}

	root.Signature = &signature
	return nil
}
