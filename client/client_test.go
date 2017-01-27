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

package client

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/golang/glog"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/testonly/integration"
)

const logID = int64(1234)

func TestAddGetLeaf(t *testing.T) {
	// TODO: Build a GetLeaf method and test a full get/set cycle.
}

func TestAddLeaf(t *testing.T) {
	ctx := context.Background()
	env, err := integration.NewLogEnv("TestAddLeaf")
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	if err := env.CreateLog(logID); err != nil {
		t.Errorf("Failed to create log: %v", err)
	}

	client := New(logID, env.ClientConn, testonly.Hasher)
	client.MaxTries = 1

	if err, want := client.AddLeaf(ctx, []byte("foo")), codes.DeadlineExceeded; grpc.Code(err) != want {
		t.Errorf("AddLeaf(): %v, want, %v", err, want)
	}
	env.Sequencer.OperationLoop() // Sequence the new node.
	glog.Infof("try AddLeaf again")
	if err := client.AddLeaf(ctx, []byte("foo")); err != nil {
		t.Errorf("Failed to add Leaf: %v", err)
	}
}

func TestUpdateSTR(t *testing.T) {
	ctx := context.Background()
	env, err := integration.NewLogEnv("TestUpdateSTR")
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	if err := env.CreateLog(logID); err != nil {
		t.Errorf("Failed to create log: %v", err)
	}
	client := New(logID, env.ClientConn, testonly.Hasher)

	before := client.STR.TreeSize
	if err, want := client.AddLeaf(ctx, []byte("foo")), codes.DeadlineExceeded; grpc.Code(err) != want {
		t.Errorf("AddLeaf(): %v, want, %v", err, want)
	}
	env.Sequencer.OperationLoop() // Sequence the new node.
	if err := client.UpdateSTR(ctx); err != nil {
		t.Error(err)
	}
	if got, want := client.STR.TreeSize, before; got <= want {
		t.Errorf("Tree size after add Leaf: %v, want > %v", got, want)
	}
}
