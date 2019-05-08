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

package keys_test

import (
	"testing"

	. "github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/crypto/keys/testonly"
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
			desc: "Ed25519",
			keySpec: &keyspb.Specification{
				Params: &keyspb.Specification_Ed25519Params{},
			},
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

		if err := testonly.CheckKeyMatchesSpec(key, test.keySpec); err != nil {
			t.Errorf("%v: CheckKeyMatchesSpec(): %v", test.desc, err)
		}
	}
}
