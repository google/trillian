// Copyright 2017 Google LLC. All Rights Reserved.
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

package interceptor

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/golang/protobuf/ptypes"
	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/quota/etcd/quotapb"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/testonly"
	"github.com/google/trillian/trees"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	serrors "github.com/google/trillian/server/errors"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
)

func TestServiceName(t *testing.T) {
	for _, tc := range []struct {
		desc   string
		method string
		want   string
	}{
		{desc: "trillian", method: "/trillian.TrillianLog/QueueLeaf", want: "trillian.TrillianLog"},
		{desc: "fullyqualified", method: "/some.package.service/method", want: "some.package.service"},
		{desc: "unqualified", method: "/service.method", want: "service"},
		{desc: "noleadingslash", method: "no.leading.slash/method"},
		{desc: "malformed", method: "/package.service.method"},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			if got, want := serviceName(tc.method), tc.want; got != want {
				t.Errorf("serviceName(%v): %v, want %v", tc.method, got, want)
			}
		})
	}
}

func TestTrillianInterceptor_TreeInterception(t *testing.T) {
	logTree := proto.Clone(testonly.LogTree).(*trillian.Tree)
	logTree.TreeId = 10
	mapTree := proto.Clone(testonly.MapTree).(*trillian.Tree)
	mapTree.TreeId = 11
	deletedTree := proto.Clone(testonly.LogTree).(*trillian.Tree)
	deletedTree.TreeId = 12
	deletedTree.Deleted = true
	deletedTree.DeleteTime = ptypes.TimestampNow()
	unknownTreeID := int64(999)

	tests := []struct {
		desc       string
		method     string
		req        interface{}
		handlerErr error
		wantErr    bool
		wantTree   *trillian.Tree
		cancelled  bool
	}{
		// TODO(codingllama): Admin requests don't benefit from tree-reading logic, but we may read
		// their tree IDs for auth purposes.
		{
			desc:   "adminReadByID",
			method: "/trillian.TrillianAdmin/GetTree",
			req:    &trillian.GetTreeRequest{TreeId: logTree.TreeId},
		},
		{
			desc:   "adminWriteByID",
			method: "/trillian.TrillianAdmin/DeleteTree",
			req:    &trillian.DeleteTreeRequest{TreeId: logTree.TreeId},
		},
		{
			desc:   "adminWriteByTree",
			method: "/trillian.TrillianAdmin/UpdateTree",
			req:    &trillian.UpdateTreeRequest{Tree: &trillian.Tree{TreeId: logTree.TreeId}},
		},
		{
			desc:     "logRPC",
			method:   "/trillian.TrillianLog/GetLatestSignedLogRoot",
			req:      &trillian.GetLatestSignedLogRootRequest{LogId: logTree.TreeId},
			wantTree: logTree,
		},
		{
			desc:    "unknownRequest",
			req:     "not-a-request",
			wantErr: false,
		},
		{
			desc:    "unknownTree",
			method:  "/trillian.TrillianLog/GetLatestSignedLogRoot",
			req:     &trillian.GetLatestSignedLogRootRequest{LogId: unknownTreeID},
			wantErr: true,
		},
		{
			desc:    "deletedTree",
			method:  "/trillian.TrillianLog/GetLatestSignedLogRoot",
			req:     &trillian.GetLatestSignedLogRootRequest{LogId: deletedTree.TreeId},
			wantErr: true,
		},
		{
			desc:      "cancelled",
			method:    "/trillian.TrillianLog/GetLatestSignedLogRoot",
			req:       &trillian.GetLatestSignedLogRootRequest{LogId: logTree.TreeId},
			cancelled: true,
			wantErr:   true,
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			admin := storage.NewMockAdminStorage(ctrl)
			adminTX := storage.NewMockReadOnlyAdminTX(ctrl)
			admin.EXPECT().Snapshot(gomock.Any()).AnyTimes().Return(adminTX, nil)
			adminTX.EXPECT().GetTree(gomock.Any(), logTree.TreeId).AnyTimes().Return(logTree, nil)
			adminTX.EXPECT().GetTree(gomock.Any(), mapTree.TreeId).AnyTimes().Return(mapTree, nil)
			adminTX.EXPECT().GetTree(gomock.Any(), deletedTree.TreeId).AnyTimes().Return(deletedTree, nil)
			adminTX.EXPECT().GetTree(gomock.Any(), unknownTreeID).AnyTimes().Return(nil, errors.New("not found"))
			adminTX.EXPECT().Close().AnyTimes().Return(nil)
			adminTX.EXPECT().Commit().AnyTimes().Return(nil)

			intercept := New(admin, quota.Noop(), false /* quotaDryRun */, nil /* mf */)
			handler := &fakeHandler{resp: "handler response", err: test.handlerErr}

			if test.cancelled {
				// Use a context that's already been cancelled
				newCtx, cancel := context.WithCancel(ctx)
				cancel()
				ctx = newCtx
			}

			resp, err := intercept.UnaryInterceptor(ctx, test.req,
				&grpc.UnaryServerInfo{FullMethod: test.method},
				handler.run)
			if hasErr := err != nil && err != test.handlerErr; hasErr != test.wantErr {
				t.Fatalf("UnaryInterceptor() returned err = %v, wantErr = %v", err, test.wantErr)
			} else if hasErr {
				return
			}

			if !handler.called {
				t.Fatal("handler not called")
			}
			if handler.resp != resp {
				t.Errorf("resp = %v, want = %v", resp, handler.resp)
			}
			if handler.err != err {
				t.Errorf("err = %v, want = %v", err, handler.err)
			}

			if test.wantTree != nil {
				switch tree, ok := trees.FromContext(handler.ctx); {
				case !ok:
					t.Error("tree not in handler ctx")
				case !proto.Equal(tree, test.wantTree):
					diff := cmp.Diff(tree, test.wantTree)
					t.Errorf("post-FromContext diff:\n%v", diff)
				}
			}
		})
	}
}

