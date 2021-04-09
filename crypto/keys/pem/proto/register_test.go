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
	"testing"

	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/crypto/keys/testonly"
	"github.com/google/trillian/crypto/keyspb"
	"google.golang.org/protobuf/proto"
)

func TestProtoHandler(t *testing.T) {
	ctx := context.Background()

	for _, test := range []struct {
		desc     string
		keyProto proto.Message
		wantErr  bool
	}{
		{
			desc: "PEMKeyFile",
			keyProto: &keyspb.PEMKeyFile{
				Path:     "../../../../testdata/log-rpc-server.privkey.pem",
				Password: "towel",
			},
		},
		{
			desc: "PemKeyFile with non-existent file",
			keyProto: &keyspb.PEMKeyFile{
				Path: "non-existent.pem",
			},
			wantErr: true,
		},
		{
			desc: "PemKeyFile with wrong password",
			keyProto: &keyspb.PEMKeyFile{
				Path:     "../../../../testdata/log-rpc-server.privkey.pem",
				Password: "wrong-password",
			},
			wantErr: true,
		},
		{
			desc: "PemKeyFile with missing password",
			keyProto: &keyspb.PEMKeyFile{
				Path: "../../../../testdata/log-rpc-server.privkey.pem",
			},
			wantErr: true,
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
