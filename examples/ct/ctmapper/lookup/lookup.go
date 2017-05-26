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

// The lookup binary looks up a specific ID in a map.
package main

import (
	"context"
	"flag"

	"github.com/golang/glog"
	pb "github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/examples/ct/ctmapper"
	"github.com/google/trillian/examples/ct/ctmapper/ctmapperpb"
	"google.golang.org/grpc"
)

var mapServer = flag.String("map_server", "", "host:port for the map server")
var mapID = flag.Int("map_id", -1, "Map ID to write to")

func main() {
	flag.Parse()

	if flag.NArg() == 0 {
		glog.Info("Usage: lookup [domain <domain> ...]")
		return
	}

	conn, err := grpc.Dial(*mapServer, grpc.WithInsecure())
	if err != nil {
		glog.Fatal(err)
	}
	defer conn.Close()

	mapID := int64(*mapID)
	vmap := trillian.NewTrillianMapClient(conn)

	for i := 0; i < flag.NArg(); i++ {
		domain := flag.Arg(i)
		req := &trillian.GetMapLeavesRequest{
			MapId:    mapID,
			Index:    [][]byte{ctmapper.HashDomain(domain)},
			Revision: -1,
		}
		resp, err := vmap.GetLeaves(context.Background(), req)
		if err != nil {
			glog.Warning("Failed to lookup domain %s: %v", domain, err)
			continue
		}
		for _, kv := range resp.MapLeafInclusion {
			el := ctmapperpb.EntryList{}
			v := kv.Leaf.LeafValue
			if len(v) == 0 {
				continue
			}
			if err := pb.Unmarshal(v, &el); err != nil {
				glog.Warning("Failed to unmarshal leaf %s: %v", kv.Leaf.LeafValue, err)
				continue
			}
			glog.Infof("Found %s with certs at indices %v and pre-certs at indices %v", el.Domain, el.CertIndex, el.PrecertIndex)
		}
	}
}