func TestTrillianInterceptor_QuotaInterception(t *testing.T) {
	logTree := proto.Clone(testonly.LogTree).(*trillian.Tree)
	logTree.TreeId = 10

	mapTree := proto.Clone(testonly.MapTree).(*trillian.Tree)
	mapTree.TreeId = 11

	preorderedTree := proto.Clone(testonly.PreorderedLogTree).(*trillian.Tree)
	preorderedTree.TreeId = 12

	charge1 := "alpaca"
	charge2 := "cama"
	charges := &trillian.ChargeTo{User: []string{charge1, charge2}}
	tests := []struct {
		desc         string
		dryRun       bool
		method       string
		req          interface{}
		specs        []quota.Spec
		getTokensErr error
		wantCode     codes.Code
		wantTokens   int
	}{
		{
			desc:   "logRead",
			method: "/trillian.TrillianLog/GetLatestSignedLogRoot",
			req:    &trillian.GetLatestSignedLogRootRequest{LogId: logTree.TreeId},
			specs: []quota.Spec{
				{Group: quota.Tree, Kind: quota.Read, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Read, Refundable: true},
			},
			wantTokens: 1,
		},
		{
			desc:   "logReadIndices",
			method: "/trillian.TrillianLog/GetLeavesByIndex",
			req:    &trillian.GetLeavesByIndexRequest{LogId: logTree.TreeId, LeafIndex: []int64{1, 2, 3}},
			specs: []quota.Spec{
				{Group: quota.Tree, Kind: quota.Read, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Read, Refundable: true},
			},
			wantTokens: 3,
		},
		{
			desc:   "logReadRange",
			method: "/trillian.TrillianLog/GetLeavesByRange",
			req:    &trillian.GetLeavesByRangeRequest{LogId: logTree.TreeId, Count: 123},
			specs: []quota.Spec{
				{Group: quota.Tree, Kind: quota.Read, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Read, Refundable: true},
			},
			wantTokens: 123,
		},
		{
			desc:   "logReadNegativeRange",
			method: "/trillian.TrillianLog/GetLeavesByRange",
			req:    &trillian.GetLeavesByRangeRequest{LogId: logTree.TreeId, Count: -123},
			specs: []quota.Spec{
				{Group: quota.Tree, Kind: quota.Read, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Read, Refundable: true},
			},
			wantTokens: 1,
		},
		{
			desc:   "logReadZeroRange",
			method: "/trillian.TrillianLog/GetLeavesByRange",
			req:    &trillian.GetLeavesByRangeRequest{LogId: logTree.TreeId, Count: 0},
			specs: []quota.Spec{
				{Group: quota.Tree, Kind: quota.Read, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Read, Refundable: true},
			},
			wantTokens: 1,
		},
		{
			desc:   "logRead with charges",
			method: "/trillian.TrillianLog/GetLatestSignedLogRoot",
			req:    &trillian.GetLatestSignedLogRootRequest{LogId: logTree.TreeId, ChargeTo: charges},
			specs: []quota.Spec{
				{Group: quota.User, Kind: quota.Read, User: charge1},
				{Group: quota.User, Kind: quota.Read, User: charge2},
				{Group: quota.Tree, Kind: quota.Read, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Read, Refundable: true},
			},
			wantTokens: 1,
		},
		{
			desc:   "logWrite",
			method: "/trillian.TrillianLog/QueueLeaf",
			req:    &trillian.QueueLeafRequest{LogId: logTree.TreeId},
			specs: []quota.Spec{
				{Group: quota.Tree, Kind: quota.Write, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Write, Refundable: true},
			},
			wantTokens: 1,
		},
		{
			desc:   "logWrite with charges",
			method: "/trillian.TrillianLog/QueueLeaf",
			req:    &trillian.QueueLeafRequest{LogId: logTree.TreeId, ChargeTo: charges},
			specs: []quota.Spec{
				{Group: quota.User, Kind: quota.Write, User: charge1},
				{Group: quota.User, Kind: quota.Write, User: charge2},
				{Group: quota.Tree, Kind: quota.Write, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Write, Refundable: true},
			},
			wantTokens: 1,
		},
		{
			desc:   "emptyBatchRequest",
			method: "/trillian.TrillianLog/QueueLeaves",
			req: &trillian.QueueLeavesRequest{
				LogId:  logTree.TreeId,
				Leaves: nil,
			},
		},
		{
			desc:   "batchLogLeavesRequest",
			method: "/trillian.TrillianLog/QueueLeaves",
			req: &trillian.QueueLeavesRequest{
				LogId:  logTree.TreeId,
				Leaves: []*trillian.LogLeaf{{}, {}, {}},
			},
			specs: []quota.Spec{
				{Group: quota.Tree, Kind: quota.Write, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Write, Refundable: true},
			},
			wantTokens: 3,
		},
		{
			desc:   "batchSequencedLogLeavesRequest",
			method: "/trillian.TrillianLog/AddSequencedLeaves",
			req: &trillian.AddSequencedLeavesRequest{
				LogId:  preorderedTree.TreeId,
				Leaves: []*trillian.LogLeaf{{}, {}, {}},
			},
			specs: []quota.Spec{
				{Group: quota.Tree, Kind: quota.Write, TreeID: preorderedTree.TreeId},
				{Group: quota.Global, Kind: quota.Write, Refundable: true},
			},
			wantTokens: 3,
		},
		{
			desc:   "batchLogLeavesRequest with charges",
			method: "/trillian.TrillianLog/QueueLeaves",
			req: &trillian.QueueLeavesRequest{
				LogId:    logTree.TreeId,
				Leaves:   []*trillian.LogLeaf{{}, {}, {}},
				ChargeTo: charges,
			},
			specs: []quota.Spec{
				{Group: quota.User, Kind: quota.Write, User: charge1},
				{Group: quota.User, Kind: quota.Write, User: charge2},
				{Group: quota.Tree, Kind: quota.Write, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Write, Refundable: true},
			},
			wantTokens: 3,
		},
		{
			desc:   "quotaError",
			method: "/trillian.TrillianLog/GetLatestSignedLogRoot",
			req:    &trillian.GetLatestSignedLogRootRequest{LogId: logTree.TreeId},
			specs: []quota.Spec{
				{Group: quota.Tree, Kind: quota.Read, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Read, Refundable: true},
			},
			getTokensErr: errors.New("not enough tokens"),
			wantCode:     codes.ResourceExhausted,
			wantTokens:   1,
		},
		{
			desc:   "quotaDryRunError",
			dryRun: true,
			method: "/trillian.TrillianLog/GetLatestSignedLogRoot",
			req:    &trillian.GetLatestSignedLogRootRequest{LogId: logTree.TreeId},
			specs: []quota.Spec{
				{Group: quota.Tree, Kind: quota.Read, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Read, Refundable: true},
			},
			getTokensErr: errors.New("not enough tokens"),
			wantTokens:   1,
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			admin := storage.NewMockAdminStorage(ctrl)
			adminTX := storage.NewMockReadOnlyAdminTX(ctrl)
			admin.EXPECT().Snapshot(gomock.Any()).AnyTimes().Return(adminTX, nil)
			adminTX.EXPECT().GetTree(gomock.Any(), logTree.TreeId).AnyTimes().Return(logTree, nil)
			adminTX.EXPECT().GetTree(gomock.Any(), mapTree.TreeId).AnyTimes().Return(mapTree, nil)
			adminTX.EXPECT().GetTree(gomock.Any(), preorderedTree.TreeId).AnyTimes().Return(preorderedTree, nil)
			adminTX.EXPECT().Close().AnyTimes().Return(nil)
			adminTX.EXPECT().Commit().AnyTimes().Return(nil)

			qm := quota.NewMockManager(ctrl)
			if test.wantTokens > 0 {
				qm.EXPECT().GetTokens(gomock.Any(), test.wantTokens, test.specs).Return(test.getTokensErr)
			}

			handler := &fakeHandler{resp: "ok"}
			intercept := New(admin, qm, test.dryRun, nil /* mf */)

			// resp and handler assertions are done by TestTrillianInterceptor_TreeInterception,
			// we're only concerned with the quota logic here.
			_, err := intercept.UnaryInterceptor(ctx, test.req,
				&grpc.UnaryServerInfo{FullMethod: test.method},
				handler.run)
			if s, ok := status.FromError(err); !ok || s.Code() != test.wantCode {
				t.Errorf("UnaryInterceptor() returned err = %q, wantCode = %v", err, test.wantCode)
			}
		})
	}
}

