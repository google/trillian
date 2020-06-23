// Copyright 2017 Google LLC. All Rights Reserved.
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

package proto

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/crypto/keys/testonly"
	"github.com/google/trillian/crypto/keyspb"
)

func TestProtoHandler(t *testing.T) {
	// ECDSA private key in DER format.
	keyDER, err := base64.StdEncoding.DecodeString("MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgS81mfpvtTmaINn+gtrYXn4XpxxgE655GLSKsA3hhjHmhRANCAASwBWDdgHS04V/cN0LZgc8vZaK4I1HWLLCoaOO27Z0B1aS1aqBE7g1Oo8ldSCBJAvee866kcHhZkVniPdCG2ZZG")
	if err != nil {
		t.Fatalf("Could not decode test key: %v", err)
	}

	ctx := context.Background()

	for _, test := range []struct {
		desc     string
		keyProto proto.Message
		wantErr  bool
	}{
		{
			desc: "PrivateKey",
			keyProto: &keyspb.PrivateKey{
				Der: keyDER,
			},
		},
		{
			desc: "PrivateKey with invalid DER",
			keyProto: &keyspb.PrivateKey{
				Der: []byte("foobar"),
			},
			wantErr: true,
		},
		{
			desc:     "PrivateKey with missing DER",
			keyProto: &keyspb.PrivateKey{},
			wantErr:  true,
		},
	} {
		signer, err := keys.NewSigner(ctx, test.keyProto)
		if gotErr := err != nil; gotErr != test.wantErr {
			t.Errorf("%v: NewSigner(_, %#v) = (_, %q), want (_, nil)", test.desc, test.keyProto, err)
			continue
		} else if gotErr {
			continue
		}

		// Check that the returned signer can produce signatures successfully.
		if err := testonly.SignAndVerify(signer, signer.Public()); err != nil {
			t.Errorf("%v: SignAndVerify() = %q, want nil", test.desc, err)
		}
	}
}
