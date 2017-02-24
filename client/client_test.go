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

	"github.com/google/trillian"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/testonly/integration"
)

func TestAddGetLeaf(t *testing.T) {
	// TODO: Build a GetLeaf method and test a full get/set cycle.
}

func TestAddLeaf(t *testing.T) {
	ctx := context.Background()
	env, err := integration.NewLogEnv(ctx, 0, "TestAddLeaf")
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	logID, err := env.CreateLog()
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}

	cli := trillian.NewTrillianLogClient(env.ClientConn)
	for _, test := range []struct {
		desc    string
		client  trillian.TrillianLogClient
		wantErr bool
	}{
		{
			desc:   "success 1",
			client: &MockLogClient{c: cli},
		},
		{
			desc:   "success 2",
			client: &MockLogClient{c: cli},
		},
		{
			desc:    "invalid inclusion proof",
			client:  &MockLogClient{c: cli, mGetInclusionProof: true},
			wantErr: true,
		},
		{
			desc:    "invalid consistency proof",
			client:  &MockLogClient{c: cli, mGetConsistencyProof: true},
			wantErr: true,
		},
	} {
		client := New(logID, test.client, testonly.Hasher, integration.PublicKey)
		client.MaxTries = 1

		if err, want := client.AddLeaf(ctx, []byte(test.desc)), codes.DeadlineExceeded; grpc.Code(err) != want {
			t.Errorf("AddLeaf(%v): %v, want, %v", test.desc, err, want)
			continue
		}
		env.Sequencer.OperationLoop() // Sequence the new node.
		err := client.AddLeaf(ctx, []byte(test.desc))
		if got := err != nil; got != test.wantErr {
			t.Errorf("AddLeaf(%v): %v, want error: %v", test.desc, err, test.wantErr)
		}
	}
}

func TestUpdateSTR(t *testing.T) {
	ctx := context.Background()
	env, err := integration.NewLogEnv(ctx, 0, "TestUpdateSTR")
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	logID, err := env.CreateLog()
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}
	cli := trillian.NewTrillianLogClient(env.ClientConn)
	client := New(logID, cli, testonly.Hasher, integration.PublicKey)

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
