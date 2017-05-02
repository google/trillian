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
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/google/trillian/crypto/sigpb"
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
				PrivateKey: marshalAny(&keyspb.PEMKeyFile{
					Path:     "../../testdata/log-rpc-server.privkey.pem",
					Password: "towel",
				}),
			},
		},
		{
			name: "PemKeyFile with non-existent file",
			tree: &trillian.Tree{
				PrivateKey: marshalAny(&keyspb.PEMKeyFile{
					Path: "non-existent.pem",
				}),
			},
			wantErr: true,
		},
		{
			name: "PemKeyFile with wrong password",
			tree: &trillian.Tree{
				PrivateKey: marshalAny(&keyspb.PEMKeyFile{
					Path:     "../../testdata/log-rpc-server.privkey.pem",
					Password: "wrong-password",
				}),
			},
			wantErr: true,
		},
		{
			name: "PemKeyFile with missing password",
			tree: &trillian.Tree{
				PrivateKey: marshalAny(&keyspb.PEMKeyFile{
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
			t.Errorf("%v: Signer(_, %v) = (%v, %v), want err? %v", test.name, test.tree, signer, err, test.wantErr)
			continue
		case gotErr:
			continue
		}

		// Check that the returned signer can produce signatures successfully.
		digest := sha256.Sum256([]byte("test"))
		if _, err := signer.Sign(rand.Reader, digest[:], crypto.SHA256); err != nil {
			t.Errorf("%v: Signer(_, %v).Sign() = (_, %v), want err? false", test.name, test.tree, err)
		}
	}
}

func TestPEMSignerFactoryGenerate(t *testing.T) {
	for _, test := range []struct {
		name    string
		keySpec *keyspb.Specification
		tree    *trillian.Tree
		wantErr bool
	}{
		{
			name: "RSA",
			keySpec: &keyspb.Specification{
				Params: &keyspb.Specification_RsaParams{
					RsaParams: &keyspb.Specification_RSA{},
				},
			},
			tree: &trillian.Tree{
				SignatureAlgorithm: sigpb.DigitallySigned_RSA,
			},
		},
		{
			name: "ECDSA",
			keySpec: &keyspb.Specification{
				Params: &keyspb.Specification_EcdsaParams{
					EcdsaParams: &keyspb.Specification_ECDSA{},
				},
			},
			tree: &trillian.Tree{
				SignatureAlgorithm: sigpb.DigitallySigned_ECDSA,
			},
		},
		{
			name: "Tree already has a PrivateKey",
			keySpec: &keyspb.Specification{
				Params: &keyspb.Specification_EcdsaParams{
					EcdsaParams: &keyspb.Specification_ECDSA{},
				},
			},
			tree: &trillian.Tree{
				PrivateKey: marshalAny(&keyspb.PEMKeyFile{
					Path: "non-existent.pem",
				}),
				PublicKey: &keyspb.PublicKey{
					Der: []byte("foo"),
				},
				SignatureAlgorithm: sigpb.DigitallySigned_ECDSA,
			},
			wantErr: true,
		},
		{
			name: "Mismatched Specification.Params and tree.SignatureAlgorithm",
			keySpec: &keyspb.Specification{
				Params: &keyspb.Specification_EcdsaParams{
					EcdsaParams: &keyspb.Specification_ECDSA{},
				},
			},
			tree: &trillian.Tree{
				SignatureAlgorithm: sigpb.DigitallySigned_RSA,
			},
			wantErr: true,
		},
		{
			name: "RSA with insufficient key size",
			keySpec: &keyspb.Specification{
				Params: &keyspb.Specification_RsaParams{
					RsaParams: &keyspb.Specification_RSA{
						Bits: 1024,
					},
				},
			},
			tree: &trillian.Tree{
				SignatureAlgorithm: sigpb.DigitallySigned_RSA,
			},
			wantErr: true,
		},
	} {
		ctx := context.Background()
		var sf PEMSignerFactory

		key, err := sf.Generate(ctx, test.tree, test.keySpec)
		if gotErr := err != nil; gotErr != test.wantErr {
			t.Errorf("%v: Generate() = (_, %v), want err? %v", test.name, err, test.wantErr)
			continue
		} else if gotErr {
			continue
		}

		newTree := *test.tree
		newTree.PrivateKey = key

		signer, err := sf.NewSigner(ctx, &newTree)
		if err != nil {
			t.Errorf("%v: NewSigner(_, %v) = (_, %v), want (_, nil)", test.name, newTree, err)
			continue
		}

		publicKey := signer.Public()
		switch test.keySpec.Params.(type) {
		case *keyspb.Specification_RsaParams:
			if _, ok := publicKey.(*rsa.PublicKey); !ok {
				t.Errorf("%v: signer.Public() = %T, want *rsa.PublicKey", test.name, publicKey)
			}
		case *keyspb.Specification_EcdsaParams:
			if _, ok := publicKey.(*ecdsa.PublicKey); !ok {
				t.Errorf("%v: signer.Public() = %T, want *ecdsa.PublicKey", test.name, publicKey)
			}
		}
	}
}