func TestTrillianInterceptor_QuotaInterception_ReturnsTokens(t *testing.T) {
	logTree := proto.Clone(testonly.LogTree).(*trillian.Tree)
	logTree.TreeId = 10

	tests := []struct {
		desc                         string
		method                       string
		req, resp                    interface{}
		specs                        []quota.Spec
		handlerErr                   error
		wantGetTokens, wantPutTokens int
	}{
		{
			desc:   "badRequest",
			method: "/trillian.TrillianLog/GetLatestSignedLogRoot",
			req:    &trillian.GetLatestSignedLogRootRequest{LogId: logTree.TreeId},
			specs: []quota.Spec{
				{Group: quota.Tree, Kind: quota.Read, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Read, Refundable: true},
			},
			handlerErr:    errors.New("bad request"),
			wantGetTokens: 1,
			wantPutTokens: 1,
		},
		{
			desc:   "newLeaf",
			method: "/trillian.TrillianLog/QueueLeaf",
			req:    &trillian.QueueLeafRequest{LogId: logTree.TreeId, Leaf: &trillian.LogLeaf{}},
			resp:   &trillian.QueueLeafResponse{QueuedLeaf: &trillian.QueuedLogLeaf{}},
			specs: []quota.Spec{
				{Group: quota.Tree, Kind: quota.Write, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Write, Refundable: true},
			},
			wantGetTokens: 1,
		},
		{
			desc:   "duplicateLeaf",
			method: "/trillian.TrillianLog/QueueLeaf",
			req:    &trillian.QueueLeafRequest{LogId: logTree.TreeId},
			resp: &trillian.QueueLeafResponse{
				QueuedLeaf: &trillian.QueuedLogLeaf{
					Status: status.New(codes.AlreadyExists, "duplicate leaf").Proto(),
				},
			},
			specs: []quota.Spec{
				{Group: quota.Tree, Kind: quota.Write, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Write, Refundable: true},
			},
			wantGetTokens: 1,
			wantPutTokens: 1,
		},
		{
			desc:   "newLeaves",
			method: "/trillian.TrillianLog/QueueLeaves",
			req: &trillian.QueueLeavesRequest{
				LogId:  logTree.TreeId,
				Leaves: []*trillian.LogLeaf{{}, {}, {}},
			},
			resp: &trillian.QueueLeavesResponse{
				QueuedLeaves: []*trillian.QueuedLogLeaf{{}, {}, {}}, // No explicit Status means OK
			},
			specs: []quota.Spec{
				{Group: quota.Tree, Kind: quota.Write, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Write, Refundable: true},
			},
			wantGetTokens: 3,
		},
		{
			desc:   "duplicateLeaves",
			method: "/trillian.TrillianLog/QueueLeaves",
			req: &trillian.QueueLeavesRequest{
				LogId:  logTree.TreeId,
				Leaves: []*trillian.LogLeaf{{}, {}, {}},
			},
			resp: &trillian.QueueLeavesResponse{
				QueuedLeaves: []*trillian.QueuedLogLeaf{
					{Status: status.New(codes.AlreadyExists, "duplicate leaf").Proto()},
					{Status: status.New(codes.AlreadyExists, "duplicate leaf").Proto()},
					{},
				},
			},
			specs: []quota.Spec{
				{Group: quota.Tree, Kind: quota.Write, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Write, Refundable: true},
			},
			wantGetTokens: 3,
			wantPutTokens: 2,
		},
		{
			desc:   "badQueueLeavesRequest",
			method: "/trillian.TrillianLog/QueueLeaves",
			req: &trillian.QueueLeavesRequest{
				LogId:  logTree.TreeId,
				Leaves: []*trillian.LogLeaf{{}, {}, {}},
			},
			specs: []quota.Spec{
				{Group: quota.Tree, Kind: quota.Write, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Write, Refundable: true},
			},
			handlerErr:    errors.New("bad request"),
			wantGetTokens: 3,
			wantPutTokens: 3,
		},
	}

	defer func(timeout time.Duration) {
		PutTokensTimeout = timeout
	}(PutTokensTimeout)
	PutTokensTimeout = 5 * time.Second

	// Use a ctx with a timeout smaller than PutTokensTimeout. Not too short or
	// spurious failures will occur when the deadline expires.
	ctx, cancel := context.WithTimeout(context.Background(), PutTokensTimeout-2*time.Second)
	defer cancel()

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			admin := storage.NewMockAdminStorage(ctrl)
			adminTX := storage.NewMockReadOnlyAdminTX(ctrl)
			admin.EXPECT().Snapshot(gomock.Any()).AnyTimes().Return(adminTX, nil)
			adminTX.EXPECT().GetTree(gomock.Any(), logTree.TreeId).AnyTimes().Return(logTree, nil)
			adminTX.EXPECT().Close().AnyTimes().Return(nil)
			adminTX.EXPECT().Commit().AnyTimes().Return(nil)
			putTokensCh := make(chan bool, 1)
			wantDeadline := time.Now().Add(PutTokensTimeout)

			qm := quota.NewMockManager(ctrl)
			if test.wantGetTokens > 0 {
				qm.EXPECT().GetTokens(gomock.Any(), test.wantGetTokens, test.specs).Return(nil)
			}
			if test.wantPutTokens > 0 {
				refunds := make([]quota.Spec, 0)
				for _, s := range test.specs {
					if s.Refundable {
						refunds = append(refunds, s)
					}
				}
				qm.EXPECT().PutTokens(gomock.Any(), test.wantPutTokens, refunds).Do(func(ctx context.Context, numTokens int, specs []quota.Spec) {
					switch d, ok := ctx.Deadline(); {
					case !ok:
						t.Errorf("PutTokens() ctx has no deadline: %v", ctx)
					case d.Before(wantDeadline):
						t.Errorf("PutTokens() ctx deadline too short, got %v, want >= %v", d, wantDeadline)
					}
					putTokensCh <- true
				}).Return(nil)
			}

			handler := &fakeHandler{resp: test.resp, err: test.handlerErr}
			intercept := New(admin, qm, false /* quotaDryRun */, nil /* mf */)

			if _, err := intercept.UnaryInterceptor(ctx, test.req,
				&grpc.UnaryServerInfo{FullMethod: test.method},
				handler.run); err != test.handlerErr {
				t.Errorf("UnaryInterceptor() returned err = [%v], want = [%v]", err, test.handlerErr)
			}

			// PutTokens may be delegated to a separate goroutine. Give it some time to complete.
			select {
			case <-putTokensCh:
				// OK
			case <-time.After(1 * time.Second):
				// No need to error here, gomock will fail if the call is missing.
			}
		})
	}
}

