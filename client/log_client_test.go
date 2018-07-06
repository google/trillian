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
	"github.com/google/trillian/testonly/integration"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/google/trillian/storage/testdb"
	stestonly "github.com/google/trillian/storage/testonly"
)

func TestAddGetLeaf(t *testing.T) {
	// TODO: Build a GetLeaf method and test a full get/set cycle.
}

// addSequencedLeaves is a temporary stand-in function for tests until the real API gets built.
func addSequencedLeaves(ctx context.Context, env *integration.LogEnv, client *LogClient, leaves [][]byte) error {
	if len(leaves) == 0 {
		return nil
	}
	for i, l := range leaves {
		if err := client.AddSequencedLeaf(ctx, l, int64(i)); err != nil {
			return fmt.Errorf("AddSequencedLeaf(): %v", err)
		}
	}
	env.Sequencer.OperationSingle(ctx)
	if err := client.WaitForInclusion(ctx, leaves[len(leaves)-1]); err != nil {
		return fmt.Errorf("WaitForInclusion(): %v", err)
	}
	return nil
}

func TestGetByIndex(t *testing.T) {
	testdb.SkipIfNoMySQL(t)
	ctx := context.Background()
	env, err := integration.NewLogEnv(ctx, 1, "unused")
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()

	tree, err := CreateAndInitTree(ctx,
		&trillian.CreateTreeRequest{Tree: stestonly.PreorderedLogTree},
		env.Admin, nil, env.Log)
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}

	client, err := NewFromTree(env.Log, tree)
	if err != nil {
		t.Fatalf("NewFromTree(): %v", err)
	}
	// Add a few test leaves.
	leafData := [][]byte{
		[]byte("A"),
		[]byte("B"),
		[]byte("C"),
	}

	if err := addSequencedLeaves(ctx, env, client, leafData); err != nil {
		t.Fatalf("Failed to add leaves: %v", err)
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
	testdb.SkipIfNoMySQL(t)
	ctx := context.Background()
	env, err := integration.NewLogEnv(ctx, 1, "unused")
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	tree, err := CreateAndInitTree(ctx,
		&trillian.CreateTreeRequest{Tree: stestonly.PreorderedLogTree},
		env.Admin, nil, env.Log)
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}

	client, err := NewFromTree(env.Log, tree)
	if err != nil {
		t.Fatalf("NewFromTree(): %v", err)
	}
	// Add a few test leaves.
	leafData := [][]byte{
		[]byte("A"),
		[]byte("B"),
		[]byte("C"),
	}

	if err := addSequencedLeaves(ctx, env, client, leafData); err != nil {
		t.Fatalf("Failed to add leaves: %v", err)
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
	testdb.SkipIfNoMySQL(t)
	ctx := context.Background()
	env, err := integration.NewLogEnv(ctx, 1, "unused")
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	tree, err := CreateAndInitTree(ctx,
		&trillian.CreateTreeRequest{Tree: stestonly.PreorderedLogTree},
		env.Admin, nil, env.Log)
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}

	client, err := NewFromTree(env.Log, tree)
	if err != nil {
		t.Fatalf("NewFromTree(): %v", err)
	}
	// Add a few test leaves.
	leafData := [][]byte{
		[]byte("A"),
		[]byte("B"),
	}

	if err := addSequencedLeaves(ctx, env, client, leafData); err != nil {
		t.Fatalf("Failed to add leaves: %v", err)
	}

	for _, l := range leafData {
		if err := client.VerifyInclusion(ctx, l); err != nil {
			t.Errorf("VerifyInclusion(%s) = %v, want nil", l, err)
		}
	}
}

