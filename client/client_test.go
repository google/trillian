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
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/testonly/integration"
)

func TestAddGetLeaf(t *testing.T) {
	// TODO: Build a GetLeaf method and test a full get/set cycle.
}

// addSequencedLeaves is a temporary stand-in function for tests until the real API gets built.
func addSequencedLeaves(ctx context.Context, client VerifyingLogClient, leaves [][]byte) error {
	// TODO(gdbelvin): Replace with batch API.
	// TODO(gdbelvin): Replace with AddSequencedLeaves API.
	for _, l := range leaves {
		if err := client.AddLeaf(ctx, l); err != nil {
			return fmt.Errorf("AddLeaf(%x): %v, want nil", l, err)
		}
	}
	return nil
}

func TestGetByIndex(t *testing.T) {
	ctx := context.Background()
	env, err := integration.NewLogEnv(ctx, 1, "TestGetByIndex")
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	logID, err := env.CreateLog()
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}

	cli := trillian.NewTrillianLogClient(env.ClientConn)
	client := New(logID, cli, testonly.Hasher, env.PublicKey)
	// Add a few test leaves.
	leafData := [][]byte{
		[]byte("A"),
		[]byte("B"),
		[]byte("C"),
	}

	if err := addSequencedLeaves(ctx, client, leafData); err != nil {
		t.Errorf("Failed to add leaves: %v", err)
	}

	for i, l := range leafData {
		leaf, err := client.GetByIndex(ctx, int64(i))
		if err != nil {
			t.Errorf("Failed to GetByIndex(%v): %v", i, err)
			continue
		}
		if got, want := leaf.LeafValue, l; !bytes.Equal(got, want) {
			t.Errorf("GetByIndex(%v) = %x, want %x", i, got, want)
		}
	}
}

func TestListByIndex(t *testing.T) {
	ctx := context.Background()
	env, err := integration.NewLogEnv(ctx, 1, "TestGetByIndex")
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	logID, err := env.CreateLog()
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}

	cli := trillian.NewTrillianLogClient(env.ClientConn)
	client := New(logID, cli, testonly.Hasher, env.PublicKey)
	// Add a few test leaves.
	leafData := [][]byte{
		[]byte("A"),
		[]byte("B"),
		[]byte("C"),
	}

	if err := addSequencedLeaves(ctx, client, leafData); err != nil {
		t.Errorf("Failed to add leaves: %v", err)
	}

	// Fetch leaves.
	leaves, err := client.ListByIndex(ctx, 0, 3)
	if err != nil {
		t.Errorf("Failed to ListByIndex: %v", err)
	}
	for i, l := range leaves {
		if got, want := l.LeafValue, leafData[i]; !bytes.Equal(got, want) {
			t.Errorf("ListIndex()[%v] = %v, want %v", i, got, want)
		}
	}
}

func TestVerifyInclusion(t *testing.T) {
	ctx := context.Background()
	env, err := integration.NewLogEnv(ctx, 1, "TestVerifyInclusion")
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	logID, err := env.CreateLog()
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}

	cli := trillian.NewTrillianLogClient(env.ClientConn)
	client := New(logID, cli, testonly.Hasher, env.PublicKey)
	// Add a few test leaves.
	leafData := [][]byte{
		[]byte("A"),
		[]byte("B"),
	}

	if err := addSequencedLeaves(ctx, client, leafData); err != nil {
		t.Errorf("Failed to add leaves: %v", err)
	}

	for _, l := range leafData {
		if err := client.VerifyInclusion(ctx, l); err != nil {
			t.Errorf("VerifyInclusion(%s) = %v, want nil", l, err)
		}
	}
}

func TestVerifyInclusionAtIndex(t *testing.T) {
	ctx := context.Background()
	env, err := integration.NewLogEnv(ctx, 1, "TestVerifyInclusionAtIndex")
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	logID, err := env.CreateLog()
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}

	cli := trillian.NewTrillianLogClient(env.ClientConn)
	client := New(logID, cli, testonly.Hasher, env.PublicKey)
	// Add a few test leaves.
	leafData := [][]byte{
		[]byte("A"),
		[]byte("B"),
	}

	if err := addSequencedLeaves(ctx, client, leafData); err != nil {
		t.Errorf("Failed to add leaves: %v", err)
	}

	for i, l := range leafData {
		if err := client.VerifyInclusionAtIndex(ctx, l, int64(i)); err != nil {
			t.Errorf("VerifyInclusion(%s) = %v, want nil", l, err)
		}
	}
}

func TestAddLeaf(t *testing.T) {
	ctx := context.Background()
	env, err := integration.NewLogEnv(ctx, 1, "TestAddLeaf")
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
			client: cli,
		},
		{
			desc:   "success 2",
			client: cli,
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
		client := New(logID, test.client, testonly.Hasher, env.PublicKey)
		err := client.AddLeaf(ctx, []byte(test.desc))
		if got := err != nil; got != test.wantErr {
			t.Errorf("AddLeaf(%v): %v, want error: %v", test.desc, err, test.wantErr)
		}
	}
}

func TestUpdateRoot(t *testing.T) {
	ctx := context.Background()
	env, err := integration.NewLogEnv(ctx, 1, "TestUpdateRoot")
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	logID, err := env.CreateLog()
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}
	cli := trillian.NewTrillianLogClient(env.ClientConn)
	client := New(logID, cli, testonly.Hasher, env.PublicKey)

	before := client.Root().TreeSize

	// UpdateRoot should succeed with no change.
	if err := client.UpdateRoot(ctx); err != nil {
		t.Error(err)
	}
	if got, want := client.Root().TreeSize, before; got != want {
		t.Errorf("Tree size changed unexpectedly: %v, want %v", got, want)
	}

	// Add the leaf without polling for inclusion.
	leaf := client.buildLeaf([]byte("foo"))
	if err := client.queueLeaf(ctx, leaf); err != nil {
		t.Fatalf("queueLeaf('foo'): %v, want nil", err)
	}
	// Wait long enough that the leaf should be included.
	time.Sleep(2 * integration.SequencerInterval)

	// UpdateRoot should see a change.
	if err := client.UpdateRoot(ctx); err != nil {
		t.Error(err)
	}
	if got, want := client.Root().TreeSize, before; got <= want {
		t.Errorf("Tree size after add Leaf: %v, want > %v", got, want)
	}
}
