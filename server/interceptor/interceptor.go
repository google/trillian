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

	"github.com/golang/glog"
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

// RequestProcessor encapsulates the logic to intercept a request, split into separate stages:
// before and after the handler is invoked.
type RequestProcessor interface {

	// Before implements all interceptor logic that happens before the handler is called.
	// It returns a (potentially) modified context that's passed forward to the handler (and After),
	// plus an error, in case the request should be interrupted before the handler is invoked.
	Before(ctx context.Context, req interface{}) (context.Context, error)

	// After implements all interceptor logic that happens after the handler is invoked.
	// Before must be invoked prior to After and the same RequestProcessor instance must to be used
	// to process a given request.
	After(ctx context.Context, resp interface{}, handlerErr error)
}

// TrillianInterceptor checks that:
// * Requests addressing a tree have the correct tree type and tree state;
// * TODO(codingllama): Requests are properly authenticated / authorized ; and
// * Requests are rate limited appropriately.
type TrillianInterceptor struct {
	Admin        storage.AdminStorage
	QuotaManager quota.Manager

	// QuotaDryRun controls whether lack of tokens actually blocks requests (if set to true, no
	// requests are blocked by lack of tokens).
	QuotaDryRun bool
}

// UnaryInterceptor executes the TrillianInterceptor logic for unary RPCs.
func (i *TrillianInterceptor) UnaryInterceptor(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	// Implement UnaryInterceptor using a RequestProcessor, so we 1. exercise it and 2. make it
	// easier to port this logic to non-gRPC implementations.
	rp := i.NewProcessor()
	var err error
	ctx, err = rp.Before(ctx, req)
	if err != nil {
		return nil, err
	}
	resp, err := handler(ctx, req)
	rp.After(ctx, resp, err)
	return resp, err
}

// NewProcessor returns a RequestProcessor for the TrillianInterceptor logic.
func (i *TrillianInterceptor) NewProcessor() RequestProcessor {
	return &trillianProcessor{parent: i}
}

type trillianProcessor struct {
	parent *TrillianInterceptor
	info   *rpcInfo
}

func (tp *trillianProcessor) Before(ctx context.Context, req interface{}) (context.Context, error) {
	incRequestCounter()

	quotaUser := tp.parent.QuotaManager.GetUser(ctx, req)
	info, err := getRPCInfo(req, quotaUser)
	if err != nil {
		incRequestDeniedCounter(badInfoReason, 0, quotaUser)
		return ctx, err
	}
	tp.info = info

	if info.treeID != 0 {
		tree, err := trees.GetTree(ctx, tp.parent.Admin, info.treeID, info.opts)
		if err != nil {
			incRequestDeniedCounter(badTreeReason, info.treeID, quotaUser)
			return ctx, err
		}
		ctx = trees.NewContext(ctx, tree)
		// TODO(codingllama): Add auth interception
	}

	if len(info.specs) > 0 && info.tokens > 0 {
		err := tp.parent.QuotaManager.GetTokens(ctx, info.tokens, info.specs)
		if err != nil {
			if !tp.parent.QuotaDryRun {
				incRequestDeniedCounter(insufficientTokensReason, info.treeID, quotaUser)
				return ctx, status.Errorf(codes.ResourceExhausted, "quota exhausted: %v", err)
			}
			glog.Warningf("(QuotaDryRun) Request %+v not denied due to dry run mode: %v", req, err)
		}
		quota.Metrics.IncAcquired(info.tokens, info.specs, err != nil)
	}
	return ctx, nil
}

func (tp *trillianProcessor) After(ctx context.Context, resp interface{}, handlerErr error) {
	if tp.info == nil {
		glog.Warningf("After called with nil rpcInfo, resp = [%+v], handlerErr = [%v]", resp, handlerErr)
		return
	}

	// Decide if we have to replenish tokens. There are a few situations that require tokens to
	// be replenished:
	// * Invalid requests (a bad request shouldn't spend sequencing-based tokens, as it won't
	//   cause a corresponding sequencing to happen)
	// * Requests that filter out duplicates (e.g., QueueLeaf and QueueLeaves, for the same
	//   reason as above: duplicates aren't queued for sequencing)
	tokens := 0
	if handlerErr != nil {
		// Return the tokens spent by invalid requests
		tokens = tp.info.tokens
	} else {
		switch resp := resp.(type) {
		case *trillian.QueueLeafResponse:
			if !isLeafOK(resp.GetQueuedLeaf()) {
				tokens = 1
			}
		case *trillian.QueueLeavesResponse:
			for _, leaf := range resp.GetQueuedLeaves() {
				if !isLeafOK(leaf) {
					tokens++
				}
			}
		}
	}
	if tokens > 0 && len(tp.info.specs) > 0 {
		// TODO(codingllama): If PutTokens turns out to be unreliable we can still leak tokens. In
		// this case, we may want to keep tabs on how many tokens we failed to replenish and bundle
		// them up in the next PutTokens call (possibly as a QuotaManager decorator, or internally
		// in its impl).
		err := tp.parent.QuotaManager.PutTokens(ctx, tokens, tp.info.specs)
		if err != nil {
			glog.Warningf("Failed to replenish %v tokens: %v", tokens, err)
		}
		quota.Metrics.IncReturned(tokens, tp.info.specs, err != nil)
	}
}

