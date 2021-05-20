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
)

func TestProtoHandler(t *testing.T) {
	ctx := context.Background()

	keyProto := &keyspb.PEMKeyFile{Path: "../../../../testdata/log-rpc-server.privkey.pem", Password: "towel"}

	signer, err := keys.NewSigner(ctx, keyProto)
	if err != nil {
		t.Errorf("keys.NewSigner(_, %#v) = (_, %q), want (_, nil)", keyProto, err)
	}

	// Check that the returned signer can produce signatures successfully.
	if err := testonly.SignAndVerify(signer, signer.Public()); err != nil {
		t.Errorf("SignAndVerify() = %q, want nil", err)
	}
}
