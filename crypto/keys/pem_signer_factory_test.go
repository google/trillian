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
	"crypto/rand"
	"crypto/sha256"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/google/trillian"
)

func marshalAny(pb proto.Message) *any.Any {
	a, err := ptypes.MarshalAny(pb)
	if err != nil {
		panic(err)
	}
	return a
}

func TestPEMSignerFactoryNewSigner(t *testing.T) {
	for _, test := range []struct {
		name    string
		tree    *trillian.Tree
		wantErr bool
	}{
		{
			name: "PEMKeyFile",
			tree: &trillian.Tree{
				PrivateKey: marshalAny(&trillian.PEMKeyFile{
					Path:     "../../testdata/log-rpc-server.privkey.pem",
					Password: "towel",
				}),
			},
		},
		{
			name: "PemKeyFile with non-existent file",
			tree: &trillian.Tree{
				PrivateKey: marshalAny(&trillian.PEMKeyFile{
					Path: "non-existent.pem",
				}),
			},
			wantErr: true,
		},
		{
			name: "PemKeyFile with wrong password",
			tree: &trillian.Tree{
				PrivateKey: marshalAny(&trillian.PEMKeyFile{
					Path:     "../../testdata/log-rpc-server.privkey.pem",
					Password: "wrong-password",
				}),
			},
			wantErr: true,
		},
		{
			name: "PemKeyFile with missing password",
			tree: &trillian.Tree{
				PrivateKey: marshalAny(&trillian.PEMKeyFile{
					Path: "../../testdata/log-rpc-server.privkey.pem",
				}),
			},
			wantErr: true,
		},
		{
			name: "Unsupported PrivateKey type",
			tree: &trillian.Tree{
				PrivateKey: marshalAny(&trillian.Tree{}),
			},
			wantErr: true,
		},
		{
			name:    "No PrivateKey",
			tree:    &trillian.Tree{},
			wantErr: true,
		},
	} {
		signer, err := PEMSignerFactory{}.NewSigner(context.Background(), test.tree)
		switch gotErr := err != nil; {
		case gotErr != test.wantErr:
			t.Errorf("%s: Signer(_, %v) = (%v, %v), want err? %t", test.name, test.tree, signer, err, test.wantErr)
			continue
		case gotErr:
			continue
		}

		// Check that the returned signer can produce signatures successfully.
		digest := sha256.Sum256([]byte("test"))
		if _, err := signer.Sign(rand.Reader, digest[:], crypto.SHA256); err != nil {
			t.Errorf("%s: Signer(_, %v).Sign(_, _, _) = (_, %v), want err? false", test.name, test.tree, err)
		}
	}
}