func TestVerifyInclusionAtIndex(t *testing.T) {
	testdb.SkipIfNoMySQL(t)
	ctx := context.Background()
	env, err := integration.NewLogEnv(ctx, 1, "unused")
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	tree, err := CreateAndInitTree(ctx,
		&trillian.CreateTreeRequest{Tree: stestonly.PreorderedLogTree},
		env.Admin, nil, env.Log)
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}

	client, err := NewFromTree(env.Log, tree)
	if err != nil {
		t.Fatalf("NewFromTree(): %v", err)
	}
	// Add a few test leaves.
	leafData := [][]byte{
		[]byte("A"),
		[]byte("B"),
	}

	if err := addSequencedLeaves(ctx, env, client, leafData); err != nil {
		t.Fatalf("Failed to add leaves: %v", err)
	}

	root, err := client.UpdateRoot(ctx)
	if err != nil {
		t.Errorf("UpdateRoot(): %v", err)
	}

	for i, l := range leafData {
		if err := client.GetAndVerifyInclusionAtIndex(ctx, l, int64(i), root); err != nil {
			t.Errorf("VerifyInclusion(%s) = %v, want nil", l, err)
		}
	}
}

func TestWaitForInclusion(t *testing.T) {
	testdb.SkipIfNoMySQL(t)
	ctx := context.Background()
	env, err := integration.NewLogEnv(ctx, 0, "unused")
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	tree, err := CreateAndInitTree(ctx,
		&trillian.CreateTreeRequest{Tree: stestonly.LogTree},
		env.Admin, nil, env.Log)
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}

	for _, test := range []struct {
		desc         string
		leaf         []byte
		client       trillian.TrillianLogClient
		skipPreCheck bool
		wantErr      bool
	}{
		{desc: "First leaf", leaf: []byte("A"), client: env.Log},
		{desc: "Make TreeSize > 1", leaf: []byte("B"), client: env.Log},
		{desc: "invalid inclusion proof", leaf: []byte("A"), skipPreCheck: true,
			client: &MockLogClient{c: env.Log, mGetInclusionProof: true}, wantErr: true},
	} {
		t.Run(test.desc, func(t *testing.T) {
			client, err := NewFromTree(test.client, tree)
			if err != nil {
				t.Fatalf("NewFromTree(): %v", err)
			}

			if !test.skipPreCheck {
				cctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
				if err := client.WaitForInclusion(cctx, test.leaf); status.Code(err) != codes.DeadlineExceeded {
					t.Errorf("WaitForInclusion before sequencing: %v, want: not-nil", err)
				}
				cancel()
			}

			if err := client.QueueLeaf(ctx, test.leaf); err != nil {
				t.Fatalf("QueueLeaf(): %v", err)
			}
			env.Sequencer.OperationSingle(ctx)
			err = client.WaitForInclusion(ctx, test.leaf)
			if got := err != nil; got != test.wantErr {
				t.Errorf("WaitForInclusion(): %v, want error: %v", err, test.wantErr)
			}
		})
	}
}

func TestUpdateRoot(t *testing.T) {
	testdb.SkipIfNoMySQL(t)
	ctx := context.Background()
	env, err := integration.NewLogEnv(ctx, 1, "unused")
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	tree, err := CreateAndInitTree(ctx,
		&trillian.CreateTreeRequest{Tree: stestonly.LogTree},
		env.Admin, nil, env.Log)
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}

	client, err := NewFromTree(env.Log, tree)
	if err != nil {
		t.Fatalf("NewFromTree(): %v", err)
	}

	before := client.root.TreeSize

	// UpdateRoot should succeed with no change.
	root, err := client.UpdateRoot(ctx)
	if err != nil {
		t.Fatalf("UpdateRoot(): %v", err)
	}
	if got, want := root.TreeSize, before; got != want {
		t.Errorf("Tree size changed unexpectedly: %v, want %v", got, want)
	}

	data := []byte("foo")
	if err := client.QueueLeaf(ctx, data); err != nil {
		t.Fatalf("QueueLeaf(%s): %v, want nil", data, err)
	}

	env.Sequencer.OperationSingle(ctx)

	// UpdateRoot should see a change.
	root, err = client.UpdateRoot(ctx)
	if err != nil {
		t.Fatalf("UpdateRoot(): %v", err)
	}
	if got, want := root.TreeSize, before; got <= want {
		t.Errorf("Tree size after add Leaf: %v, want > %v", got, want)
	}
}
