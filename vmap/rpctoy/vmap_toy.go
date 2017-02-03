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
	"bytes"
	"context"
	"encoding/base64"
	"flag"
	"fmt"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/testonly"
	"google.golang.org/grpc"
)

var server = flag.String("server", "localhost:8091", "Server address:port")

// TODO(al): refactor this and the regular vmap toy to not repeat all this boilerplate.
func main() {
	flag.Parse()

	conn, err := grpc.Dial(*server, grpc.WithInsecure())
	if err != nil {
		glog.Fatal(err)
	}
	defer conn.Close()

	c := trillian.NewTrillianMapClient(conn)

	testVecs := []struct {
		batchSize       int
		numBatches      int
		expectedRootB64 string
	}{
		// roots calculated using python code.
		{1024, 4, "Av30xkERsepT6F/AgbZX3sp91TUmV1TKaXE6QPFfUZA="},
		{10, 4, "6Pk5sprCr3ACfo0OLRZw7sAGdIBTc+7+MxfdW3n76Pc="},
		{6, 4, "QZJ42Te4bw+uGdUaIqzhelxpERU5Ru6uLdy0ixJAuWQ="},
		{4, 4, "9myL1k8Ik6m3Q3JXljHLzfNQHS2d5X6CCbpE/x3mixg="},
		{5, 4, "4xyGOe2DQYi2Qb4aBto9R7jSmiRYqfJ+TcMxUZTXMkM="},
		{6, 3, "FeB/9D+Gzo6oYB2Zi2JMHdrr9KvfvMk7o6DOzjPYG4w="},
		{10, 3, "RfJ6JPERbkDiwlov8/alCqr4yeYYIWl3dWWS3trHsiY="},
		{1, 4, "pQhTahkoXM3WTeAO1o8BYKhgMNzS1yG03vg/fQSVyIc="},
		{2, 4, "RdcEkg5qEuW5eV3VJJLr6uSzvlc27D55AZChG76LHGA="},
		{4, 1, "3dpnVw5Le3HDq/GAkGoSYT9VkzJRV8z18huOk5qMbco="},
		{1024, 1, "7R5uvGy5MJ2Y8xrQr4/mnn3aPw39vYscghmg9KBJaKc="},
		{1, 2, "cZIYiv7ZQ/3rBfpCrha1NKdUnQ8NsTm21WWdV3P4qcU="},
		{1, 3, "KUaQinjLtPQ/ZAek4nHrR7tVXDxLt5QsvZK3vGopDkA="}}

	const testIndex = 0

	batchSize := testVecs[testIndex].batchSize
	numBatches := testVecs[testIndex].numBatches
	expectedRootB64 := testVecs[testIndex].expectedRootB64

	rev := int64(0)
	var root []byte
	for x := 0; x < numBatches; x++ {
		glog.Infof("Starting batch %d...", x)

		req := &trillian.SetMapLeavesRequest{
			MapId:      1,
			IndexValue: make([]*trillian.IndexValue, batchSize),
		}

		for y := 0; y < batchSize; y++ {
			req.IndexValue[y] = &trillian.IndexValue{
				Index: []byte(fmt.Sprintf("key-%d-%d", x, y)),
				Value: &trillian.MapLeaf{
					LeafValue: []byte(fmt.Sprintf("value-%d-%d", x, y)),
				},
			}
		}
		glog.Infof("Created %d k/v pairs...", len(req.IndexValue))

		glog.Info("SetLeaves...")
		resp, err := c.SetLeaves(context.Background(), req)
		if err != nil {
			glog.Fatalf("Failed to write batch %d: %v", x, err)
		}
		glog.Infof("SetLeaves done: %v", resp)
		root = resp.MapRoot.RootHash
		rev++
	}

	if expected, got := testonly.MustDecodeBase64(expectedRootB64), root; !bytes.Equal(expected, root) {
		glog.Fatalf("Expected root %s, got root: %s", base64.StdEncoding.EncodeToString(expected), base64.StdEncoding.EncodeToString(got))
	}
	glog.Infof("Finished, root: %s", base64.StdEncoding.EncodeToString(root))

}
