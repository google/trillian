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
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/quota/etcd/quotapb"
	"github.com/google/trillian/server/errors"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/trees"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	badInfoReason            = "bad_info"
	badTreeReason            = "bad_tree"
	insufficientTokensReason = "insufficient_tokens"
	getTreeStage             = "get_tree"
	getTokensStage           = "get_tokens"
)

var (
	// PutTokensTimeout is the timeout used for PutTokens calls.
	// PutTokens happens in a separate goroutine and with an independent context, therefore it has
	// its own timeout, separate from the RPC that causes the calls.
	PutTokensTimeout = 5 * time.Second

	requestCounter       monitoring.Counter
	requestDeniedCounter monitoring.Counter
	contextErrCounter    monitoring.Counter
	metricsOnce          sync.Once
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
	admin storage.AdminStorage
	qm    quota.Manager

	// quotaDryRun controls whether lack of tokens actually blocks requests (if set to true, no
	// requests are blocked by lack of tokens).
	quotaDryRun bool
}

// New returns a new TrillianInterceptor instance.
func New(admin storage.AdminStorage, qm quota.Manager, quotaDryRun bool, mf monitoring.MetricFactory) *TrillianInterceptor {
	metricsOnce.Do(func() { initMetrics(mf) })
	return &TrillianInterceptor{
		admin:       admin,
		qm:          qm,
		quotaDryRun: quotaDryRun,
	}
}

func initMetrics(mf monitoring.MetricFactory) {
	if mf == nil {
		mf = monitoring.InertMetricFactory{}
	}
	quota.InitMetrics(mf)
	requestCounter = mf.NewCounter(
		"interceptor_request_count",
		"Total number of intercepted requests",
		monitoring.TreeIDLabel)
	requestDeniedCounter = mf.NewCounter(
		"interceptor_request_denied_count",
		"Number of requests by denied, labeled according to the reason for denial",
		"reason", monitoring.TreeIDLabel, "quota_user")
	contextErrCounter = mf.NewCounter(
		"interceptor_context_err_counter",
		"Total number of times request context has been cancelled or deadline exceeded by stage",
		"stage")
}

func incRequestDeniedCounter(reason string, treeID int64, quotaUser string) {
	requestDeniedCounter.Inc(reason, fmt.Sprint(treeID), quotaUser)
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
	quotaUser := tp.parent.qm.GetUser(ctx, req)
	info, err := newRPCInfo(req, quotaUser)
	if err != nil {
		glog.Warningf("Failed to read tree info: %v", err)
		incRequestDeniedCounter(badInfoReason, 0, quotaUser)
		return ctx, err
	}
	tp.info = info
	requestCounter.Inc(fmt.Sprint(info.treeID))

	// TODO(codingllama): Add auth interception

	if info.getTree {
		tree, err := trees.GetTree(
			ctx, tp.parent.admin, info.treeID, trees.GetOpts{TreeType: info.treeType, Readonly: info.readonly})
		if err != nil {
			incRequestDeniedCounter(badTreeReason, info.treeID, quotaUser)
			return ctx, err
		}
		if err := ctx.Err(); err != nil {
			contextErrCounter.Inc(getTreeStage)
			return ctx, err
		}
		ctx = trees.NewContext(ctx, tree)
	}

	if info.quota && len(info.specs) > 0 && info.tokens > 0 {
		err := tp.parent.qm.GetTokens(ctx, info.tokens, info.specs)
		if err != nil {
			if !tp.parent.quotaDryRun {
				incRequestDeniedCounter(insufficientTokensReason, info.treeID, quotaUser)
				return ctx, status.Errorf(codes.ResourceExhausted, "quota exhausted: %v", err)
			}
			glog.Warningf("(quotaDryRun) Request %+v not denied due to dry run mode: %v", req, err)
		}
		quota.Metrics.IncAcquired(info.tokens, info.specs, err == nil)
		if err = ctx.Err(); err != nil {
			contextErrCounter.Inc(getTokensStage)
			return ctx, err
		}
	}

	return ctx, nil
}

func (tp *trillianProcessor) After(ctx context.Context, resp interface{}, handlerErr error) {
	switch {
	case tp.info == nil:
		glog.Warningf("After called with nil rpcInfo, resp = [%+v], handlerErr = [%v]", resp, handlerErr)
		return
	case !tp.info.quota:
		// After() currently only does quota processing
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
		case logLeafResponse:
			if !isLeafOK(resp.GetQueuedLeaf()) {
				tokens = 1
			}
		case logLeavesResponse:
			for _, leaf := range resp.GetQueuedLeaves() {
				if !isLeafOK(leaf) {
					tokens++
				}
			}
		}
	}
	if len(tp.info.specs) > 0 && tokens > 0 {
		// Run PutTokens in a separate goroutine and with a separate context.
		// It shouldn't block RPC completion, nor should it share the RPC's context deadline.
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), PutTokensTimeout)
			defer cancel()

			// TODO(codingllama): If PutTokens turns out to be unreliable we can still leak tokens. In
			// this case, we may want to keep tabs on how many tokens we failed to replenish and bundle
			// them up in the next PutTokens call (possibly as a QuotaManager decorator, or internally
			// in its impl).
			err := tp.parent.qm.PutTokens(ctx, tokens, tp.info.specs)
			if err != nil {
				glog.Warningf("Failed to replenish %v tokens: %v", tokens, err)
			}
			quota.Metrics.IncReturned(tokens, tp.info.specs, err == nil)
		}()
	}
}

func isLeafOK(leaf *trillian.QueuedLogLeaf) bool {
	// Be biased in favor of OK, as that matches TrillianLogRPCServer's behavior.
	return leaf == nil || leaf.Status == nil || leaf.Status.Code == int32(codes.OK)
}

