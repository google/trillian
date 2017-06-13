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

package admin

import (
	"context"
	"net"
	"sort"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	sa "github.com/google/trillian/server/admin"
	"github.com/google/trillian/server/interceptor"
	"github.com/google/trillian/storage/testonly"
	"github.com/google/trillian/testonly/integration"
	"github.com/kylelemons/godebug/pretty"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestAdminServer_Unimplemented(t *testing.T) {
	client, closeFn, err := setupAdminServer()
	if err != nil {
		t.Fatalf("setupAdminServer() failed: %v", err)
	}
	defer closeFn()

	ctx := context.Background()
	tree, err := client.CreateTree(ctx, &trillian.CreateTreeRequest{Tree: testonly.LogTree})
	if err != nil {
		t.Fatalf("CreateTree returned err = %v", err)
	}

	tests := []struct {
		desc string
		fn   func(context.Context, trillian.TrillianAdminClient) error
	}{
		{
			desc: "DeleteTree",
			fn: func(ctx context.Context, c trillian.TrillianAdminClient) error {
				_, err := c.DeleteTree(ctx, &trillian.DeleteTreeRequest{TreeId: tree.TreeId})
				return err
			},
		},
	}
	for _, test := range tests {
		err := test.fn(ctx, client)
		if s, ok := status.FromError(err); !ok || s.Code() != codes.Unimplemented {
			t.Errorf("%v: got = %v, want = %s", test.desc, err, codes.Unimplemented)
		}
	}
}

func TestAdminServer_CreateTree(t *testing.T) {
	client, closeFn, err := setupAdminServer()
	if err != nil {
		t.Fatalf("setupAdminServer() failed: %v", err)
	}
	defer closeFn()

	invalidTree := *testonly.LogTree
	invalidTree.TreeState = trillian.TreeState_HARD_DELETED

	tests := []struct {
		desc     string
		req      *trillian.CreateTreeRequest
		wantCode codes.Code
	}{
		{
			desc: "validTree",
			req: &trillian.CreateTreeRequest{
				Tree: testonly.LogTree,
			},
		},
		{
			desc:     "nilTree",
			req:      &trillian.CreateTreeRequest{},
			wantCode: codes.InvalidArgument,
		},
		{
			desc: "invalidTree",
			req: &trillian.CreateTreeRequest{
				Tree: &invalidTree,
			},
			wantCode: codes.InvalidArgument,
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		createdTree, err := client.CreateTree(ctx, test.req)
		if s, ok := status.FromError(err); !ok || s.Code() != test.wantCode {
			t.Errorf("%v: CreateTree() = (_, %v), wantCode = %v", test.desc, err, test.wantCode)
			continue
		} else if err != nil {
			continue
		}

		storedTree, err := client.GetTree(ctx, &trillian.GetTreeRequest{TreeId: createdTree.TreeId})
		if err != nil {
			t.Errorf("%v: GetTree() = (_, %v), want = (_, nil)", test.desc, err)
			continue
		}
		if diff := pretty.Compare(storedTree, createdTree); diff != "" {
			t.Errorf("%v: post-CreateTree diff (-stored +created):\n%v", test.desc, diff)
		}
	}
}

func TestAdminServer_UpdateTree(t *testing.T) {
	client, closeFn, err := setupAdminServer()
	if err != nil {
		t.Fatalf("setupAdminServer() failed: %v", err)
	}
	defer closeFn()

	baseTree := *testonly.LogTree

	// successTree specifies changes in all rw fields
	successTree := &trillian.Tree{
		TreeState:   trillian.TreeState_FROZEN,
		DisplayName: "Brand New Tree Name",
		Description: "Brand New Tree Desc",
	}
	successMask := &field_mask.FieldMask{Paths: []string{"tree_state", "display_name", "description"}}

	successWant := baseTree
	successWant.TreeState = successTree.TreeState
	successWant.DisplayName = successTree.DisplayName
	successWant.Description = successTree.Description
	successWant.PrivateKey = nil // redacted on responses

	tests := []struct {
		desc                 string
		createTree, wantTree *trillian.Tree
		req                  *trillian.UpdateTreeRequest
		wantCode             codes.Code
	}{
		{
			desc:       "success",
			createTree: &baseTree,
			wantTree:   &successWant,
			req:        &trillian.UpdateTreeRequest{Tree: successTree, UpdateMask: successMask},
		},
		{
			desc: "notFound",
			req: &trillian.UpdateTreeRequest{
				Tree:       &trillian.Tree{TreeId: 12345, DisplayName: "New Name"},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"display_name"}},
			},
			wantCode: codes.NotFound,
		},
		{
			desc:       "readonlyField",
			createTree: &baseTree,
			req: &trillian.UpdateTreeRequest{
				Tree:       successTree,
				UpdateMask: &field_mask.FieldMask{Paths: []string{"tree_type"}},
			},
			wantCode: codes.InvalidArgument,
		},
		{
			desc:       "invalidUpdate",
			createTree: &baseTree,
			req: &trillian.UpdateTreeRequest{
				Tree:       &trillian.Tree{}, // tree_state = UNKNOWN_TREE_STATE
				UpdateMask: &field_mask.FieldMask{Paths: []string{"tree_state"}},
			},
			wantCode: codes.InvalidArgument,
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		if test.createTree != nil {
			tree, err := client.CreateTree(ctx, &trillian.CreateTreeRequest{Tree: test.createTree})
			if err != nil {
				t.Errorf("%v: CreateTree() returned err = %v", test.desc, err)
				continue
			}
			test.req.Tree.TreeId = tree.TreeId
		}

		tree, err := client.UpdateTree(ctx, test.req)
		if s, ok := status.FromError(err); !ok || s.Code() != test.wantCode {
			t.Errorf("%v: UpdateTree() returned err = %q, wantCode = %v", test.desc, err, test.wantCode)
			continue
		} else if err != nil {
			continue
		}

		if tree.CreateTimeMillisSinceEpoch > tree.UpdateTimeMillisSinceEpoch {
			t.Errorf("%v: CreateTime > UpdateTime (%v > %v)", test.desc, tree.CreateTimeMillisSinceEpoch, tree.UpdateTimeMillisSinceEpoch)
		}

		// Copy storage-generated fields to the expected tree
		want := *test.wantTree
		want.TreeId = tree.TreeId
		want.CreateTimeMillisSinceEpoch = tree.CreateTimeMillisSinceEpoch
		want.UpdateTimeMillisSinceEpoch = tree.UpdateTimeMillisSinceEpoch
		if !proto.Equal(tree, &want) {
			diff := pretty.Compare(tree, &want)
			t.Errorf("%v: post-UpdateTree diff:\n%v", test.desc, diff)
		}
	}
}

func TestAdminServer_GetTree(t *testing.T) {
	client, closeFn, err := setupAdminServer()
	if err != nil {
		t.Fatalf("setupAdminServer() failed: %v", err)
	}
	defer closeFn()

	tests := []struct {
		desc     string
		treeID   int64
		wantCode codes.Code
	}{
		{
			desc:     "negativeTreeID",
			treeID:   -1,
			wantCode: codes.NotFound,
		},
		{
			desc:     "notFound",
			treeID:   12345,
			wantCode: codes.NotFound,
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		_, err := client.GetTree(ctx, &trillian.GetTreeRequest{TreeId: test.treeID})
		if s, ok := status.FromError(err); !ok || s.Code() != test.wantCode {
			t.Errorf("%v: GetTree() = (_, %v), wantCode = %v", test.desc, err, test.wantCode)
		}
		// Success of GetTree is part of TestAdminServer_CreateTree, so it's not asserted here.
	}
}

func TestAdminServer_ListTrees(t *testing.T) {
	client, closeFn, err := setupAdminServer()
	if err != nil {
		t.Fatalf("setupAdminServer() failed: %v", err)
	}
	defer closeFn()

	tests := []struct {
		desc string
		// numTrees is the number of trees in storage. New trees are created as necessary
		// and carried over to following tests.
		numTrees int
	}{
		{desc: "empty"},
		{desc: "oneTree", numTrees: 1},
		{desc: "threeTrees", numTrees: 3},
	}

	ctx := context.Background()
	createdTrees := []*trillian.Tree{}
	for _, test := range tests {
		if l := len(createdTrees); l > test.numTrees {
			t.Fatalf("%v: numTrees = %v, but we already have %v stored trees", test.desc, test.numTrees, l)
		} else if l < test.numTrees {
			for i := l; i < test.numTrees; i++ {
				var tree *trillian.Tree
				if i%2 == 0 {
					tree = testonly.LogTree
				} else {
					tree = testonly.MapTree
				}
				req := &trillian.CreateTreeRequest{Tree: tree}
				resp, err := client.CreateTree(ctx, req)
				if err != nil {
					t.Fatalf("%v: CreateTree(_, %v) = (_, %q), want = (_, nil)", test.desc, req, err)
				}
				createdTrees = append(createdTrees, resp)
			}
			sortByTreeID(createdTrees)
		}

		resp, err := client.ListTrees(ctx, &trillian.ListTreesRequest{})
		if err != nil {
			t.Errorf("%v: ListTrees() = (_, %q), want = (_, nil)", test.desc, err)
			continue
		}

		got := resp.Tree
		sortByTreeID(got)
		if diff := pretty.Compare(got, createdTrees); diff != "" {
			t.Errorf("%v: post-ListTrees diff:\n%v", test.desc, diff)
		}

		for _, tree := range resp.Tree {
			if tree.PrivateKey != nil {
				t.Errorf("%v: PrivateKey not redacted: %v", test.desc, tree)
			}
		}
	}
}

func sortByTreeID(s []*trillian.Tree) {
	less := func(i, j int) bool {
		return s[i].TreeId < s[j].TreeId
	}
	sort.Slice(s, less)
}

// setupAdminServer prepares and starts an Admin Server, returning a client and
// a close function if successful.
// The close function should be defer-called if error is not nil to ensure a
// clean shutdown of resources.
func setupAdminServer() (trillian.TrillianAdminClient, func(), error) {
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, nil, err
	}
	// lis is closed via returned func

	registry, err := integration.NewRegistryForTests("AdminIntegrationTest")
	if err != nil {
		return nil, nil, err
	}

	ti := &interceptor.TrillianInterceptor{
		Admin:        registry.AdminStorage,
		QuotaManager: registry.QuotaManager,
	}
	netInterceptor := interceptor.Combine(interceptor.ErrorWrapper, ti.UnaryInterceptor)
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(netInterceptor))
	// grpcServer is stopped via returned func
	server := sa.New(registry)
	trillian.RegisterTrillianAdminServer(grpcServer, server)
	go grpcServer.Serve(lis)

	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		grpcServer.GracefulStop()
		lis.Close()
		return nil, nil, err
	}
	// conn is closed via returned func
	client := trillian.NewTrillianAdminClient(conn)

	closeFn := func() {
		conn.Close()
		grpcServer.GracefulStop()
		lis.Close()
	}
	return client, closeFn, nil
}
