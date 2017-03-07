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
	"flag"
	"testing"

	"github.com/google/trillian"
	"google.golang.org/grpc"
)

var server = flag.String("map_rpc_server", "localhost:8091", "Server address:port")
var mapID = flag.Int64("map_id", -1, "Trillian MapID to use for test")

func getClient() (*grpc.ClientConn, trillian.TrillianMapClient, error) {
	conn, err := grpc.Dial(*server, grpc.WithInsecure())
	if err != nil {
		return nil, nil, err
	}
	return conn, trillian.NewTrillianMapClient(conn), nil
}

func TestLiveMapIntegration(t *testing.T) {
	flag.Parse()
	if *mapID == -1 {
		t.Skip("Map integration test skipped as no map ID provided")
	}

	conn, client, err := getClient()
	if err != nil {
		t.Fatalf("Failed to get map client: %v", err)
	}
	defer conn.Close()
	ctx := context.Background()
	if err := RunMapIntegration(ctx, *mapID, client); err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}