func TestTrillianInterceptor_NotIntercepted(t *testing.T) {
	tests := []struct {
		method string
		req    interface{}
	}{
		// Admin
		{method: "/trillian.TrillianAdmin/CreateTree", req: &trillian.CreateTreeRequest{}},
		{method: "/trillian.TrillianAdmin/ListTrees", req: &trillian.ListTreesRequest{}},
		// Quota
		{method: "/quotapb.Quota/CreateConfig", req: &quotapb.CreateConfigRequest{}},
		{method: "/quotapb.Quota/DeleteConfig", req: &quotapb.DeleteConfigRequest{}},
		{method: "/quotapb.Quota/GetConfig", req: &quotapb.GetConfigRequest{}},
		{method: "/quotapb.Quota/ListConfigs", req: &quotapb.ListConfigsRequest{}},
		{method: "/quotapb.Quota/UpdateConfig", req: &quotapb.UpdateConfigRequest{}},
	}

	ctx := context.Background()
	for _, test := range tests {
		handler := &fakeHandler{}
		intercept := New(nil /* admin */, quota.Noop(), false /* quotaDryRun */, nil /* mf */)
		if _, err := intercept.UnaryInterceptor(ctx, test.req,
			&grpc.UnaryServerInfo{FullMethod: test.method},
			handler.run); err != nil {
			t.Errorf("UnaryInterceptor(%#v) returned err = %v", test.req, err)
		}
		if !handler.called {
			t.Errorf("UnaryInterceptor(%#v): handler not called", test.req)
		}
	}
}

