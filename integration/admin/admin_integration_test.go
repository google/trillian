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
	"reflect"
	"testing"

	_ "github.com/go-sql-driver/mysql" // Load MySQL driver

	"github.com/google/trillian"
	"github.com/google/trillian/extension/builtin"
	sa "github.com/google/trillian/server/admin"
	"github.com/google/trillian/storage/mysql"
	"github.com/google/trillian/storage/testonly"
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
		desc string
		req  *trillian.CreateTreeRequest
		// TODO(codingllama): Check correctness of returned codes.Code
		wantErr bool
	}{
		{
			desc: "validTree",
			req: &trillian.CreateTreeRequest{
				Tree: testonly.LogTree,
			},
		},
		{
			desc:    "nilTree",
			req:     &trillian.CreateTreeRequest{},
			wantErr: true,
		},
		{
			desc: "invalidTree",
			req: &trillian.CreateTreeRequest{
				Tree: &invalidTree,
			},
			wantErr: true,
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		createdTree, err := client.CreateTree(ctx, test.req)
		if hasErr := err != nil; hasErr != test.wantErr {
			t.Errorf("%v: CreateTree() = (_, %v), wantErr = %v", test.desc, err, test.wantErr)
			continue
		} else if hasErr {
			continue
		}
		storedTree, err := client.GetTree(ctx, &trillian.GetTreeRequest{TreeId: createdTree.TreeId})
		if err != nil {
			t.Errorf("%v: GetTree() = (_, %v), want = (_, nil)", test.desc, err)
			continue
		}
		if !reflect.DeepEqual(storedTree, createdTree) {
			t.Errorf("%v: storedTree doesn't match createdTree:\nstoredTree =  %v,\ncreatedTree = %v", test.desc, storedTree, createdTree)
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
		desc    string
		treeID  int64
		wantErr bool
	}{
		{
			desc:   "notFound",
			treeID: -1,
			// TODO(codingllama): Check correctness of returned codes.Code
			wantErr: true,
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		_, err = client.GetTree(ctx, &trillian.GetTreeRequest{TreeId: test.treeID})
		if hasErr := err != nil; hasErr != test.wantErr {
			t.Errorf("%v: GetTree() = (_, %v), wantErr = %v", test.desc, err, test.wantErr)
		}
		// Success of GetTree is part of TestAdminServer_CreateTree, so it's not asserted
		// here.
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

	// TODO(alanparra): Have a standard way to get a registry for tests. With a few changes
	// we could leverage the utilities under testonly/integration.
	db, err := mysql.OpenDB(*builtin.MySQLURIFlag)
	if err != nil {
		return nil, nil, err
	}

	registry, err := builtin.NewExtensionRegistry(db, nil /* signer */)
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
