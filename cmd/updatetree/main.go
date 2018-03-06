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

// Package main contains the implementation and entry point for the updatetree
// command.
//
// Example usage:
// $ ./updatetree --admin_server=host:port --tree_id=123456789 --tree_state=FROZEN
//
// The output is minimal to allow for easy usage in automated scripts.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/glog"
	"github.com/google/trillian"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	adminServerAddr = flag.String("admin_server", "", "Address of the gRPC Trillian Admin Server (host:port)")
	rpcDeadline     = flag.Duration("rpc_deadline", time.Second*10, "Deadline for RPC requests")
	treeID          = flag.Int64("tree_id", 0, "The ID of the tree to be set updated")
	treeState       = flag.String("tree_state", "", "If set the tree state will be updated")
)

func updateTree(ctx context.Context) (*trillian.Tree, error) {
	if *adminServerAddr == "" {
		return nil, errors.New("empty --admin_server, please provide the Admin server host:port")
	}

	// We only want to update the state of the tree, which means we need a field
	// mask on the request.
	treeStateMask := &field_mask.FieldMask{
		Paths: []string{"tree_state"},
	}

	newState, ok := proto.EnumValueMap("trillian.TreeState")[*treeState]
	if !ok {
		return nil, fmt.Errorf("invalid tree state: %v", *treeState)
	}

	req := &trillian.UpdateTreeRequest{
		Tree: &trillian.Tree{
			TreeId:    *treeID,
			TreeState: trillian.TreeState(newState),
		},
		UpdateMask: treeStateMask,
	}

	conn, err := grpc.Dial(*adminServerAddr, grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("failed to dial %v: %v", *adminServerAddr, err)
	}
	defer conn.Close()

	client := trillian.NewTrillianAdminClient(conn)
	for {
		tree, err := client.UpdateTree(ctx, req)
		if err == nil {
			return tree, nil
		}
		if s, ok := status.FromError(err); ok && s.Code() == codes.Unavailable {
			glog.Errorf("Admin server unavailable, trying again: %v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		return nil, fmt.Errorf("failed to UpdateTree(%+v): %T %v", req, err, err)
	}
}

func main() {
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *rpcDeadline)
	defer cancel()
	tree, err := updateTree(ctx)
	if err != nil {
		glog.Exitf("Failed to update tree: %v", err)
	}

	// DO NOT change the output format, scripts are meant to depend on it.
	// If you really want to change it, provide an output_format flag and
	// keep the default as-is.
	fmt.Println(tree.TreeState)
}
