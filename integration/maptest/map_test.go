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

package maptest

import (
	"context"
	"flag"
	"log"
	"testing"

	"github.com/google/trillian/storage/testdb"
	"github.com/google/trillian/testonly/integration"

	_ "github.com/google/trillian/merkle/coniks"
	_ "github.com/google/trillian/merkle/maphasher"
)

var (
	server        = flag.String("map_rpc_server", "", "Server address:port")
	singleTX      = flag.Bool("single_transaction", true, "Experimental: whether to update the map in a single transaction")
	stress        = flag.Bool("enable_stress", false, "Enable large stress tests")
	stressBatches = flag.Int("stress_num_batches", 1, "Number of batches to write in MapWriteStress test")
)

func TestMapIntegration(t *testing.T) {
	ctx := context.Background()
	var env *integration.MapEnv
	var err error
	if *server == "" {
		if !testdb.MySQLAvailable() {
			t.Skip("Skipping map integration test, MySQL not available")
		}
		env, err = integration.NewMapEnv(ctx, *singleTX)
	} else {
		env, err = integration.NewMapEnvFromConn(*server)
	}
	if err != nil {
		log.Fatalf("Could not create MapEnv: %v", err)
	}
	defer env.Close()

	for _, test := range AllTests {
		t.Run(test.Name, func(t *testing.T) {
			test.Fn(ctx, t, env.Admin, env.Map)
		})
	}
}

func TestMapWriteStress(t *testing.T) {
	if !*stress {
		t.Skip("Skipped by default, enable me if with --enable_stress you know what you're doing")
	}

	ctx := context.Background()
	var env *integration.MapEnv
	var err error
	if *server == "" {
		if !testdb.MySQLAvailable() {
			t.Skip("Skipping map integration test, MySQL not available")
		}
		env, err = integration.NewMapEnv(ctx, *singleTX)
	} else {
		env, err = integration.NewMapEnvFromConn(*server)
	}
	if err != nil {
		log.Fatalf("Could not create MapEnv: %v", err)
	}
	defer env.Close()

	RunWriteBatchStress(ctx, t, env.Admin, env.Map, 512, *stressBatches)
}
