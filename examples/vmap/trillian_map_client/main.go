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

// The trillian_map_client binary performs a trivial map operation.
package main

import (
	"context"
	"flag"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/testonly"
	"google.golang.org/grpc"
)

var (
	server = flag.String("server", "localhost:8091", "Server address:port")
	mapID = flag.Int64("map_id", 1, "Trillian Map ID")
)

func main() {
	flag.Parse()
	defer glog.Flush()

	conn, err := grpc.Dial(*server, grpc.WithInsecure())
	if err != nil {
		glog.Fatal(err)
	}
	defer conn.Close()

	c := trillian.NewTrillianMapClient(conn)

	key := "This Is A Key"
	index := testonly.HashKey(key)

	{
		req := &trillian.SetMapLeavesRequest{
			MapId: *mapID,
			Leaves: []*trillian.MapLeaf{
				{
					Index:     index,
					LeafHash:  []byte("This is a leaf hash"),
					LeafValue: []byte("This is a leaf value"),
					ExtraData: []byte("This is some extra data"),
				},
			},
		}
		resp, err := c.SetLeaves(context.Background(), req)
		if err != nil {
			glog.Error(err)
			return
		}
		glog.Infof("Got SetLeaves response: %+v", resp)
	}

	{
		req := &trillian.GetMapLeavesRequest{
			MapId: *mapID,
			Index: [][]byte{
				index,
			},
		}
		resp, err := c.GetLeaves(context.Background(), req)
		if err != nil {
			glog.Error(err)
		}
		glog.Infof("Got GetLeaves response: %+v", resp)
	}
}
