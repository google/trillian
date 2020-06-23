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

package integration_test

import (
	"context"
	"flag"
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/client"
	"github.com/google/trillian/integration"
	"github.com/google/trillian/storage/testdb"
	tintegration "github.com/google/trillian/testonly/integration"

	_ "github.com/google/trillian/crypto/keys/der/proto" // Register PrivateKey ProtoHandler
	stestonly "github.com/google/trillian/storage/testonly"
)

var singleTX = flag.Bool("single_transaction", false, "Experimental: whether to update the map in a single transaction")

func TestInProcessMapIntegration(t *testing.T) {
	testdb.SkipIfNoMySQL(t)
	ctx := context.Background()
	env, err := tintegration.NewMapEnv(ctx, *singleTX)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()

	tree, err := client.CreateAndInitTree(ctx, &trillian.CreateTreeRequest{Tree: stestonly.MapTree}, env.Admin, env.Map, nil)
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}
	info, err := integration.New(env.Map, env.Write, tree)
	if err != nil {
		t.Fatalf("Failed to create MapInfo for test: %v", err)
	}

	if err := info.RunIntegration(ctx); err != nil {
		t.Fatalf("[%d] map integration test failed: %v", tree.TreeId, err)
	}
}
