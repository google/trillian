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
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"fmt"
	"testing"

	"github.com/google/trillian/crypto/keyspb"
)

func TestNewFromSpec(t *testing.T) {
	for _, test := range []struct {
		desc    string
		keySpec *keyspb.Specification
		wantErr bool
	}{
		{
			desc: "ECDSA with default params",
			keySpec: &keyspb.Specification{
				Params: &keyspb.Specification_EcdsaParams{},
			},
		},
		{
			desc: "ECDSA with explicit params",
			keySpec: &keyspb.Specification{
				Params: &keyspb.Specification_EcdsaParams{
					EcdsaParams: &keyspb.Specification_ECDSA{
						Curve: keyspb.Specification_ECDSA_P521,
					},
				},
			},
		},
		{
			desc: "RSA with default params",
			keySpec: &keyspb.Specification{
				Params: &keyspb.Specification_RsaParams{},
			},
		},
		{
			desc: "RSA with explicit params",
			keySpec: &keyspb.Specification{
				Params: &keyspb.Specification_RsaParams{
					RsaParams: &keyspb.Specification_RSA{
						Bits: 4096,
					},
				},
			},
		},
		{
			desc: "RSA with negative key size",
			keySpec: &keyspb.Specification{
				Params: &keyspb.Specification_RsaParams{
					RsaParams: &keyspb.Specification_RSA{
						Bits: -4096,
					},
				},
			},
			wantErr: true,
		},
		{
			desc: "RSA with insufficient key size",
			keySpec: &keyspb.Specification{
				Params: &keyspb.Specification_RsaParams{
					RsaParams: &keyspb.Specification_RSA{
						Bits: MinRsaKeySizeInBits - 1,
					},
				},
			},
			wantErr: true,
		},
		{
			desc:    "No params",
			keySpec: &keyspb.Specification{},
			wantErr: true,
		},
		{
			desc:    "Nil KeySpec",
			wantErr: true,
		},
	} {
		key, err := NewFromSpec(test.keySpec)
		if gotErr := err != nil; gotErr != test.wantErr {
			t.Errorf("%v: NewFromSpec() = (_, %v), want err? %v", test.desc, err, test.wantErr)
			continue
		} else if gotErr {
			continue
		}

		if err := checkKeyMatchesSpec(key, test.keySpec); err != nil {
			t.Errorf("%v: checkKeyMatchesSpec(): %v", test.desc, err)
		}
	}
}

func checkKeyMatchesSpec(key crypto.PrivateKey, spec *keyspb.Specification) error {
	switch params := spec.Params.(type) {
	case *keyspb.Specification_EcdsaParams:
		if key, ok := key.(*ecdsa.PrivateKey); ok {
			return checkEcdsaKeyMatchesParams(key, params.EcdsaParams)
		}
		return fmt.Errorf("%T, want *ecdsa.PrivateKey", key)
	case *keyspb.Specification_RsaParams:
		if key, ok := key.(*rsa.PrivateKey); ok {
			return checkRsaKeyMatchesParams(key, params.RsaParams)
		}
		return fmt.Errorf("%T, want *rsa.PrivateKey", key)
	}

	return fmt.Errorf("%T is not a supported keyspb.Specification.Params type", spec.Params)
}

func checkEcdsaKeyMatchesParams(key *ecdsa.PrivateKey, params *keyspb.Specification_ECDSA) error {
	wantCurve := curveFromParams(params)
	if wantCurve.Params().Name != key.Params().Name {
		return fmt.Errorf("ECDSA key on %v curve, want %v curve", key.Params().Name, wantCurve.Params().Name)
	}

	return nil
}

func checkRsaKeyMatchesParams(key *rsa.PrivateKey, params *keyspb.Specification_RSA) error {
	wantBits := defaultRsaKeySizeInBits
	if params.GetBits() != 0 {
		wantBits = int(params.GetBits())
	}

	if got, want := key.N.BitLen(), wantBits; got != want {
		return fmt.Errorf("%v-bit RSA key, want %v-bit", got, want)
	}

	return nil
}
