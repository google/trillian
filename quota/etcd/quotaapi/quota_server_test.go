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

package quotaapi

import (
	"context"
	"net"
	"testing"

	"github.com/google/trillian/quota/etcd/quotapb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestServer_Unimplemented(t *testing.T) {
	client, cleanup, err := startServer()
	if err != nil {
		t.Fatalf("startServer() returned err = %v", err)
	}
	defer cleanup()

	tests := []struct {
		desc string
		fn   func(context.Context, quotapb.QuotaClient) error
	}{
		{
			desc: "CreateConfig",
			fn: func(ctx context.Context, client quotapb.QuotaClient) error {
				_, err := client.CreateConfig(ctx, &quotapb.CreateConfigRequest{})
				return err
			},
		},
		{
			desc: "DeleteConfig",
			fn: func(ctx context.Context, client quotapb.QuotaClient) error {
				_, err := client.DeleteConfig(ctx, &quotapb.DeleteConfigRequest{})
				return err
			},
		},
		{
			desc: "GetConfig",
			fn: func(ctx context.Context, client quotapb.QuotaClient) error {
				_, err := client.GetConfig(ctx, &quotapb.GetConfigRequest{})
				return err
			},
		},
		{
			desc: "ListConfig",
			fn: func(ctx context.Context, client quotapb.QuotaClient) error {
				_, err := client.ListConfig(ctx, &quotapb.ListConfigRequest{})
				return err
			},
		},
		{
			desc: "UpdateConfig",
			fn: func(ctx context.Context, client quotapb.QuotaClient) error {
				_, err := client.UpdateConfig(ctx, &quotapb.UpdateConfigRequest{})
				return err
			},
		},
	}

	ctx := context.Background()
	want := codes.Unimplemented
	for _, test := range tests {
		err := test.fn(ctx, client)
		switch s, ok := status.FromError(err); {
		case !ok:
			t.Errorf("%v() returned a non-gRPC error: %v", test.desc, err)
		case s.Code() != want:
			t.Errorf("%v() returned err = %q, want code = %s", test.desc, err, want)
		}
	}
}

func startServer() (quotapb.QuotaClient, func(), error) {
	var lis net.Listener
	var s *grpc.Server
	var conn *grpc.ClientConn

	cleanup := func() {
		if conn != nil {
			conn.Close()
		}
		if s != nil {
			s.GracefulStop()
		}
		if lis != nil {
			lis.Close()
		}
	}

	var err error
	lis, err = net.Listen("tcp", "localhost:0")
	if err != nil {
		cleanup()
		return nil, nil, err
	}

	s = grpc.NewServer()
	quotapb.RegisterQuotaServer(s, &Server{})
	go s.Serve(lis)

	conn, err = grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		cleanup()
		return nil, nil, err
	}

	client := quotapb.NewQuotaClient(conn)
	return client, cleanup, nil
}
