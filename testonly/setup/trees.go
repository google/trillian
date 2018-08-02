// Copyright 2018 Google Inc. All Rights Reserved.
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

package setup

import (
	"context"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian"
	"github.com/google/trillian/client"
	ktestonly "github.com/google/trillian/crypto/keys/testonly"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/testonly/integration"
)

// CreateLog creats a Trillian log with for testing purposes
//
// It optionally allows specifying an overrides argument to override some
// properties of the tree to be created.
func CreateLog(t *testing.T, logEnv *integration.LogEnv, maxRootDuration time.Duration) *trillian.Tree {
	t.Helper()

	ctx := context.Background()

	privateKey, err := ptypes.MarshalAny(&keyspb.PrivateKey{
		Der: ktestonly.MustMarshalPrivatePEMToDER(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass),
	})
	if err != nil {
		t.Errorf("failed to marshal private key as an any.Any proto: %v", err)
	}

	tree, err := client.CreateAndInitTree(ctx, &trillian.CreateTreeRequest{
		Tree: &trillian.Tree{
			DisplayName:        "Test Log",
			Description:        "This is a test log.",
			TreeType:           trillian.TreeType_LOG,
			TreeState:          trillian.TreeState_ACTIVE,
			HashStrategy:       trillian.HashStrategy_RFC6962_SHA256,
			SignatureAlgorithm: sigpb.DigitallySigned_ECDSA,
			HashAlgorithm:      sigpb.DigitallySigned_SHA256,
			MaxRootDuration:    ptypes.DurationProto(maxRootDuration),

			// Explicitly set the public and private keys for the new tree.
			PrivateKey: privateKey,
			PublicKey: &keyspb.PublicKey{
				Der: ktestonly.MustMarshalPublicPEMToDER(testonly.DemoPublicKey),
			},
		},
	}, logEnv.Admin, nil, logEnv.Log)

	if err != nil {
		t.Errorf("failed to create a new log: %v", err)
	}

	return tree
}