// TestTrillianInterceptor_BeforeAfter tests a few Before/After interactions that are
// difficult/impossible to get unless the methods are called separately (i.e., not via
// UnaryInterceptor()).
func TestTrillianInterceptor_BeforeAfter(t *testing.T) {
	logTree := proto.Clone(testonly.LogTree).(*trillian.Tree)
	logTree.TreeId = 10

	qm := quota.Noop()

	tests := []struct {
		desc          string
		req, resp     interface{}
		handlerErr    error
		wantBeforeErr bool
	}{
		{
			desc: "success",
			req:  &trillian.CreateTreeRequest{},
			resp: &trillian.Tree{},
		},
		{
			desc:          "badRequest",
			req:           "bad",
			resp:          nil,
			handlerErr:    errors.New("bad"),
			wantBeforeErr: true,
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			admin := storage.NewMockAdminStorage(ctrl)
			adminTX := storage.NewMockReadOnlyAdminTX(ctrl)
			admin.EXPECT().Snapshot(gomock.Any()).AnyTimes().Return(adminTX, nil)
			adminTX.EXPECT().GetTree(gomock.Any(), logTree.TreeId).AnyTimes().Return(logTree, nil)
			adminTX.EXPECT().Close().AnyTimes().Return(nil)
			adminTX.EXPECT().Commit().AnyTimes().Return(nil)

			intercept := New(admin, qm, false /* quotaDryRun */, nil /* mf */)
			p := intercept.NewProcessor()

			_, err := p.Before(ctx, test.req, "/trillian.TrillianLog/foo")
			if gotErr := err != nil; gotErr != test.wantBeforeErr {
				t.Fatalf("Before() returned err = %v, wantErr = %v", err, test.wantBeforeErr)
			}

			// Other TrillianInterceptor tests assert After side-effects more in-depth, silently
			// returning is good enough here.
			p.After(ctx, test.resp, "", test.handlerErr)
		})
	}
}

