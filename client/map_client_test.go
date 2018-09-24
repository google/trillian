// Copyright 2018 Google Inc. All Rights Reserved.
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
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/storage/testdb"
	"github.com/google/trillian/storage/testonly"
	"github.com/google/trillian/testonly/integration"
)

func TestGetLatestMapRoot(t *testing.T) {
	testdb.SkipIfNoMySQL(t)
	ctx := context.Background()
	env, err := integration.NewMapEnv(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	tree, err := CreateAndInitTree(ctx,
		&trillian.CreateTreeRequest{Tree: testonly.MapTree},
		env.Admin, env.Map, nil)
	if err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}

	client, err := NewMapClientFromTree(env.Map, tree)
	if err != nil {
		t.Fatalf("NewMapClientFromTree(): %v", err)
	}

	root, err := client.GetAndVerifyLatestMapRoot(ctx)
	if err != nil {
		t.Fatalf("GetAndVerifyLatestMapRoot(): %v", err)
	}
	if got, want := root.Revision, uint64(0); got != want {
		t.Errorf("root.Revision: %v, want %v", got, want)
	}
}

func TestGetLeavesAtRevision(t *testing.T) {
	testdb.SkipIfNoMySQL(t)
	ctx := context.Background()
	env, err := integration.NewMapEnv(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	tree, err := CreateAndInitTree(ctx,
		&trillian.CreateTreeRequest{Tree: testonly.MapTree},
		env.Admin, env.Map, nil)
	if err != nil {
		t.Fatalf("Failed to create map: %v", err)
	}

	client, err := NewMapClientFromTree(env.Map, tree)
	if err != nil {
		t.Fatalf("NewMapClientFromTree(): %v", err)
	}

	index := []byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	if _, err := env.Map.SetLeaves(ctx, &trillian.SetMapLeavesRequest{
		MapId: client.MapID,
		Leaves: []*trillian.MapLeaf{
			{
				Index:     index,
				LeafValue: []byte("A"),
			},
		},
	}); err != nil {
		t.Fatalf("SetLeaves(): %v", err)
	}
	leaves, err := client.GetAndVerifyMapLeaves(ctx, [][]byte{index})
	if err != nil {
		t.Fatalf("GetAndVerifyMapLeavesAtRevision(): %v", err)
	}
	if got := len(leaves); got != 1 {
		t.Errorf("len(leaves): %v, want 1", got)
	}
	if got, want := leaves[0].LeafValue, []byte("A"); !bytes.Equal(got, want) {
		t.Errorf("LeafValue: %v, want %v", got, want)
	}
	if got, want := leaves[0].Index, index; !bytes.Equal(got, want) {
		t.Errorf("LeafIndex: %v, want %v", got, want)
	}
}
