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

package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/google/trillian"
	"google.golang.org/grpc"
)

var (
	treeIDFlag     = flag.Int64("treeid", 3, "The tree id to use")
	serverPortFlag = flag.Int("port", 8090, "Log server port (must be on localhost)")
	startLeafFlag  = flag.Int64("start_leaf", 0, "The first leaf index to fetch")
	numLeavesFlag  = flag.Int64("num_leaves", 1, "The number of leaves to fetch")
)

func buildGetLeavesByIndexRequest(logID int64, startLeaf, numLeaves int64) *trillian.GetLeavesByIndexRequest {
	if startLeaf < 0 || numLeaves <= 0 {
		panic("Start leaf index and num_leaves must be >= 0")
	}

	return &trillian.GetLeavesByIndexRequest{
		LogId:      logID,
		StartIndex: startLeaf,
		PageSize:   numLeaves,
	}
}

// TODO: Move this code out to a better place when we tidy up the initial test main stuff
// It's just a basic skeleton at the moment.
func main() {
	flag.Parse()

	// TODO: Other options apart from insecure connections
	port := *serverPortFlag
	conn, err := grpc.Dial(fmt.Sprintf("localhost:%d", port), grpc.WithInsecure(), grpc.WithTimeout(time.Second*5))

	if err != nil {
		panic(err)
	}

	defer conn.Close()

	client := trillian.NewTrillianLogClient(conn)

	req := buildGetLeavesByIndexRequest(*treeIDFlag, *startLeafFlag, *numLeavesFlag)
	getLeafByIndexResponse, err := client.GetLeavesByIndex(context.Background(), req)

	if err != nil {
		fmt.Printf("Got error in call: %v", err)
	} else {
		fmt.Printf("Got server response: %v", getLeafByIndexResponse)
	}
}
