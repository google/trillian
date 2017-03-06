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
			desc: "GetTree",
			fn: func(ctx context.Context, c trillian.TrillianAdminClient) error {
				_, err := c.GetTree(ctx, &trillian.GetTreeRequest{})
				return err
			},
		},
		{
			desc: "CreateTree",
			fn: func(ctx context.Context, c trillian.TrillianAdminClient) error {
				_, err := c.CreateTree(ctx, &trillian.CreateTreeRequest{})
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

	grpcServer := grpc.NewServer()
	// grpcServer is stopped via returned func
	server := sa.New()
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
