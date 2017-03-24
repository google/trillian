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
	"testing"

	"github.com/google/trillian"
	sa "github.com/google/trillian/server/admin"
	"github.com/google/trillian/storage/testonly"
	"github.com/google/trillian/testonly/integration"
	"github.com/kylelemons/godebug/pretty"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

func TestAdminServer_Unimplemented(t *testing.T) {
	client, closeFn, err := setupAdminServer()
	if err != nil {
		t.Fatalf("setupAdminServer() failed: %v", err)
	}
	defer closeFn()

	tests := []struct {
		desc string
		fn   func(context.Context, trillian.TrillianAdminClient) error
	}{
		{
			desc: "ListTrees",
			fn: func(ctx context.Context, c trillian.TrillianAdminClient) error {
				_, err := c.ListTrees(ctx, &trillian.ListTreesRequest{})
				return err
			},
		},
		{
			desc: "UpdateTree",
			fn: func(ctx context.Context, c trillian.TrillianAdminClient) error {
				_, err := c.UpdateTree(ctx, &trillian.UpdateTreeRequest{})
				return err
			},
		},
		{
			desc: "DeleteTree",
			fn: func(ctx context.Context, c trillian.TrillianAdminClient) error {
				_, err := c.DeleteTree(ctx, &trillian.DeleteTreeRequest{})
				return err
			},
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		if err := test.fn(ctx, client); grpc.Code(err) != codes.Unimplemented {
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
		if grpc.Code(err) != test.wantCode {
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
		if grpc.Code(err) != test.wantCode {
			t.Errorf("%v: GetTree() = (_, %v), wantCode = %v", test.desc, err, test.wantCode)
		}
		// Success of GetTree is part of TestAdminServer_CreateTree, so it's not asserted here.
	}
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

	grpcServer := grpc.NewServer()
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
