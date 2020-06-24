// Copyright 2018 Google LLC. All Rights Reserved.
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
	"fmt"
	"strings"
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/client"
	"github.com/google/trillian/testonly/flagsaver"
	"github.com/google/trillian/testonly/integration"
	"github.com/google/trillian/testonly/setup"
	"google.golang.org/grpc"

	stestonly "github.com/google/trillian/storage/testonly"
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
	log, err := client.CreateAndInitTree(context.Background(), &trillian.CreateTreeRequest{
		Tree: stestonly.LogTree,
	}, logEnv.Admin, nil, logEnv.Log)
	if err != nil {
		t.Errorf("Failed to create test tree: %v", err)
	}

	// Set the flags.
	defer flagsaver.Save().MustRestore()
	setup.SetFlag(t, "admin_server", logEnv.Address)
	setup.SetFlag(t, "log_id", fmt.Sprint(log.TreeId))

	publicKeyPEM, err := getPublicKeyPEM()
	if err != nil {
		t.Errorf("Got an unexpected error: %v", err)
	}

	// Check that the returned public key PEM is the one we expected.
	expectedPublicKeyPEM := strings.TrimSpace(stestonly.PublicKeyPEM)
	if strings.TrimSpace(publicKeyPEM) != expectedPublicKeyPEM {
		t.Errorf("Expected the public key PEM to equal:\n%s\nInstead got:\n%s", expectedPublicKeyPEM, publicKeyPEM)
	}
}
