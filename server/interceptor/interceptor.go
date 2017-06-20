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
	"fmt"
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

const (
	// requestTypePrefix is the expected prefix for all request protos.
	requestTypePrefix = "*trillian."
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
func (i *TrillianInterceptor) UnaryInterceptor(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	// IMPORTANT: Do not rely on grpc.UnaryServerInfo in this filter. It makes life a lot harder
	// when adapting the code to our internal branch.

	quotaUser := i.QuotaManager.GetUser(ctx, req)
	rpcInfo, err := getRPCInfo(req, quotaUser)
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
// RPCs are mapped using the following logic:
// treeID is acquired via one of the "Request" interfaces defined below (treeIDRequest,
// logIDRequest, mapIDRequest, etc). Requests must implement to one of those.
// TreeType and Readonly are determined based on the request type.
func getRPCInfo(req interface{}, quotaUser string) (*rpcInfo, error) {
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

	var treeType trillian.TreeType
	switch {
	case isAdminRPC(req):
		treeType = trillian.TreeType_UNKNOWN_TREE_TYPE // unrestricted
	case isLogRPC(req):
		treeType = trillian.TreeType_LOG
	case isMapRPC(req):
		treeType = trillian.TreeType_MAP
	default:
		return nil, status.Errorf(codes.Internal, "unmapped request type: %T", req)
	}

	reqType := fmt.Sprintf("%T", req)
	if !strings.HasPrefix(reqType, requestTypePrefix) {
		return nil, status.Errorf(codes.Internal, "unexpected request type prefix: %v", reqType)
	}
	reqType = reqType[len(requestTypePrefix):]
	readonly := strings.HasPrefix(reqType, "Get") || strings.HasPrefix(reqType, "List")

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

func isAdminRPC(req interface{}) bool {
	switch req.(type) {
	// Please keep in alphabetical order
	case *trillian.CreateTreeRequest:
	case *trillian.DeleteTreeRequest:
	case *trillian.GetTreeRequest:
	case *trillian.ListTreesRequest:
	case *trillian.UpdateTreeRequest:
	default:
		return false
	}
	return true
}

func isLogRPC(req interface{}) bool {
	switch req.(type) {
	// Please keep in alphabetical order
	case *trillian.GetConsistencyProofRequest:
	case *trillian.GetEntryAndProofRequest:
	case *trillian.GetInclusionProofByHashRequest:
	case *trillian.GetInclusionProofRequest:
	case *trillian.GetLatestSignedLogRootRequest:
	case *trillian.GetLeavesByHashRequest:
	case *trillian.GetLeavesByIndexRequest:
	case *trillian.GetSequencedLeafCountRequest:
	case *trillian.QueueLeafRequest:
	case *trillian.QueueLeavesRequest:
	default:
		return false
	}
	return true
}

func isMapRPC(req interface{}) bool {
	switch req.(type) {
	// Please keep in alphabetical order
	case *trillian.GetMapLeavesRequest:
	case *trillian.GetSignedMapRootByRevisionRequest:
	case *trillian.GetSignedMapRootRequest:
	case *trillian.SetMapLeavesRequest:
	default:
		return false
	}
	return true
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

// Combine combines unary interceptors.
// They are nested in order, so interceptor[0] calls on to (and sees the result of) interceptor[1], etc.
func Combine(interceptors ...grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		for i := len(interceptors) - 1; i >= 0; i-- {
			interceptor := interceptors[i]
			baseHandler := handler
			handler = func(ctx context.Context, req interface{}) (interface{}, error) {
				return interceptor(ctx, req, info, baseHandler)
			}
		}
		return handler(ctx, req)
	}
}

// ErrorWrapper is a grpc.UnaryServerInterceptor that wraps the errors emitted by the underlying handler.
func ErrorWrapper(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	rsp, err := handler(ctx, req)
	return rsp, errors.WrapError(err)
}
