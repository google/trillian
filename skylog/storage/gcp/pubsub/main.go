// Copyright 2019 Google Inc. All Rights Reserved.
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

// Package contains a binary that sends Merkle tree building jobs to a
// collection of workers through a GCP Pub/Sub topic.
package main

import (
	"context"
	"flag"

	"cloud.google.com/go/pubsub"
	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	pb "github.com/google/trillian/skylog/storage/gcp/gcppb"
	"golang.org/x/time/rate"
)

var (
	projectID   = flag.String("project", "skylog-test", "The GCP project ID")
	psTopic     = flag.String("topic", "build-jobs", "The Pub/Sub topic for build jobs")
	treeID      = flag.Int64("tree_id", 1, "The ID of the tree under construction")
	beginIndex  = flag.Int64("begin", 0, "The beginning of the tree building range")
	endIndex    = flag.Int64("end", 0, "The ending of the tree building range")
	maxJobSize  = flag.Int("job_size", 256, "The maximal number of entries in a build job")
	maxRate     = flag.Float64("rate", 100000, "The average rate of adding entries per second")
	shardLevels = flag.Int("shard_levels", 10, "The number of tree levels in a shard")
	treeShards  = flag.Int("tree_shards", 16, "The number of shards in the tree storage")
)

func runSender(ctx context.Context, cli *pubsub.Client) error {
	top := cli.Topic(*psTopic)
	defer top.Stop()

	var results []*pubsub.PublishResult

	sharding := &pb.TreeSharding{Levels: uint32(*shardLevels), Shards: uint32(*treeShards)}

	lim := rate.NewLimiter(rate.Limit(*maxRate), *maxJobSize)
	for index, next := *beginIndex, int64(0); index < *endIndex; index = next {
		next = index + int64(*maxJobSize)
		if next > *endIndex {
			next = *endIndex
		}
		job := pb.BuildJob{TreeId: *treeID, Begin: uint64(index), End: uint64(next), TreeSharding: sharding}
		msg, err := proto.Marshal(&job)
		if err != nil {
			return err
		}
		if err := lim.WaitN(ctx, int(job.End-job.Begin)); err != nil {
			return err
		}
		res := top.Publish(ctx, &pubsub.Message{Data: msg})
		results = append(results, res)
	}

	for _, res := range results {
		if _, err := res.Get(ctx); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	flag.Parse()
	ctx := context.Background()
	cli, err := pubsub.NewClient(ctx, *projectID)
	if err != nil {
		glog.Exitf("pubsub.NewClient: %v", err)
	}
	defer cli.Close()

	if err := runSender(ctx, cli); err != nil {
		glog.Exitf("runSender: %v", err)
	}
}