func isLeafOK(leaf *trillian.QueuedLogLeaf) bool {
	// Be biased in favor of OK, as that matches TrillianLogRPCServer's behavior.
	return leaf == nil || leaf.Status == nil || leaf.Status.Code == int32(codes.OK)
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

	// tokens is number of quota tokens consumed by this request.
	tokens int
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

	treeType, readonly, err := getRequestInfo(req)
	if err != nil {
		return nil, err
	}

	kind := quota.Read
	if !readonly {
		kind = quota.Write
	}
	var specs []quota.Spec
	switch {
	case treeType == trillian.TreeType_UNKNOWN_TREE_TYPE:
		// Don't impose quota on Admin requests.
		// Sequencing-based replenishment is not tied in any way to Admin, so charging tokens for it
		// leads to direct leakage.
		// Admin is meant to be internal and unlikely to be a source of high QPS, in any case.
	case treeID == 0:
		specs = []quota.Spec{
			{Group: quota.User, Kind: kind, User: quotaUser},
			{Group: quota.Global, Kind: kind},
		}
	default:
		specs = []quota.Spec{
			{Group: quota.User, Kind: kind, User: quotaUser},
			{Group: quota.Tree, Kind: kind, TreeID: treeID},
			{Group: quota.Global, Kind: kind},
		}
	}

	tokens := 1
	switch req := req.(type) {
	case logLeavesRequest:
		tokens = len(req.GetLeaves())
	case mapLeavesRequest:
		tokens = len(req.GetLeaves())
	}

	return &rpcInfo{
		treeID: treeID,
		opts:   trees.GetOpts{TreeType: treeType, Readonly: readonly},
		specs:  specs,
		tokens: tokens,
	}, nil
}

func getRequestInfo(req interface{}) (trillian.TreeType, bool, error) {
	if readonly, ok := getAdminRequestInfo(req); ok {
		return trillian.TreeType_UNKNOWN_TREE_TYPE, readonly, nil
	}
	if readonly, ok := getLogRequestInfo(req); ok {
		return trillian.TreeType_LOG, readonly, nil
	}
	if readonly, ok := getMapRequestInfo(req); ok {
		return trillian.TreeType_MAP, readonly, nil
	}
	return trillian.TreeType_UNKNOWN_TREE_TYPE, false, fmt.Errorf("unmapped request type: %T", req)
}

func getAdminRequestInfo(req interface{}) (bool, bool) {
	readonly := false
	ok := true
	switch req.(type) {
	case *trillian.GetTreeRequest,
		*trillian.ListTreesRequest:
		readonly = true
	case *trillian.CreateTreeRequest,
		*trillian.DeleteTreeRequest,
		*trillian.UpdateTreeRequest:
	default:
		ok = false
	}
	return readonly, ok
}

func getLogRequestInfo(req interface{}) (bool, bool) {
	readonly := false
	ok := true
	switch req.(type) {
	case *trillian.GetConsistencyProofRequest,
		*trillian.GetEntryAndProofRequest,
		*trillian.GetInclusionProofByHashRequest,
		*trillian.GetInclusionProofRequest,
		*trillian.GetLatestSignedLogRootRequest,
		*trillian.GetLeavesByHashRequest,
		*trillian.GetLeavesByIndexRequest,
		*trillian.GetSequencedLeafCountRequest:
		readonly = true
	case *trillian.QueueLeafRequest,
		*trillian.QueueLeavesRequest:
	default:
		ok = false
	}
	return readonly, ok
}

func getMapRequestInfo(req interface{}) (bool, bool) {
	readonly := false
	ok := true
	switch req.(type) {
	case *trillian.GetMapLeavesRequest,
		*trillian.GetSignedMapRootByRevisionRequest,
		*trillian.GetSignedMapRootRequest:
		readonly = true
	case *trillian.SetMapLeavesRequest:
	default:
		ok = false
	}
	return readonly, ok
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

type logLeavesRequest interface {
	GetLeaves() []*trillian.LogLeaf
}

type mapLeavesRequest interface {
	GetLeaves() []*trillian.MapLeaf
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