type rpcInfo struct {
	// auth, getTree and quota enable their corresponding interceptor logic.
	auth, getTree, quota bool

	readonly bool
	treeID   int64
	treeType trillian.TreeType

	specs  []quota.Spec
	tokens int
}

func newRPCInfo(req interface{}, quotaUser string) (*rpcInfo, error) {
	// Set "safe" defaults: enable all interception and assume requests are readonly.
	info := &rpcInfo{
		auth:     true,
		getTree:  true,
		quota:    true,
		readonly: true,
		treeType: trillian.TreeType_UNKNOWN_TREE_TYPE,
	}

	switch req.(type) {

	// Not intercepted at all
	case
		// Quota configuration requests
		*quotapb.CreateConfigRequest,
		*quotapb.DeleteConfigRequest,
		*quotapb.GetConfigRequest,
		*quotapb.ListConfigsRequest,
		*quotapb.UpdateConfigRequest:
		info.auth = false
		info.getTree = false
		info.quota = false
		info.readonly = false // Doesn't really matter as all interceptors are turned off

	// Admin create
	case *trillian.CreateTreeRequest:
		info.auth = false    // Tree doesn't exist
		info.getTree = false // Tree doesn't exist
		info.quota = false   // No quota for admin
		info.readonly = false

	// Admin list
	case *trillian.ListTreesRequest:
		info.auth = false    // Auth done within RPC handler
		info.getTree = false // Zero to many trees
		info.quota = false   // No quota for admin

	// Admin / readonly
	case *trillian.GetTreeRequest:
		info.getTree = false // Read done within RPC handler
		info.quota = false   // No quota for admin

	// Admin / readwrite
	case *trillian.DeleteTreeRequest,
		*trillian.UndeleteTreeRequest,
		*trillian.UpdateTreeRequest:
		info.getTree = false // Read-modify-write done within RPC handler
		info.quota = false   // No quota for admin
		info.readonly = false

	// Log / readonly
	case *trillian.GetConsistencyProofRequest,
		*trillian.GetEntryAndProofRequest,
		*trillian.GetInclusionProofByHashRequest,
		*trillian.GetInclusionProofRequest,
		*trillian.GetLatestSignedLogRootRequest,
		*trillian.GetLeavesByHashRequest,
		*trillian.GetLeavesByIndexRequest,
		*trillian.GetSequencedLeafCountRequest:
		info.treeType = trillian.TreeType_LOG

	// Log / readwrite
	case *trillian.QueueLeafRequest,
		*trillian.QueueLeavesRequest:
		info.readonly = false
		info.treeType = trillian.TreeType_LOG

	// Map / readonly
	case *trillian.GetMapLeavesRequest,
		*trillian.GetSignedMapRootByRevisionRequest,
		*trillian.GetSignedMapRootRequest:
		info.treeType = trillian.TreeType_MAP

	// Map / readwrite
	case *trillian.SetMapLeavesRequest:
		info.readonly = false
		info.treeType = trillian.TreeType_MAP

	default:
		return nil, status.Errorf(codes.Internal, "unmapped request type: %T", req)
	}

	if info.auth || info.getTree || info.quota {
		switch req := req.(type) {
		case logIDRequest:
			info.treeID = req.GetLogId()
		case mapIDRequest:
			info.treeID = req.GetMapId()
		case treeIDRequest:
			info.treeID = req.GetTreeId()
		case treeRequest:
			info.treeID = req.GetTree().GetTreeId()
		default:
			return nil, status.Errorf(codes.Internal, "cannot retrieve treeID from request: %T", req)
		}
	}

	if info.quota {
		var kind quota.Kind
		if info.readonly {
			kind = quota.Read
		} else {
			kind = quota.Write
		}
		info.specs = []quota.Spec{
			{Group: quota.User, Kind: kind, User: quotaUser},
			{Group: quota.Tree, Kind: kind, TreeID: info.treeID},
			{Group: quota.Global, Kind: kind},
		}
		switch req := req.(type) {
		case logLeavesRequest:
			info.tokens = len(req.GetLeaves())
		case mapLeavesRequest:
			info.tokens = len(req.GetLeaves())
		default:
			info.tokens = 1
		}
	}

	return info, nil
}

type logIDRequest interface {
	GetLogId() int64
}

type mapIDRequest interface {
	GetMapId() int64
}

type treeIDRequest interface {
	GetTreeId() int64
}

type treeRequest interface {
	GetTree() *trillian.Tree
}

type logLeavesRequest interface {
	GetLeaves() []*trillian.LogLeaf
}

type mapLeavesRequest interface {
	GetLeaves() []*trillian.MapLeaf
}

type logLeafResponse interface {
	GetQueuedLeaf() *trillian.QueuedLogLeaf
}

type logLeavesResponse interface {
	GetQueuedLeaves() []*trillian.QueuedLogLeaf
}

// Combine combines unary interceptors.
// They are nested in order, so interceptor[0] calls on to (and sees the result of) interceptor[1], etc.
func Combine(interceptors ...grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		for i := len(interceptors) - 1; i >= 0; i-- {
			intercept := interceptors[i]
			baseHandler := handler
			handler = func(ctx context.Context, req interface{}) (interface{}, error) {
				return intercept(ctx, req, info, baseHandler)
			}
		}
		return handler(ctx, req)
	}
}

// ErrorWrapper is a grpc.UnaryServerInterceptor that wraps the errors emitted by the underlying handler.
func ErrorWrapper(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	rsp, err := handler(ctx, req)
	return rsp, errors.WrapError(err)
}
