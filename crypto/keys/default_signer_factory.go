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
	"fmt"
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/google/trillian/crypto/keys/der"
	"github.com/google/trillian/crypto/keys/pem"
	"github.com/google/trillian/crypto/keys/pkcs11"
	"github.com/google/trillian/crypto/keyspb"
)

// SignerFactory gets and creates cryptographic signers.
// This may be done by loading a key from a file, interfacing with a HSM, or
// sending requests to a remote key management service, to give a few examples.
type SignerFactory interface {
	// NewSigner uses the information in the provided protobuf message to obtain and return a crypto.Signer.
	NewSigner(context.Context, proto.Message) (crypto.Signer, error)

	// Generate creates a new private key based on a key specification.
	// It returns a proto that describes how to access that key.
	// This proto can be passed to NewSigner() to get a crypto.Signer.
	Generate(context.Context, *keyspb.Specification) (proto.Message, error)
}

// DefaultSignerFactory produces a crypto.Signer from a protobuf message describing a key.
// It can also generate new private keys.
// It implements keys.SignerFactory.
type DefaultSignerFactory struct {
	pkcs11Module string
	pMu          sync.Mutex
}

// NewSigner uses the information in pb to return a crypto.Signer.
// pb must be a keyspb.PEMKeyFile, keyspb.PrivateKey or keyspb.PKCS11Config.
func (f *DefaultSignerFactory) NewSigner(ctx context.Context, pb proto.Message) (crypto.Signer, error) {
	switch privateKey := pb.(type) {
	case *keyspb.PEMKeyFile:
		return pem.ReadPrivateKeyFile(privateKey.GetPath(), privateKey.GetPassword())
	case *keyspb.PrivateKey:
		return der.UnmarshalPrivateKey(privateKey.GetDer())
	case *keyspb.PKCS11Config:
		return pkcs11.FromConfig(f.pkcs11Module, privateKey)
	}

	return nil, fmt.Errorf("unsupported private key protobuf type: %T", pb)
}

// Generate creates a new private key based on a key specification.
// It returns a proto that can be passed to NewSigner() to get a crypto.Signer.
func (f *DefaultSignerFactory) Generate(ctx context.Context, spec *keyspb.Specification) (proto.Message, error) {
	key, err := NewFromSpec(spec)
	if err != nil {
		return nil, fmt.Errorf("error generating key: %v", err)
	}

	keyDER, err := der.MarshalPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("error marshaling private key as DER: %v", err)
	}

	return &keyspb.PrivateKey{Der: keyDER}, nil
}

// SetPKCS11Module sets the PKCS#11 module used to load PKCS#11 keys.
func (f *DefaultSignerFactory) SetPKCS11Module(modulePath string) {
	f.pMu.Lock()
	defer f.pMu.Unlock()
	f.pkcs11Module = modulePath
}
