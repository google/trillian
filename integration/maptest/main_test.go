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
	"os"
	"testing"

	"github.com/google/trillian/testonly/integration"

	_ "github.com/google/trillian/merkle/coniks"
	_ "github.com/google/trillian/merkle/maphasher"
)

var (
	server = flag.String("map_rpc_server", "", "Server address:port")
	env    *integration.MapEnv
)

func TestMain(m *testing.M) {
	flag.Parse()

	if *server == "" {
		ctx := context.Background()
		mapEnv, err := integration.NewMapEnv(ctx, "MapIntegrationTestMain")
		if err != nil {
			log.Fatalf("NewMapEnv(): %v", err)
		}
		env = mapEnv
		defer env.Close()
	} else {
		mapEnv, err := integration.NewMapEnvFromConn(*server)
		if err != nil {
			log.Fatalf("failed to get map client: %v", err)
		}
		env = mapEnv
		defer env.Close()
	}

	os.Exit(m.Run())
}
