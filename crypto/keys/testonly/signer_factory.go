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

package testonly

import (
	"context"
	"crypto"

	"github.com/google/trillian"
	"github.com/google/trillian/errors"
	ttestonly "github.com/google/trillian/testonly"
)

// SignerFactory is a stub keys.SignerFactory.
type SignerFactory struct {
	// signer is the crypto.Signer that will be returned by NewSigner().
	signer crypto.Signer
	// newSignerErr is the error that will be returned by NewSigner().
	newSignerErr error
}

// NewSigner returns a fake signer or an error, depending on how it was created.
func (s SignerFactory) NewSigner(ctx context.Context, tree *trillian.Tree) (crypto.Signer, error) {
	return s.signer, s.newSignerErr
}

// NewSignerFactory returns a stub SignerFactory.
// This stub will provide fake crypto.Signers that always produce the same signature regardless of input.
func NewSignerFactory() SignerFactory {
	return SignerFactory{
		signer: ttestonly.NewSignerWithFixedSig(ttestonly.DemoPublicKey, []byte("test")),
	}
}

// NewSignerFactoryWithErr returns a stub SignerFactory whose NewSigner() method always returns err.
func NewSignerFactoryWithErr(err error) SignerFactory {
	return SignerFactory{
		newSignerErr: err,
	}
}

// KeyGenerator is a stub keys.SignerFactory and keys.Generator.
type KeyGenerator struct {
	SignerFactory
	// generateErr is the error that will be returned by Generate().
	generateErr error
}

// Generate simulates generation of a new private key for the given tree.
func (s KeyGenerator) Generate(ctx context.Context, tree *trillian.Tree) error {
	if s.generateErr != nil {
		return s.generateErr
	}

	s.SignerFactory = NewSignerFactory()
	s.generateErr = errors.New(errors.AlreadyExists, "key already exists")
	return nil
}

// NewKeyGenerator returns a stub that implements both keys.SignerFactory and keys.Generator.
// Its NewSigner() method will return a NotFound error until Generate() is called, after which
// it will behave like the stub returned by NewSignerFactory().
// If Generate() is called more than once, it will return an AlreadyExists error.
func NewKeyGenerator() KeyGenerator {
	return KeyGenerator{
		SignerFactory: NewSignerFactoryWithErr(errors.New(errors.NotFound, "key not found")),
	}
}

// NewKeyGeneratorWithErr returns a stub that implements both keys.SignerFactory and keys.Generator.
// Its NewSigner() method will return a NotFound error.
// Its Generate() method will return err.
func NewKeyGeneratorWithErr(err error) KeyGenerator {
	keygen := NewKeyGenerator()
	keygen.generateErr = err
	return keygen
}
