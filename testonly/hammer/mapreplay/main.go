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

// mapreplay replays a log of Trillian Map requests.
package main

import (
	"context"
	"flag"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/testonly/hammer"
	"google.golang.org/grpc"
)

var (
	mapIDs     = flag.String("map_ids", "", "Comma-separated list of mapID:mapID pairs to convert")
	rpcServer  = flag.String("rpc_server", "", "Server address:port; leave blank for dry-run mode")
	replayFrom = flag.String("replay_from", "", "File to record operations in")
)

func main() {
	flag.Parse()
	defer glog.Flush()
	ctx := context.Background()

	if *replayFrom == "" {
		glog.Exit("Need --replay_from option")
	}
	f, err := os.Open(*replayFrom)
	if err != nil {
		glog.Exitf("Failed to open replay file: %v", err)
	}

	var cl trillian.TrillianMapClient
	var write trillian.TrillianMapWriteClient
	if *rpcServer != "" {
		c, err := grpc.Dial(*rpcServer, grpc.WithInsecure())
		if err != nil {
			glog.Exitf("Failed to create map client conn: %v", err)
		}
		cl = trillian.NewTrillianMapClient(c)
		write = trillian.NewTrillianMapWriteClient(c)
	}

	pairRE := regexp.MustCompile(`(\d+):(\d+)`)
	mapmap := make(map[int64]int64)
	mIDs := strings.Split(*mapIDs, ",")
	for _, pair := range mIDs {
		if pair == "" {
			continue
		}
		results := pairRE.FindStringSubmatch(pair)
		if len(results) < 3 {
			glog.Exitf("Malformed mapID mapping in %q", *mapIDs)
		}
		from, err := strconv.ParseInt(results[1], 10, 64)
		if err != nil {
			glog.Exitf("Malformed mapID mapping in %q", *mapIDs)
		}
		to, err := strconv.ParseInt(results[2], 10, 64)
		if err != nil {
			glog.Exitf("Malformed mapID mapping in %q", *mapIDs)
		}
		mapmap[from] = to
	}
	if err := hammer.ReplayFile(ctx, f, cl, write, mapmap); err != nil {
		glog.Exitf("Error replaying messages: %v", err)
	}
}