func TestCombine(t *testing.T) {
	i1 := &fakeInterceptor{key: "key1", val: "foo"}
	i2 := &fakeInterceptor{key: "key2", val: "bar"}
	i3 := &fakeInterceptor{key: "key3", val: "baz"}
	e1 := &fakeInterceptor{err: errors.New("intercept error")}

	handlerErr := errors.New("handler error")

	tests := []struct {
		desc         string
		interceptors []*fakeInterceptor
		handlerErr   error
		wantCalled   int
		wantErr      error
	}{
		{
			desc: "noInterceptors",
		},
		{
			desc:         "single",
			interceptors: []*fakeInterceptor{i1},
			wantCalled:   1,
		},
		{
			desc:         "multi1",
			interceptors: []*fakeInterceptor{i1, i2, i3},
			wantCalled:   3,
		},
		{
			desc:         "multi2",
			interceptors: []*fakeInterceptor{i3, i1, i2},
			wantCalled:   3,
		},
		{
			desc:         "handlerErr",
			interceptors: []*fakeInterceptor{i1, i2},
			handlerErr:   handlerErr,
			wantCalled:   2,
			wantErr:      handlerErr,
		},
		{
			desc:         "interceptErr",
			interceptors: []*fakeInterceptor{i1, e1, i2},
			wantCalled:   2,
			wantErr:      e1.err,
		},
	}

	ctx := context.Background()
	req := "request"
	info := &grpc.UnaryServerInfo{}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			if l := len(test.interceptors); l < test.wantCalled {
				t.Fatalf("len(interceptors) = %v, want >= %v", l, test.wantCalled)
			}

			intercepts := []grpc.UnaryServerInterceptor{}
			for _, i := range test.interceptors {
				i.called = false
				intercepts = append(intercepts, i.run)
			}
			intercept := grpc_middleware.ChainUnaryServer(intercepts...)

			handler := &fakeHandler{resp: "response", err: test.handlerErr}
			resp, err := intercept(ctx, req, info, handler.run)
			if err != test.wantErr {
				t.Fatalf("err = %q, want = %q", err, test.wantErr)
			}

			called := 0
			callsStopped := false
			for _, i := range test.interceptors {
				switch {
				case i.called:
					if callsStopped {
						t.Errorf("interceptor called out of order: %v", i)
					}
					called++
				case !i.called:
					// No calls should have happened from here on
					callsStopped = true
				}
			}
			if called != test.wantCalled {
				t.Errorf("called %v interceptors, want = %v", called, test.wantCalled)
			}

			// Assertions below this point assume that the handler was called (ie, all
			// interceptors succeeded).
			if err != nil && err != test.handlerErr {
				return
			}

			if resp != handler.resp {
				t.Errorf("resp = %v, want = %v", resp, handler.resp)
			}

			// Chain the ctxs for all called interceptors and verify it got through to the
			// handler.
			wantCtx := ctx
			for _, i := range test.interceptors {
				h := &fakeHandler{resp: "ok"}
				i.called = false
				_, err = i.run(wantCtx, req, info, h.run)
				if err != nil {
					t.Fatalf("unexpected handler failure: %v", err)
				}
				wantCtx = h.ctx
			}
			if got, want := fmt.Sprintf("%v", handler.ctx), fmt.Sprintf("%v", wantCtx); got != want {
				t.Errorf("handler ctx, %v, want %v", got, want)
			}
		})
	}
}

