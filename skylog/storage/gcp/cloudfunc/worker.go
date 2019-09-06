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

// Package cloudfunc wraps Merkle tree build worker into a Cloud Function.
package cloudfunc

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian/merkle/compact"
	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/skylog/core"
	cs "github.com/google/trillian/skylog/storage/gcp/cloudspanner"
	pb "github.com/google/trillian/skylog/storage/gcp/gcppb"
)

const spannerEnvVar = "SPANNER_DB"

var (
	hasher  = rfc6962.DefaultHasher
	factory = &compact.RangeFactory{Hash: hasher.HashChildren}

	client *spanner.Client
)

// BuildMessage is the payload of a Merkle tree building Cloud Pub/Sub event.
type BuildMessage struct {
	// Data contains an encoded BuildJob proto message.
	Data []byte `json:"data"`
}

// BuildSubtree consumes a builder job message.
func BuildSubtree(ctx context.Context, msg BuildMessage) error {
	client, err := spannerClient(ctx)
	if err != nil {
		return fmt.Errorf("connecting to Spanner failed: %w", err)
	}

	var job pb.BuildJob
	if err := proto.Unmarshal(msg.Data, &job); err != nil {
		return err
	}
	log.Printf("Accepted job: %s", proto.CompactTextString(&job))
	if job.End <= job.Begin {
		return errors.New("invalid job: end <= begin")
	}

	// TODO(pavelkalinnikov): Read hashes from storage.
	hashes := make([][]byte, 0, job.End-job.Begin)
	for i := job.Begin; i < job.End; i++ {
		data := []byte(fmt.Sprintf("data:%d", i))
		hash := hasher.HashLeaf(data)
		hashes = append(hashes, hash)
	}
	cJob := core.BuildJob{RangeStart: job.Begin, Hashes: hashes}

	opts, err := treeOpts(&job)
	if err != nil {
		return err
	}
	ts := cs.NewTreeStorage(client, job.TreeId, opts)
	bw := core.NewBuildWorker(ts, factory)

	_, err = bw.Process(ctx, cJob)
	return err
}

func treeOpts(job *pb.BuildJob) (cs.TreeOpts, error) {
	ts := job.GetTreeSharding()
	if ts == nil {
		return cs.TreeOpts{}, errors.New("missing tree sharding info")
	} else if ts.Levels <= 0 || ts.Shards <= 0 {
		return cs.TreeOpts{}, fmt.Errorf("invalid tree sharding: %s", proto.CompactTextString(ts))
	}
	return cs.TreeOpts{ShardLevels: uint(ts.Levels), LeafShards: int64(ts.Shards)}, nil
}

// spannerClient creates a Could Spanner client, or returns the cached one.
func spannerClient(ctx context.Context) (*spanner.Client, error) {
	if client != nil {
		return client, nil
	}

	db, ok := os.LookupEnv(spannerEnvVar)
	if !ok {
		return nil, fmt.Errorf("env variable %s not found", spannerEnvVar)
	}

	var err error
	if client, err = spanner.NewClient(ctx, db); err != nil {
		return nil, fmt.Errorf("spanner.NewClient: %w", err)
	}
	log.Printf("Connected to Cloud Spanner: %s", db)
	return client, err
}
