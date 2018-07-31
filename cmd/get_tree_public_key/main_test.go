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

package main

import (
	"context"
	"flag"
	"fmt"
	"strings"
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
	"github.com/google/trillian/util/flagsaver"
	"google.golang.org/grpc"
)

func TestGetTreePublicKey(t *testing.T) {
	// Set up Trillian servers
	const numSequencers = 0 // we don't actually need any sequencers.
	serverOpts := []grpc.ServerOption{}
	clientOpts := []grpc.DialOption{grpc.WithInsecure()}
	logEnv, err := integration.NewLogEnvWithGRPCOptions(context.Background(), numSequencers, serverOpts, clientOpts)
	if err != nil {
		t.Fatal(err)
	}
	defer logEnv.Close()

	// Create a new Trillian log
	log := createLog(t, logEnv)

	// Set the flags.
	defer flagsaver.Save().Restore()
	setFlag(t, "admin_server", logEnv.Address)
	setFlag(t, "log_id", fmt.Sprint(log.TreeId))

	publicKeyPEM, err := getPublicKeyPEM()
	if err != nil {
		t.Errorf("Got an unexpected error: %v", err)
	}

	// Check that the returned public key PEM is the one we expected.
	expectedPublicKeyPEM := strings.TrimSpace(testonly.DemoPublicKey)
	if strings.TrimSpace(publicKeyPEM) != expectedPublicKeyPEM {
		t.Errorf("Expected the public key PEM to equal:\n%s\nInstead got:\n%s", expectedPublicKeyPEM, publicKeyPEM)
	}
}

func setFlag(t *testing.T, name, value string) {
	t.Helper()

	if err := flag.Set(name, value); err != nil {
		t.Errorf("failed to set the -%s flag: %v", name, err)
	}
}

func createLog(t *testing.T, logEnv *integration.LogEnv) *trillian.Tree {
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
			MaxRootDuration:    ptypes.DurationProto(0 * time.Millisecond),

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
