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
	"context"
	"fmt"
	"regexp"
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
	traceSpanRoot            = "/trillian/server/int"
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
	enabledServices      = map[string]bool{
		"trillian.TrillianLog":   true,
		"trillian.TrillianMap":   true,
		"trillian.TrillianAdmin": true,
		"TrillianLog":            true,
		"TrillianMap":            true,
		"TrillianAdmin":          true,
	}
)

// RequestProcessor encapsulates the logic to intercept a request, split into separate stages:
// before and after the handler is invoked.
type RequestProcessor interface {

	// Before implements all interceptor logic that happens before the handler is called.
	// It returns a (potentially) modified context that's passed forward to the handler (and After),
	// plus an error, in case the request should be interrupted before the handler is invoked.
	Before(ctx context.Context, req interface{}, method string) (context.Context, error)

	// After implements all interceptor logic that happens after the handler is invoked.
	// Before must be invoked prior to After and the same RequestProcessor instance must to be used
	// to process a given request.
	After(ctx context.Context, resp interface{}, method string, handlerErr error)
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
func (i *TrillianInterceptor) UnaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	// Implement UnaryInterceptor using a RequestProcessor, so we
	// 1. exercise it
	// 2. make it easier to port this logic to non-gRPC implementations.

	rp := i.NewProcessor()
	var err error
	ctx, err = rp.Before(ctx, req, info.FullMethod)
	if err != nil {
		return nil, err
	}
	resp, err := handler(ctx, req)
	rp.After(ctx, resp, info.FullMethod, err)
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

func (tp *trillianProcessor) Before(ctx context.Context, req interface{}, method string) (context.Context, error) {
	// Skip if the interceptor is not enabled for this service.
	if !enabledServices[serviceName(method)] {
		return ctx, nil
	}

	// Don't want the Before to contain the action, so don't overwrite the ctx.
	innerCtx, spanEnd := spanFor(ctx, "Before")
	defer spanEnd()
	info, err := newRPCInfo(req)
	if err != nil {
		glog.Warningf("Failed to read tree info: %v", err)
		incRequestDeniedCounter(badInfoReason, 0, "")
		return ctx, err
	}
	tp.info = info
	requestCounter.Inc(fmt.Sprint(info.treeID))

	// TODO(codingllama): Add auth interception

	if info.getTree {
		tree, err := trees.GetTree(
			innerCtx, tp.parent.admin, info.treeID, trees.NewGetOpts(trees.Admin, info.treeTypes...))
		if err != nil {
			incRequestDeniedCounter(badTreeReason, info.treeID, info.quotaUsers)
			return ctx, err
		}
		if err := innerCtx.Err(); err != nil {
			contextErrCounter.Inc(getTreeStage)
			return ctx, err
		}
		ctx = trees.NewContext(ctx, tree)
	}

	if info.tokens > 0 && len(info.specs) > 0 {
		err := tp.parent.qm.GetTokens(innerCtx, info.tokens, info.specs)
		if err != nil {
			if !tp.parent.quotaDryRun {
				incRequestDeniedCounter(insufficientTokensReason, info.treeID, info.quotaUsers)
				return ctx, status.Errorf(codes.ResourceExhausted, "quota exhausted: %v", err)
			}
			glog.Warningf("(quotaDryRun) Request %+v not denied due to dry run mode: %v", req, err)
		}
		quota.Metrics.IncAcquired(info.tokens, info.specs, err == nil)
		if err = innerCtx.Err(); err != nil {
			contextErrCounter.Inc(getTokensStage)
			return ctx, err
		}
	}

	return ctx, nil
}

func (tp *trillianProcessor) After(ctx context.Context, resp interface{}, method string, handlerErr error) {
	if !enabledServices[serviceName(method)] {
		return
	}
	_, spanEnd := spanFor(ctx, "After")
	defer spanEnd()
	switch {
	case tp.info == nil:
		glog.Warningf("After called with nil rpcInfo, resp = [%+v], handlerErr = [%v]", resp, handlerErr)
		return
	case tp.info.tokens == 0:
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
		case *trillian.QueueLeafResponse:
			if !isLeafOK(resp.GetQueuedLeaf()) {
				tokens = 1
			}
		case *trillian.AddSequencedLeavesResponse:
			for _, leaf := range resp.GetResults() {
				if !isLeafOK(leaf) {
					tokens++
				}
			}
		case *trillian.QueueLeavesResponse:
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
			ctx, spanEnd := spanFor(context.Background(), "After.PutTokens")
			defer spanEnd()
			ctx, cancel := context.WithTimeout(ctx, PutTokensTimeout)
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

var fullyQualifiedRE = regexp.MustCompile(`^/([\w.]+)/(\w+)$`)
var unqualifiedRE = regexp.MustCompile(`^/(\w+)\.(\w+)$`)

// serviceName returns the fully qualified service name
// "some.package.service" for "/some.package.service/method".
// It returns the unqualified service name "service" for "/service.method".
func serviceName(fullMethod string) string {
	if matches := fullyQualifiedRE.FindStringSubmatch(fullMethod); len(matches) == 3 {
		return matches[1]
	}
	if matches := unqualifiedRE.FindStringSubmatch(fullMethod); len(matches) == 3 {
		return matches[1]
	}
	return ""
}

type rpcInfo struct {
	// getTree indicates whether the interceptor should populate treeID.
	getTree bool

	readonly  bool
	treeID    int64
	treeTypes []trillian.TreeType

	specs  []quota.Spec
	tokens int
	// Single string describing all of the users against which quota is requested.
	quotaUsers string
}

// chargable is satisfied by request proto messages which contain a GetChargeTo
// accessor.
type chargable interface {
	GetChargeTo() *trillian.ChargeTo
}

// chargedUsers returns user identifiers for any chargable user quotas.
func chargedUsers(req interface{}) []string {
	c, ok := req.(chargable)
	if !ok {
		return nil
	}
	chargeTo := c.GetChargeTo()
	if chargeTo == nil {
		return nil
	}

	return chargeTo.User
}

func newRPCInfoForRequest(req interface{}) (*rpcInfo, error) {
	// Set "safe" defaults: enable all interception and assume requests are readonly.
	info := &rpcInfo{
		getTree:   true,
		readonly:  true,
		treeTypes: nil,
		tokens:    0,
	}

	switch req := req.(type) {

	// Not intercepted at all
	case
		// Quota configuration requests
		*quotapb.CreateConfigRequest,
		*quotapb.DeleteConfigRequest,
		*quotapb.GetConfigRequest,
		*quotapb.ListConfigsRequest,
		*quotapb.UpdateConfigRequest:
		info.getTree = false
		info.readonly = false // Doesn't really matter as all interceptors are turned off

	// Admin create
	case *trillian.CreateTreeRequest:
		info.getTree = false // Tree doesn't exist
		info.readonly = false

	// Admin list
	case *trillian.ListTreesRequest:
		info.getTree = false // Zero to many trees

	// Admin / readonly
	case *trillian.GetTreeRequest:
		info.getTree = false // Read done within RPC handler

	// Admin / readwrite
	case *trillian.DeleteTreeRequest,
		*trillian.UndeleteTreeRequest,
		*trillian.UpdateTreeRequest:
		info.getTree = false // Read-modify-write done within RPC handler
		info.readonly = false

	// (Log + Pre-ordered Log) / readonly
	case *trillian.GetConsistencyProofRequest,
		*trillian.GetEntryAndProofRequest,
		*trillian.GetInclusionProofByHashRequest,
		*trillian.GetInclusionProofRequest,
		*trillian.GetLatestSignedLogRootRequest:
		info.treeTypes = []trillian.TreeType{trillian.TreeType_LOG, trillian.TreeType_PREORDERED_LOG}
		info.tokens = 1
	case *trillian.GetLeavesByHashRequest:
		info.treeTypes = []trillian.TreeType{trillian.TreeType_LOG, trillian.TreeType_PREORDERED_LOG}
		info.tokens = len(req.GetLeafHash())
	case *trillian.GetLeavesByIndexRequest:
		info.treeTypes = []trillian.TreeType{trillian.TreeType_LOG, trillian.TreeType_PREORDERED_LOG}
		info.tokens = len(req.GetLeafIndex())
	case *trillian.GetLeavesByRangeRequest:
		info.treeTypes = []trillian.TreeType{trillian.TreeType_LOG, trillian.TreeType_PREORDERED_LOG}
		info.tokens = 1
		if c := req.GetCount(); c > 1 {
			info.tokens = int(c)
		}
	case *trillian.GetSequencedLeafCountRequest:
		info.treeTypes = []trillian.TreeType{trillian.TreeType_LOG, trillian.TreeType_PREORDERED_LOG}

	// Log / readwrite
	case *trillian.QueueLeafRequest:
		info.readonly = false
		info.treeTypes = []trillian.TreeType{trillian.TreeType_LOG}
		info.tokens = 1
	case *trillian.QueueLeavesRequest:
		info.readonly = false
		info.treeTypes = []trillian.TreeType{trillian.TreeType_LOG}
		info.tokens = len(req.GetLeaves())

	// Pre-ordered Log / readwrite
	case *trillian.AddSequencedLeafRequest:
		info.readonly = false
		info.treeTypes = []trillian.TreeType{trillian.TreeType_PREORDERED_LOG}
		info.tokens = 1
	case *trillian.AddSequencedLeavesRequest:
		info.readonly = false
		info.treeTypes = []trillian.TreeType{trillian.TreeType_PREORDERED_LOG}
		info.tokens = len(req.GetLeaves())

	// (Log + Pre-ordered Log) / readwrite
	case *trillian.InitLogRequest:
		info.readonly = false
		info.treeTypes = []trillian.TreeType{trillian.TreeType_LOG, trillian.TreeType_PREORDERED_LOG}
		info.tokens = 1

	// Map / readonly
	case *trillian.GetMapLeafByRevisionRequest:
		info.treeTypes = []trillian.TreeType{trillian.TreeType_MAP}
		info.tokens = len(req.GetIndex())
	case *trillian.GetMapLeafRequest:
		info.treeTypes = []trillian.TreeType{trillian.TreeType_MAP}
		info.tokens = len(req.GetIndex())
	case *trillian.GetMapLeavesByRevisionRequest:
		info.treeTypes = []trillian.TreeType{trillian.TreeType_MAP}
		info.tokens = len(req.GetIndex())
	case *trillian.GetMapLeavesRequest:
		info.treeTypes = []trillian.TreeType{trillian.TreeType_MAP}
		info.tokens = len(req.GetIndex())
	case *trillian.GetSignedMapRootByRevisionRequest,
		*trillian.GetSignedMapRootRequest:
		info.treeTypes = []trillian.TreeType{trillian.TreeType_MAP}
		info.tokens = 1

	// Map / readwrite
	case *trillian.SetMapLeavesRequest:
		info.readonly = false
		info.treeTypes = []trillian.TreeType{trillian.TreeType_MAP}
		info.tokens = len(req.GetLeaves())
	case *trillian.InitMapRequest:
		info.readonly = false
		info.treeTypes = []trillian.TreeType{trillian.TreeType_MAP}
		info.tokens = 1

	default:
		return nil, status.Errorf(codes.Internal, "newRPCInfo: unmapped request type: %T", req)
	}

	return info, nil
}

func newRPCInfo(req interface{}) (*rpcInfo, error) {
	info, err := newRPCInfoForRequest(req)
	if err != nil {
		return nil, err
	}

	if info.getTree || info.tokens > 0 {
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

	if info.tokens > 0 {
		kind := quota.Write
		if info.readonly {
			kind = quota.Read
		}

		for _, user := range chargedUsers(req) {
			info.specs = append(info.specs, quota.Spec{Group: quota.User, Kind: kind, User: user})
			if len(info.quotaUsers) > 0 {
				info.quotaUsers += "+"
			}
			info.quotaUsers += user
		}
		info.specs = append(info.specs, []quota.Spec{
			{Group: quota.Tree, Kind: kind, TreeID: info.treeID},
			{Group: quota.Global, Kind: kind},
		}...)
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

// ErrorWrapper is a grpc.UnaryServerInterceptor that wraps the errors emitted by the underlying handler.
func ErrorWrapper(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	ctx, spanEnd := spanFor(ctx, "ErrorWrapper")
	defer spanEnd()
	rsp, err := handler(ctx, req)
	return rsp, errors.WrapError(err)
}

func spanFor(ctx context.Context, name string) (context.Context, func()) {
	return monitoring.StartSpan(ctx, fmt.Sprintf("%s.%s", traceSpanRoot, name))
}
