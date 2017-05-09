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
	"strings"

	"github.com/google/trillian"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/server/errors"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/trees"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TrillianInterceptor checks that:
// * Requests addressing a tree have the correct tree type and tree state;
// * TODO(codingllama): Requests are properly authenticated / authorized ; and
// * Requests are rate limited appropriately.
type TrillianInterceptor struct {
	Admin        storage.AdminStorage
	QuotaManager quota.Manager
}

// UnaryInterceptor executes the TrillianInterceptor logic for unary RPCs.
func (i *TrillianInterceptor) UnaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	quotaUser := i.QuotaManager.GetUser(ctx, req)

	rpcInfo, err := getRPCInfo(req, info.FullMethod, quotaUser)
	if err != nil {
		return nil, err
	}

	if rpcInfo.treeID != 0 {
		tree, err := trees.GetTree(ctx, i.Admin, rpcInfo.treeID, rpcInfo.opts)
		if err != nil {
			return nil, err
		}
		ctx = trees.NewContext(ctx, tree)

		// TODO(codingllama): Add auth interception
	}

	if err := i.QuotaManager.GetTokens(ctx, 1 /* numTokens */, rpcInfo.specs); err != nil {
		return nil, status.Errorf(codes.ResourceExhausted, "quota exhausted: %v", err)
	}

	return handler(ctx, req)
}

// rpcInfo contains information about an RPC, as extracted from its request message.
type rpcInfo struct {
	// treeID is the tree ID tied to this RPC, if any (zero means no tree).
	treeID int64

	// opts is the trees.GetOpts appropriate to this RPC (TreeType, readonly vs readwrite, etc).
	// opts is not set if doesNotHaveTree is true.
	opts trees.GetOpts

	// specs contains the quota specifications for this RPC.
	specs []quota.Spec
}

// getRPCInfo returns the rpcInfo for the given request, or an error if the request is not mapped.
// Full method is the full RPC method name, as acquired from grpc.UnaryServerInfo (e.g.,
// /trillian.TrillianLog/GetInclusionProof).
// RPCs are mapped using the following logic:
// Tree IDs are acquired via one of the "Request" interfaces defined below (treeIDRequest,
// logIDRequest, mapIDRequest, etc). Requests must implement to one of those.
// Tree type is determined by the RPC service name: TrillianAdmin is unrestricted,
// TrillianLog = LOG, TrillianMap = MAP.
// Readonly status is determined by the RPC method name: if it starts with Get or List it's
// considered readonly.
// Finally, a few RPCs are hand-mapped as doesNotHaveTree, such as Create and ListTree.
func getRPCInfo(req interface{}, fullMethod, quotaUser string) (*rpcInfo, error) {
	var treeID int64
	switch req := req.(type) {
	case *trillian.CreateTreeRequest:
		// OK, tree is being created
	case *trillian.ListTreesRequest:
		// OK, no single tree ID (potentially many trees)
	case treeIDRequest:
		treeID = req.GetTreeId()
	case treeRequest:
		treeID = req.GetTree().GetTreeId()
	case logIDRequest:
		treeID = req.GetLogId()
	case mapIDRequest:
		treeID = req.GetMapId()
	default:
		return nil, status.Errorf(codes.Internal, "cannot retrieve treeID from request: %T", req)
	}

	serviceName, methodName, err := parseFullMethod(fullMethod)
	if err != nil {
		return nil, err
	}

	var treeType trillian.TreeType
	switch serviceName {
	case "trillian.TrillianAdmin":
		treeType = trillian.TreeType_UNKNOWN_TREE_TYPE // unrestricted
	case "trillian.TrillianLog":
		treeType = trillian.TreeType_LOG
	case "trillian.TrillianMap":
		treeType = trillian.TreeType_MAP
	default:
		return nil, status.Errorf(codes.Internal, "cannot determine treeType for request: %T", req)
	}

	readonly := strings.HasPrefix(methodName, "Get") || strings.HasPrefix(methodName, "List")

	kind := quota.Read
	if !readonly {
		kind = quota.Write
	}
	var specs []quota.Spec
	if treeID == 0 {
		specs = []quota.Spec{
			{Group: quota.User, Kind: kind, User: quotaUser},
			{Group: quota.Global, Kind: kind},
		}
	} else {
		specs = []quota.Spec{
			{Group: quota.User, Kind: kind, User: quotaUser},
			{Group: quota.Tree, Kind: kind, TreeID: treeID},
			{Group: quota.Global, Kind: kind},
		}
	}

	return &rpcInfo{
		treeID: treeID,
		opts:   trees.GetOpts{TreeType: treeType, Readonly: readonly},
		specs:  specs,
	}, nil
}

// parseFullMethod returns the service and method names as separate strings, without a trailing
// slash in either of them.
func parseFullMethod(fullMethod string) (string, string, error) {
	if !strings.HasPrefix(fullMethod, "/") {
		return "", "", status.Errorf(codes.Internal, "fullMethod must begin with '/': %v", fullMethod)
	}
	tmp := strings.Split(fullMethod[1:], "/")
	if len(tmp) != 2 {
		return "", "", status.Errorf(codes.Internal, "unexpected number of components in fullMethod (%v != 2): %v", len(tmp), fullMethod)
	}
	return tmp[0], tmp[1], nil
}

type treeIDRequest interface {
	GetTreeId() int64
}

type treeRequest interface {
	GetTree() *trillian.Tree
}

type logIDRequest interface {
	GetLogId() int64
}

type mapIDRequest interface {
	GetMapId() int64
}

// Combine combines multiple unary interceptors. Interceptors are executed in the supplied order.
// If an interceptor fails (non-nil error), processing will stop and the error will be returned.
// If all interceptors succeed the handler will be called.
// Contexts are propagated between calls: the context of the first interceptor, as passed in to the
// handler function, is used for the second interceptor call and so on, until the handler is
// invoked. This ensures that request-level variables, transmitted via contexts, behave properly.
// Note that, as a limitation of the chaining logic, handler output cannot be modified by any of
// the interceptors supplied.
func Combine(interceptors ...grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	return func(initialCtx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Build a handler function that just updates the scope's ctx variable.
		ctx := initialCtx
		ctxUpdater := func(innerCtx context.Context, req interface{}) (interface{}, error) {
			ctx = innerCtx
			return nil, nil
		}

		// Run each of the interceptors in order, using the ctxUpdater to accumulate changes
		// to the context, and exit if any of the interceptors give an error.
		for _, intercept := range interceptors {
			_, err := intercept(ctx, req, info, ctxUpdater)
			if err != nil {
				return nil, err
			}
		}

		// Run the real handler with the accumulated context.
		return handler(ctx, req)
	}
}

// WrapErrors wraps the errors returned by the supplied interceptor using errors.WrapError.
// If used in conjunction with Combine, it should be the last call in the chain, ie:
// WrapErrors(Combine(i1, i2, ..., in)).
func WrapErrors(i grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		resp, err := i(ctx, req, info, handler)
		return resp, errors.WrapError(err)
	}
}
