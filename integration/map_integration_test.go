// Copyright 2016 Google Inc. All Rights Reserved.
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

package integration

import (
	"context"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keyspb"
	stestonly "github.com/google/trillian/storage/testonly"
	"github.com/google/trillian/testonly/integration"
)

var (
	server       = flag.String("map_rpc_server", "localhost:8091", "Server address:port")
	mapID        = flag.Int64("map_id", -1, "Trillian MapID to use for test")
	pubKeyPath   = flag.String("pubkey", "", "Path to public PEM key for map")
	hashStrategy = flag.String("hash_strategy", "TEST_MAP_HASHER", "Hash strategy to use")
)

func newPublicKeyFromFile(keyFile string) (*keyspb.PublicKey, error) {
	pemData, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read: %s. %v", keyFile, err)
	}

	publicBlock, rest := pem.Decode(pemData)
	if publicBlock == nil {
		return nil, errors.New("could not decode PEM for public key")
	}
	if len(rest) > 0 {
		return nil, errors.New("extra data found after PEM key decoded")
	}

	return &keyspb.PublicKey{
		Der: publicBlock.Bytes,
	}, nil
}

// newTreeFromFlags interprets the commandline flags as a trillian.Tree configuration.
func newTreeFromFlags() (*trillian.Tree, error) {
	treeParams := stestonly.MapTree // Use sensible defaults.

	if *mapID == -1 {
		return nil, fmt.Errorf("Map integration test skipped as no map ID provided")
	}
	pubKey, err := newPublicKeyFromFile(*pubKeyPath)
	if err != nil {
		return nil, fmt.Errorf("No public key provided")
	}

	strategy, ok := trillian.HashStrategy_value[*hashStrategy]
	if !ok {
		return nil, fmt.Errorf("Invalid hash strategy %s", *hashStrategy)
	}

	treeParams.TreeId = *mapID
	treeParams.PublicKey = pubKey
	treeParams.HashStrategy = trillian.HashStrategy(strategy)
	return treeParams, nil
}

func TestLiveMapIntegration(t *testing.T) {
	flag.Parse()

	tree, err := newTreeFromFlags()
	if err != nil {
		t.Skip(err)
	}

	env, err := integration.NewMapEnvFromConn(*server)
	if err != nil {
		t.Fatalf("Failed to get map client: %v", err)
	}
	defer env.Close()

	ctx := context.Background()
	if err := RunMapIntegration(ctx, env, tree); err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}
