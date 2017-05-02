// Copyright 2017 Google Inc. All Rights Reserved.
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

package keys

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"fmt"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keyspb"
)

// PEMSignerFactory handles PEM-encoded private keys.
// It supports trees whose PrivateKey field is a:
// - keyspb.PEMKeyFile
// - keyspb.PrivateKey
// It implements keys.SignerFactory.
type PEMSignerFactory struct{}

// NewSigner returns a crypto.Signer for the given tree.
func (f PEMSignerFactory) NewSigner(ctx context.Context, tree *trillian.Tree) (crypto.Signer, error) {
	if tree.GetPrivateKey() == nil {
		return nil, fmt.Errorf("tree %d has no PrivateKey", tree.GetTreeId())
	}

	var privateKey ptypes.DynamicAny
	if err := ptypes.UnmarshalAny(tree.GetPrivateKey(), &privateKey); err != nil {
		return nil, fmt.Errorf("failed to unmarshal private key for tree %d: %v", tree.GetTreeId(), err)
	}

	switch privateKey := privateKey.Message.(type) {
	case *keyspb.PEMKeyFile:
		return NewFromPrivatePEMFile(privateKey.GetPath(), privateKey.GetPassword())
	case *keyspb.PrivateKey:
		return NewFromPrivateDER(privateKey.GetDer())
	}

	return nil, fmt.Errorf("unsupported PrivateKey type for tree %d: %T", tree.GetTreeId(), privateKey.Message)
}

// Generate creates a new private key for a tree based on a key specification.
// It returns a proto that can be used as the value of tree.PrivateKey.
func (f PEMSignerFactory) Generate(ctx context.Context, tree *trillian.Tree, spec *keyspb.Specification) (*any.Any, error) {
	if tree.PrivateKey != nil {
		return nil, fmt.Errorf("tree already has a private key")
	}

	key, err := NewFromSpec(spec)
	if err != nil {
		return nil, fmt.Errorf("error generating key: %v", err)
	}

	if SignatureAlgorithm(key.Public()) != tree.GetSignatureAlgorithm() {
		return nil, fmt.Errorf("Specification produces %T, but tree.SignatureAlgorithm = %v", key, tree.GetSignatureAlgorithm())
	}

	return marshalPrivateKey(key)
}

func marshalPrivateKey(key crypto.Signer) (*any.Any, error) {
	var privateKeyDER []byte
	var err error

	switch key := key.(type) {
	case *ecdsa.PrivateKey:
		privateKeyDER, err = x509.MarshalECPrivateKey(key)
	case *rsa.PrivateKey:
		privateKeyDER = x509.MarshalPKCS1PrivateKey(key)
	default:
		return nil, fmt.Errorf("unsupported key type: %T", key)
	}
	if err != nil {
		return nil, fmt.Errorf("error marshaling private key as DER: %v", err)
	}

	return ptypes.MarshalAny(&keyspb.PrivateKey{Der: privateKeyDER})
}
