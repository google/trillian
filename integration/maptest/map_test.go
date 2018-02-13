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

var server = flag.String("map_rpc_server", "", "Server address:port")

func TestMapIntegration(t *testing.T) {

	ctx := context.Background()
	var env *integration.MapEnv
	var err error
	if *server == "" {
		if provider := testdb.Default(); !provider.IsMySQL() {
			t.Skipf("Skipping map integration test, SQL driver is %v", provider.Driver)
		}
		env, err = integration.NewMapEnv(ctx)
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