func TestErrorWrapper(t *testing.T) {
	badLlamaErr := status.Errorf(codes.InvalidArgument, "Bad Llama")
	tests := []struct {
		desc         string
		resp         interface{}
		err, wantErr error
	}{
		{
			desc: "success",
			resp: "ok",
		},
		{
			desc:    "error",
			err:     badLlamaErr,
			wantErr: serrors.WrapError(badLlamaErr),
		},
	}
	ctx := context.Background()
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			handler := fakeHandler{resp: test.resp, err: test.err}
			resp, err := ErrorWrapper(ctx, "req", &grpc.UnaryServerInfo{}, handler.run)
			if resp != test.resp {
				t.Errorf("resp = %v, want = %v", resp, test.resp)
			}
			if !equalError(err, test.wantErr) {
				t.Errorf("post-WrapErrors: got %v, want %v", err, test.wantErr)
			}
		})
	}
}

func equalError(x, y error) bool {
	return x == y || (x != nil && y != nil && x.Error() == y.Error())
}

type fakeHandler struct {
	called bool
	resp   interface{}
	err    error
	// Attributes recorded by run calls
	ctx context.Context
	req interface{}
}

func (f *fakeHandler) run(ctx context.Context, req interface{}) (interface{}, error) {
	if f.called {
		panic("handler already called; either create a new handler or set called to false before reusing")
	}
	f.called = true
	f.ctx = ctx
	f.req = req
	return f.resp, f.err
}

type fakeInterceptor struct {
	key    interface{}
	val    interface{}
	called bool
	err    error
}

func (f *fakeInterceptor) run(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	if f.called {
		panic("interceptor already called; either create a new interceptor or set called to false before reusing")
	}
	f.called = true
	if f.err != nil {
		return nil, f.err
	}
	return handler(context.WithValue(ctx, f.key, f.val), req)
}
