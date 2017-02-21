// Copyright 2016 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	"github.com/google/trillian/crypto/sigpb"
)

// Constants used as map keys when building input for ObjectHash. They must not be changed
// as this will change the output of hashRoot()
const (
	mapKeyRootHash       string = "RootHash"
	mapKeyTimestampNanos string = "TimestampNanos"
	mapKeyTreeSize       string = "TreeSize"
)

var (
	signerHashLookup = map[sigpb.DigitallySigned_HashAlgorithm]crypto.Hash{
		sigpb.DigitallySigned_SHA256: crypto.SHA256,
	}
	reverseSignerHashLookup = map[crypto.Hash]sigpb.DigitallySigned_HashAlgorithm{
		crypto.SHA256: sigpb.DigitallySigned_SHA256,
	}
)

// Signer is responsible for signing log-related data and producing the appropriate
// application specific signature objects.
type Signer struct {
	hash         crypto.Hash
	signer       crypto.Signer
	sigAlgorithm sigpb.DigitallySigned_SignatureAlgorithm
}

// NewSigner creates a new Signer wrapping up a hasher and a signer. For the moment
// we only support SHA256 hashing and either ECDSA or RSA signing but this is not enforced
// here.
func NewSigner(sigAlgo sigpb.DigitallySigned_SignatureAlgorithm, signer crypto.Signer) *Signer {
	return &Signer{
		hash:         crypto.SHA256,
		signer:       signer,
		sigAlgorithm: sigAlgo,
	}
}

// Sign obtains a signature after first hashing the input data.
func (s Signer) Sign(data []byte) (sigpb.DigitallySigned, error) {
	h := s.hash.New()
	h.Write(data)
	digest := h.Sum(nil)

	if len(digest) != s.hash.Size() {
		return sigpb.DigitallySigned{}, fmt.Errorf("hasher returned unexpected digest length: %d, %d",
			len(digest), s.hash.Size())
	}

	sig, err := s.signer.Sign(rand.Reader, digest, s.hash)

	if err != nil {
		return sigpb.DigitallySigned{}, err
	}

	return sigpb.DigitallySigned{
		SignatureAlgorithm: s.sigAlgorithm,
		HashAlgorithm:      reverseSignerHashLookup[s.hash],
		Signature:          sig,
	}, nil
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
func (s Signer) SignLogRoot(root trillian.SignedLogRoot) (sigpb.DigitallySigned, error) {
	objectHash := s.hashRoot(root)
	signature, err := s.Sign(objectHash[:])

	if err != nil {
		glog.Warningf("Signer failed to sign root: %v", err)
		return sigpb.DigitallySigned{}, err
	}

	return signature, nil
}
