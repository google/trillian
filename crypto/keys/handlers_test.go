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

package keys

import (
	"context"
	"crypto"
	"errors"
	"testing"

	"github.com/google/trillian/crypto/keys/pem"
	"github.com/google/trillian/testonly"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

func fakeHandler(signer crypto.Signer, err error) ProtoHandler {
	return func(ctx context.Context, pb proto.Message) (crypto.Signer, error) {
		return signer, err
	}
}

func TestNewSigner(t *testing.T) {
	wantSigner, err := pem.UnmarshalPrivateKey(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass)
	if err != nil {
		t.Fatalf("Error unmarshaling test private key: %v", err)
	}

	ctx := context.Background()

	for _, test := range []struct {
		desc     string
		keyProto proto.Message
		handler  ProtoHandler
		wantErr  bool
	}{
		{
			desc:     "KeyProto with handler",
			keyProto: &emptypb.Empty{},
			handler:  fakeHandler(wantSigner, nil),
		},
		{
			desc:     "Invalid KeyProto with handler",
			keyProto: &emptypb.Empty{},
			handler:  fakeHandler(nil, errors.New("invalid KeyProto")),
			wantErr:  true,
		},
		{
			desc:     "KeyProto with no handler",
			keyProto: &emptypb.Empty{},
			wantErr:  true,
		},
		{
			desc:    "Nil KeyProto",
			wantErr: true,
		},
	} {
		if test.handler != nil {
			RegisterHandler(test.keyProto, test.handler)
		}

		gotSigner, err := NewSigner(ctx, test.keyProto)
		switch gotErr := err != nil; {
		case gotErr != test.wantErr:
			t.Errorf("%v: NewSigner() = (_, %q), want err? %v", test.desc, err, test.wantErr)
		case !gotErr && gotSigner != wantSigner:
			t.Errorf("%v: NewSigner() = (%#v, _), want (%#v, _)", test.desc, gotSigner, wantSigner)
		}

		if test.handler != nil {
			UnregisterHandler(test.keyProto)
		}
	}
}
