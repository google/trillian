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

// Package interceptor defines gRPC interceptors for Trillian.
package interceptor

import (
	"github.com/google/trillian"
	"github.com/google/trillian/errors"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/trees"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// TreeInterceptor ensures that all requests pertaining a specific tree are of correct type and
// respect the current tree state (frozen, deleted, etc).
type TreeInterceptor struct {
	Admin storage.AdminStorage
}

// UnaryInterceptor executes the TreeInterceptor logic for unary RPCs.
func (i *TreeInterceptor) UnaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	rpcInfo, err := getRPCInfo(req)
	if err != nil {
		return nil, err
	}

	if rpcInfo.doesNotHaveTree {
		return handler(ctx, req)
	}

	tree, err := trees.GetTree(ctx, i.Admin, rpcInfo.treeID, rpcInfo.opts)
	if err != nil {
		return nil, err
	}

	ctx = trees.NewContext(ctx, tree)
	return handler(ctx, req)
}

// rpcInfo contains information about an RPC, as extracted from its request message.
type rpcInfo struct {
	// doesNotHaveTree states whether the RPC is tied to a specific tree.
	// Examples of RPCs without trees are CreateTree (no tree exists yet) and ListTrees
	// (zero to many trees returned).
	doesNotHaveTree bool
	// treeID is the tree ID tied to this RPC, if any.
	treeID int64
	// opts is the trees.GetOpts appropriate to this RPC (TreeType, readonly vs readwrite, etc).
	// opts is not set if doesNotHaveTree is true.
	opts trees.GetOpts
}

// getRPCInfo returns the rpcInfo for the given request, or an error if the request is not mapped.
func getRPCInfo(req interface{}) (*rpcInfo, error) {
	switch req := req.(type) {
	// TrillianAdmin
	case *trillian.CreateTreeRequest:
		return &rpcInfo{doesNotHaveTree: true}, nil
	case *trillian.GetTreeRequest:
		return &rpcInfo{
			treeID: req.GetTreeId(),
			opts:   trees.GetOpts{Readonly: true},
		}, nil
	case *trillian.DeleteTreeRequest:
		return &rpcInfo{treeID: req.GetTreeId()}, nil
	case *trillian.ListTreesRequest:
		return &rpcInfo{doesNotHaveTree: true}, nil
	case *trillian.UpdateTreeRequest:
		return &rpcInfo{treeID: req.GetTree().GetTreeId()}, nil

	// TrillianLog
	case *trillian.GetConsistencyProofRequest:
		return &rpcInfo{
			treeID: req.GetLogId(),
			opts:   trees.GetOpts{TreeType: trillian.TreeType_LOG, Readonly: true},
		}, nil
	case *trillian.GetEntryAndProofRequest:
		return &rpcInfo{
			treeID: req.GetLogId(),
			opts:   trees.GetOpts{TreeType: trillian.TreeType_LOG, Readonly: true},
		}, nil
	case *trillian.GetInclusionProofByHashRequest:
		return &rpcInfo{
			treeID: req.GetLogId(),
			opts:   trees.GetOpts{TreeType: trillian.TreeType_LOG, Readonly: true},
		}, nil
	case *trillian.GetInclusionProofRequest:
		return &rpcInfo{
			treeID: req.GetLogId(),
			opts:   trees.GetOpts{TreeType: trillian.TreeType_LOG, Readonly: true},
		}, nil
	case *trillian.GetLatestSignedLogRootRequest:
		return &rpcInfo{
			treeID: req.GetLogId(),
			opts:   trees.GetOpts{TreeType: trillian.TreeType_LOG, Readonly: true},
		}, nil
	case *trillian.GetLeavesByHashRequest:
		return &rpcInfo{
			treeID: req.GetLogId(),
			opts:   trees.GetOpts{TreeType: trillian.TreeType_LOG, Readonly: true},
		}, nil
	case *trillian.GetLeavesByIndexRequest:
		return &rpcInfo{
			treeID: req.GetLogId(),
			opts:   trees.GetOpts{TreeType: trillian.TreeType_LOG, Readonly: true},
		}, nil
	case *trillian.GetSequencedLeafCountRequest:
		return &rpcInfo{
			treeID: req.GetLogId(),
			opts:   trees.GetOpts{TreeType: trillian.TreeType_LOG, Readonly: true},
		}, nil
	case *trillian.QueueLeafRequest:
		return &rpcInfo{
			treeID: req.GetLogId(),
			opts:   trees.GetOpts{TreeType: trillian.TreeType_LOG},
		}, nil
	case *trillian.QueueLeavesRequest:
		return &rpcInfo{
			treeID: req.GetLogId(),
			opts:   trees.GetOpts{TreeType: trillian.TreeType_LOG},
		}, nil

	// TrillianMap
	case *trillian.GetMapLeavesRequest:
		return &rpcInfo{
			treeID: req.GetMapId(),
			opts:   trees.GetOpts{TreeType: trillian.TreeType_MAP, Readonly: true},
		}, nil
	case *trillian.SetMapLeavesRequest:
		return &rpcInfo{
			treeID: req.GetMapId(),
			opts:   trees.GetOpts{TreeType: trillian.TreeType_MAP},
		}, nil
	case *trillian.GetSignedMapRootRequest:
		return &rpcInfo{
			treeID: req.GetMapId(),
			opts:   trees.GetOpts{TreeType: trillian.TreeType_MAP, Readonly: true},
		}, nil
	}

	return nil, errors.Errorf(errors.Internal, "request type not configured for interception: %T", req)
}

// Combine combines multiple unary interceptors. Interceptors are executed in the supplied order.
// If an interceptor fails (non-nil error), processing will stop and the error will be returned.
// If all interceptors succeed the handler will be called.
// Contexts are propagated between calls: the context of the first interceptor, as passed in to the
// handler function, is used for the second interceptor call and so on, until the handler is
// invoked. This ensures that request-level variables, transmitted via contexts, behave properly.
func Combine(interceptors ...grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	return func(initialCtx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Save and forward the ctx passed by interceptors, otherwise we lose the data they add.
		ctx := initialCtx
		ctxHandler := func(innerCtx context.Context, req interface{}) (interface{}, error) {
			ctx = innerCtx
			return nil, nil
		}
		for _, intercept := range interceptors {
			_, err := intercept(ctx, req, info, ctxHandler)
			if err != nil {
				return nil, err
			}
		}
		return handler(ctx, req)
	}
}
