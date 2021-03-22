// Copyright 2016 Google LLC. All Rights Reserved.
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

// The log_client binary retrieves leaves from a log.
package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"google.golang.org/grpc"
)

var (
	treeIDFlag     = flag.Int64("treeid", 3, "The tree id to use")
	serverPortFlag = flag.Int("port", 8090, "Log server port (must be on localhost)")
	startLeafFlag  = flag.Int64("start_leaf", 0, "The first leaf index to fetch")
	numLeavesFlag  = flag.Int64("num_leaves", 1, "The number of leaves to fetch")
)

// TODO: Move this code out to a better place when we tidy up the initial test main stuff
// It's just a basic skeleton at the moment.
func main() {
	flag.Parse()
	defer glog.Flush()

	ctx := context.Background()

	// TODO: Other options apart from insecure connections
	port := *serverPortFlag
	dialCtx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()
	conn, err := grpc.DialContext(dialCtx, fmt.Sprintf("localhost:%d", port), grpc.WithInsecure())
	if err != nil {
		panic(err)
	}

	defer conn.Close()

	client := trillian.NewTrillianLogClient(conn)

	req := &trillian.GetLeavesByRangeRequest{LogId: *treeIDFlag, StartIndex: *startLeafFlag, Count: *numLeavesFlag}
	getLeafByRangeResponse, err := client.GetLeavesByRange(ctx, req)

	if err != nil {
		fmt.Printf("Got error in call: %v", err)
	} else {
		fmt.Printf("Got server response: %v", getLeafByRangeResponse)
	}
}
