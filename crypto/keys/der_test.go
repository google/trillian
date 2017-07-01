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
	"testing"

	"github.com/google/trillian/crypto/keyspb"
)

func TestNewPrivateKeyProtoFromSpec(t *testing.T) {
	ctx := context.Background()

	for _, test := range []struct {
		desc    string
		keySpec *keyspb.Specification
		wantErr bool
	}{
		{
			desc: "ECDSA",
			keySpec: &keyspb.Specification{
				Params: &keyspb.Specification_EcdsaParams{},
			},
		},
		{
			desc: "RSA",
			keySpec: &keyspb.Specification{
				Params: &keyspb.Specification_RsaParams{},
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
		pb, err := NewPrivateKeyProtoFromSpec(ctx, test.keySpec)
		if gotErr := err != nil; gotErr != test.wantErr {
			t.Errorf("%v: NewPrivateKeyProtoFromSpec() = (_, %q), want err? %v", test.desc, err, test.wantErr)
			continue
		} else if gotErr {
			continue
		}

		// Get the key out of the proto, check that it matches the spec and test that it works.
		key, err := NewFromPrivateKeyProto(ctx, pb)
		if err != nil {
			t.Errorf("%v: NewFromPrivateKeyProto(%#v) = (_, %q), want (_, nil)", test.desc, pb, err)
		}

		if err := checkKeyMatchesSpec(key, test.keySpec); err != nil {
			t.Errorf("%v: checkKeyMatchesSpec() => %v", test.desc, err)
		}

		if err := signAndVerify(key, key.Public()); err != nil {
			t.Errorf("%v: signAndVerify(%#v) = %q, want nil")
		}
	}
}
