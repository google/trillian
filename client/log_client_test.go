// Copyright 2017 Google LLC. All Rights Reserved.
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

	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/google/trillian"
	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/testonly/integration"
	"github.com/google/trillian/types"
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

func clientEnvForTest(ctx context.Context, t *testing.T, template *trillian.Tree) (*integration.LogEnv, *LogClient) {
	t.Helper()
	testdb.SkipIfNoMySQL(t)
	env, err := integration.NewLogEnvWithGRPCOptions(ctx, 1, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := CreateAndInitTree(ctx, &trillian.CreateTreeRequest{Tree: template}, env.Admin, nil, env.Log)
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}

	client, err := NewFromTree(env.Log, tree, types.LogRootV1{})
	if err != nil {
		t.Fatalf("NewFromTree(): %v", err)
	}
	return env, client
}

func TestGetByIndex(t *testing.T) {
	ctx := context.Background()
	env, client := clientEnvForTest(ctx, t, stestonly.PreorderedLogTree)
	defer env.Close()

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
	ctx := context.Background()
	env, client := clientEnvForTest(ctx, t, stestonly.PreorderedLogTree)
	defer env.Close()

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
	ctx := context.Background()
	env, client := clientEnvForTest(ctx, t, stestonly.PreorderedLogTree)
	defer env.Close()

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
	ctx := context.Background()
	env, client := clientEnvForTest(ctx, t, stestonly.PreorderedLogTree)
	defer env.Close()

	// Add a few test leaves.
	leafData := [][]byte{
		[]byte("A"),
		[]byte("B"),
	}

	if err := addSequencedLeaves(ctx, env, client, leafData); err != nil {
		t.Fatalf("Failed to add leaves: %v", err)
	}

	root := client.GetRoot()
	for i, l := range leafData {
		if err := client.GetAndVerifyInclusionAtIndex(ctx, l, int64(i), root); err != nil {
			t.Errorf("VerifyInclusion(%s) = %v, want nil", l, err)
		}
	}

	// Ask for inclusion in a too-large tree.
	root.TreeSize += 1000
	if err := client.GetAndVerifyInclusionAtIndex(ctx, leafData[0], 0, root); err == nil {
		t.Errorf("GetAndVerifyInclusionAtIndex(0, %d)=nil, want error", root.TreeSize)
	}
}

func TestWaitForInclusion(t *testing.T) {
	ctx := context.Background()
	tree := proto.Clone(stestonly.LogTree).(*trillian.Tree)
	env, client := clientEnvForTest(ctx, t, tree)
	tree.TreeId = client.LogID
	defer env.Close()

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
			client: &MutatingLogClient{TrillianLogClient: env.Log, mutateInclusionProof: true}, wantErr: true},
	} {
		t.Run(test.desc, func(t *testing.T) {
			client, err := NewFromTree(test.client, tree, types.LogRootV1{})
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
	ctx := context.Background()
	env, client := clientEnvForTest(ctx, t, stestonly.LogTree)
	defer env.Close()

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

func TestUpdateRootSkew(t *testing.T) {
	ctx := context.Background()
	tree := proto.Clone(stestonly.LogTree).(*trillian.Tree)
	env, client := clientEnvForTest(ctx, t, tree)
	tree.TreeId = client.LogID
	defer env.Close()

	// Start with a single leaf.
	data := []byte("foo")
	if err := client.QueueLeaf(ctx, data); err != nil {
		t.Fatalf("QueueLeaf(%s): %v, want nil", data, err)
	}
	env.Sequencer.OperationSingle(ctx)

	root, err := client.UpdateRoot(ctx)
	if err != nil {
		t.Fatalf("UpdateRoot(): %v", err)
	}

	// Put in a second leaf after root.
	data2 := []byte("bar")
	if err := client.QueueLeaf(ctx, data2); err != nil {
		t.Fatalf("QueueLeaf(%s): %v, want nil", data2, err)
	}
	env.Sequencer.OperationSingle(ctx)

	// Now force a bad request.
	badRawClient := &MutatingLogClient{TrillianLogClient: env.Log, mutateRootSize: true}
	badClient, err := NewFromTree(badRawClient, tree, *root)
	if err != nil {
		t.Fatalf("failed to create mutating client: %v", err)
	}
	if _, err := badClient.UpdateRoot(ctx); err == nil {
		t.Error("UpdateRoot()=nil, want error")
	}
}

func TestAddSequencedLeaves(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		desc        string
		dataByIndex map[int64][]byte
		wantErr     bool
	}{
		{desc: "empty", dataByIndex: nil},
		{desc: "non-contiguous", dataByIndex: map[int64][]byte{
			0: []byte("A"),
			2: []byte("C"),
		}, wantErr: true},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			c := &LogClient{LogVerifier: &LogVerifier{Hasher: rfc6962.DefaultHasher}}
			err := c.AddSequencedLeaves(ctx, tc.dataByIndex)
			if gotErr := err != nil; gotErr != tc.wantErr {
				t.Errorf("AddSequencedLeaves(): %v, wantErr: %v", err, tc.wantErr)
			}
		})
	}
}
