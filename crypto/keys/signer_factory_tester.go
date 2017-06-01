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
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/asn1"
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/trillian/crypto/keyspb"
)

// NewSignerTest is a test case to be run by TestNewSigner().
type NewSignerTest struct {
	// Name describes the test.
	Name string
	// KeyProto is marshaled as a protobuf "Any" and passed to SignerFactory.NewSigner().
	KeyProto proto.Message
	// WantErr should be true if SignerFactory.NewSigner() is expected to return an error.
	WantErr bool
}

// SignerFactoryTester runs a suite of tests against a SignerFactory implementation.
type SignerFactoryTester struct {
	// NewSignerFactory returns an SignerFactory instance setup for testing.
	NewSignerFactory func() SignerFactory
	// NewSignerTests are additional test cases to exercise the specific
	// PrivateKey protos that this SignerFactory implementation supports.
	NewSignerTests []NewSignerTest
}

// RunAllTests runs all SignerFactory tests.
func (tester *SignerFactoryTester) RunAllTests(t *testing.T) {
	t.Run("TestNewSigner", tester.TestNewSigner)
	t.Run("TestGenerate", tester.TestGenerate)
}

// TestNewSigner runs test on the SignerFactory's NewSigner() method.
func (tester *SignerFactoryTester) TestNewSigner(t *testing.T) {
	for _, test := range append(tester.NewSignerTests, []NewSignerTest{
		{
			Name:     "Unsupported protobuf message type",
			KeyProto: &empty.Empty{},
			WantErr:  true,
		},
	}...) {
		anyKeyProto, err := ptypes.MarshalAny(test.KeyProto)
		if err != nil {
			t.Errorf("%v: Could not marshal test.KeyProto as protobuf Any: %v", test.Name, err)
		}

		signer, err := tester.NewSignerFactory().NewSigner(context.Background(), anyKeyProto)
		switch gotErr := err != nil; {
		case gotErr != test.WantErr:
			t.Errorf("%v: Signer() = (%v, %v), want err? %v", test.Name, signer, err, test.WantErr)
			continue
		case gotErr:
			continue
		}

		// Check that the returned signer can produce signatures successfully.
		hasher := crypto.SHA256
		digest := sha256.Sum256([]byte("test"))
		signature, err := signer.Sign(rand.Reader, digest[:], hasher)
		if err != nil {
			t.Errorf("%v: Signer().Sign() = (_, %v), want (_, nil)", test.Name, err)
		}

		if err := verify(signer.Public(), digest[:], signature, hasher, hasher); err != nil {
			t.Errorf("%v: %v", test.Name, err)
		}
	}
}

// TestGenerate runs test on the SignerFactory's Generate() method.
func (tester *SignerFactoryTester) TestGenerate(t *testing.T) {
	for _, test := range []struct {
		Name    string
		KeySpec *keyspb.Specification
		WantErr bool
	}{
		{
			Name: "RSA",
			KeySpec: &keyspb.Specification{
				Params: &keyspb.Specification_RsaParams{
					RsaParams: &keyspb.Specification_RSA{},
				},
			},
		},
		{
			Name: "ECDSA",
			KeySpec: &keyspb.Specification{
				Params: &keyspb.Specification_EcdsaParams{
					EcdsaParams: &keyspb.Specification_ECDSA{},
				},
			},
		},
		{
			Name: "RSA with insufficient key size",
			KeySpec: &keyspb.Specification{
				Params: &keyspb.Specification_RsaParams{
					RsaParams: &keyspb.Specification_RSA{
						Bits: 1024,
					},
				},
			},
			WantErr: true,
		},
	} {
		ctx := context.Background()
		sf := tester.NewSignerFactory()

		key, err := sf.Generate(ctx, test.KeySpec)
		if gotErr := err != nil; gotErr != test.WantErr {
			t.Errorf("%v: Generate() = (_, %v), want err? %v", test.Name, err, test.WantErr)
			continue
		} else if gotErr {
			continue
		}

		signer, err := sf.NewSigner(ctx, key)
		if err != nil {
			t.Errorf("%v: NewSigner() = (_, %v), want (_, nil)", test.Name, err)
			continue
		}

		publicKey := signer.Public()
		switch test.KeySpec.Params.(type) {
		case *keyspb.Specification_RsaParams:
			if _, ok := publicKey.(*rsa.PublicKey); !ok {
				t.Errorf("%v: signer.Public() = %T, want *rsa.PublicKey", test.Name, publicKey)
			}
		case *keyspb.Specification_EcdsaParams:
			if _, ok := publicKey.(*ecdsa.PublicKey); !ok {
				t.Errorf("%v: signer.Public() = %T, want *ecdsa.PublicKey", test.Name, publicKey)
			}
		}
	}
}

func mustMarshalAny(pb proto.Message) *any.Any {
	a, err := ptypes.MarshalAny(pb)
	if err != nil {
		panic(err)
	}
	return a
}

// verify checks that sig is a valid signature for a digest (hash of some data).
// A private key will have been used to generate the signature;
// the corresponding public key must be provided in order to verify the signature.
// Hasher must identify the hash algorithm that was used to produce digest.
// The options, if any, that were used when generating the signature must be provided.
// If sig is an RSA PSS signature, opts must be *rsa.PSSOptions.
func verify(pub crypto.PublicKey, digest, sig []byte, hasher crypto.Hash, opts crypto.SignerOpts) error {
	switch pub := pub.(type) {
	case *ecdsa.PublicKey:
		return verifyECDSA(pub, digest, sig)
	case *rsa.PublicKey:
		return verifyRSA(pub, digest, sig, hasher, opts)
	}

	return fmt.Errorf("unknown public key type: %T", pub)
}

func verifyECDSA(pub *ecdsa.PublicKey, digest, sig []byte) error {
	var ecdsaSig struct {
		R, S *big.Int
	}

	rest, err := asn1.Unmarshal(sig, &ecdsaSig)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return fmt.Errorf("ECDSA signature %v bytes longer than expected", len(rest))
	}

	if !ecdsa.Verify(pub, digest, ecdsaSig.R, ecdsaSig.S) {
		return errors.New("ECDSA signature failed verification")
	}
	return nil
}

func verifyRSA(pub *rsa.PublicKey, digest, sig []byte, hasher crypto.Hash, opts crypto.SignerOpts) error {
	if pssOpts, ok := opts.(*rsa.PSSOptions); ok {
		return rsa.VerifyPSS(pub, hasher, digest, sig, pssOpts)
	}
	return rsa.VerifyPKCS1v15(pub, hasher, digest, sig)
}
