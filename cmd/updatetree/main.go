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

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/client/rpcflags"
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
	treeType        = flag.String("tree_type", "", "If set the tree type will be updated")
	printTree       = flag.Bool("print", false, "Print the resulting tree")
)

type updateParams struct {
	treeState string
	treeType string
}

func updateTree(ctx context.Context, conn *grpc.ClientConn, up updateParams) (*trillian.Tree, error) {
	tree := &trillian.Tree{TreeId: *treeID}
	paths := make([]string, 0)

	if len(up.treeState) > 0 {
		m := proto.EnumValueMap("trillian.TreeState")
		if m == nil {
			return nil, fmt.Errorf("can't find enum value map for states")
		}
		newState, ok := m[up.treeState]
		if !ok {
			return nil, fmt.Errorf("invalid tree state: %v", up.treeState)
		}
		tree.TreeState = trillian.TreeState(newState)
		paths = append(paths, "tree_state")
	}

	if len(up.treeType) > 0 {
		m := proto.EnumValueMap("trillian.TreeType")
		if m == nil {
			return nil, fmt.Errorf("can't find enum value map for types")
		}
		newType, ok := m[up.treeType]
		if !ok {
			return nil, fmt.Errorf("invalid tree type: %v", up.treeType)
		}
		tree.TreeType = trillian.TreeType(newType)
		paths = append(paths, "tree_type")
	}

	if len(paths) == 0 {
		return nil, errors.New("nothing to change")
	}

	// We only want to update certain fields of the tree, which means we
	// need a field mask on the request.
	req := &trillian.UpdateTreeRequest{
		Tree:       tree,
		UpdateMask: &field_mask.FieldMask{Paths: paths},
	}

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
	defer glog.Flush()

	if *adminServerAddr == "" {
		glog.Exitf("empty --admin_server, please provide the Admin server host:port")
	}

	ctx, cancel := context.WithTimeout(context.Background(), *rpcDeadline)
	defer cancel()
	dialOpts, err := rpcflags.NewClientDialOptionsFromFlags()
	if err != nil {
		glog.Exitf("failed to create dial options: %v", err)
	}
	conn, err := grpc.Dial(*adminServerAddr, dialOpts...)
	if err != nil {
		glog.Exitf("failed to dial %v: %v", *adminServerAddr, err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			glog.Warningf("Close: %v", err)
		}
	}()

	up := updateParams{
		treeState: *treeState,
		treeType: *treeType,
	}

	tree, err := updateTree(ctx, conn, up)
	if err != nil {
		glog.Exitf("Failed to update tree: %v", err)
	}

	if *printTree {
		fmt.Println(proto.MarshalTextString(tree))
	} else {
		// DO NOT change the default output format, some scripts depend on it. If
		// you really want to change it, hide the new format behind a flag.
		fmt.Println(tree.TreeState)
	}
}
