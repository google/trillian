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

package interceptor

import (
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/quota/etcd/quotapb"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/testonly"
	"github.com/google/trillian/trees"
	"github.com/kylelemons/godebug/pretty"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	terrors "github.com/google/trillian/errors"
	serrors "github.com/google/trillian/server/errors"
)

func TestTrillianInterceptor_TreeInterception(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logTree := proto.Clone(testonly.LogTree).(*trillian.Tree)
	logTree.TreeId = 10
	mapTree := proto.Clone(testonly.MapTree).(*trillian.Tree)
	mapTree.TreeId = 11
	deletedTree := proto.Clone(testonly.LogTree).(*trillian.Tree)
	deletedTree.TreeId = 12
	deletedTree.Deleted = true
	deletedTree.DeleteTime = ptypes.TimestampNow()
	unknownTreeID := int64(999)

	admin := storage.NewMockAdminStorage(ctrl)
	adminTX := storage.NewMockReadOnlyAdminTX(ctrl)
	admin.EXPECT().Snapshot(gomock.Any()).AnyTimes().Return(adminTX, nil)
	adminTX.EXPECT().GetTree(gomock.Any(), logTree.TreeId).AnyTimes().Return(logTree, nil)
	adminTX.EXPECT().GetTree(gomock.Any(), mapTree.TreeId).AnyTimes().Return(mapTree, nil)
	adminTX.EXPECT().GetTree(gomock.Any(), deletedTree.TreeId).AnyTimes().Return(deletedTree, nil)
	adminTX.EXPECT().GetTree(gomock.Any(), unknownTreeID).AnyTimes().Return(nil, errors.New("not found"))
	adminTX.EXPECT().Close().AnyTimes().Return(nil)
	adminTX.EXPECT().Commit().AnyTimes().Return(nil)

	tests := []struct {
		desc       string
		req        interface{}
		handlerErr error
		wantErr    bool
		wantTree   *trillian.Tree
		cancelled  bool
	}{
		// TODO(codingllama): Admin requests don't benefit from tree-reading logic, but we may read
		// their tree IDs for auth purposes.
		{
			desc: "adminReadByID",
			req:  &trillian.GetTreeRequest{TreeId: logTree.TreeId},
		},
		{
			desc: "adminWriteByID",
			req:  &trillian.DeleteTreeRequest{TreeId: logTree.TreeId},
		},
		{
			desc: "adminWriteByTree",
			req:  &trillian.UpdateTreeRequest{Tree: &trillian.Tree{TreeId: logTree.TreeId}},
		},
		{
			desc:     "logRPC",
			req:      &trillian.GetLatestSignedLogRootRequest{LogId: logTree.TreeId},
			wantTree: logTree,
		},
		{
			desc:     "mapRPC",
			req:      &trillian.GetSignedMapRootRequest{MapId: mapTree.TreeId},
			wantTree: mapTree,
		},
		{
			desc:    "unknownRequest",
			req:     "not-a-request",
			wantErr: true,
		},
		{
			desc:    "unknownTree",
			req:     &trillian.GetLatestSignedLogRootRequest{LogId: unknownTreeID},
			wantErr: true,
		},
		{
			desc:    "deletedTree",
			req:     &trillian.GetLatestSignedLogRootRequest{LogId: deletedTree.TreeId},
			wantErr: true,
		},
		{
			desc:      "cancelled",
			req:       &trillian.GetLatestSignedLogRootRequest{LogId: logTree.TreeId},
			cancelled: true,
			wantErr:   true,
		},
	}

	ctx := context.Background()
	intercept := New(admin, quota.Noop(), false /* quotaDryRun */, nil /* mf */)
	for _, test := range tests {
		handler := &fakeHandler{resp: "handler response", err: test.handlerErr}

		if test.cancelled {
			// Use a context that's already been cancelled
			newCtx, cancel := context.WithCancel(ctx)
			cancel()
			ctx = newCtx
		}

		resp, err := intercept.UnaryInterceptor(ctx, test.req, &grpc.UnaryServerInfo{}, handler.run)
		if hasErr := err != nil && err != test.handlerErr; hasErr != test.wantErr {
			t.Errorf("%v: UnaryInterceptor() returned err = %v, wantErr = %v", test.desc, err, test.wantErr)
			continue
		} else if hasErr {
			continue
		}

		if !handler.called {
			t.Errorf("%v: handler not called", test.desc)
			continue
		}
		if handler.resp != resp {
			t.Errorf("%v: resp = %v, want = %v", test.desc, resp, handler.resp)
		}
		if handler.err != err {
			t.Errorf("%v: err = %v, want = %v", test.desc, err, handler.err)
		}

		if test.wantTree != nil {
			switch tree, ok := trees.FromContext(handler.ctx); {
			case !ok:
				t.Errorf("%v: tree not in handler ctx", test.desc)
			case !proto.Equal(tree, test.wantTree):
				diff := pretty.Compare(tree, test.wantTree)
				t.Errorf("%v: post-FromContext diff:\n%v", test.desc, diff)
			}
		}
	}
}

func TestTrillianInterceptor_QuotaInterception(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logTree := *testonly.LogTree
	logTree.TreeId = 10

	mapTree := *testonly.MapTree
	mapTree.TreeId = 11

	admin := storage.NewMockAdminStorage(ctrl)
	adminTX := storage.NewMockReadOnlyAdminTX(ctrl)
	admin.EXPECT().Snapshot(gomock.Any()).AnyTimes().Return(adminTX, nil)
	adminTX.EXPECT().GetTree(gomock.Any(), logTree.TreeId).AnyTimes().Return(&logTree, nil)
	adminTX.EXPECT().GetTree(gomock.Any(), mapTree.TreeId).AnyTimes().Return(&mapTree, nil)
	adminTX.EXPECT().Close().AnyTimes().Return(nil)
	adminTX.EXPECT().Commit().AnyTimes().Return(nil)

	user := "llama"
	tests := []struct {
		desc         string
		dryRun       bool
		req          interface{}
		specs        []quota.Spec
		getTokensErr error
		wantCode     codes.Code
		wantTokens   int
	}{
		{
			desc: "logRead",
			req:  &trillian.GetLatestSignedLogRootRequest{LogId: logTree.TreeId},
			specs: []quota.Spec{
				{Group: quota.User, Kind: quota.Read, User: user},
				{Group: quota.Tree, Kind: quota.Read, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Read},
			},
			wantTokens: 1,
		},
		{
			desc: "logWrite",
			req:  &trillian.QueueLeafRequest{LogId: logTree.TreeId},
			specs: []quota.Spec{
				{Group: quota.User, Kind: quota.Write, User: user},
				{Group: quota.Tree, Kind: quota.Write, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Write},
			},
			wantTokens: 1,
		},
		{
			desc: "mapRead",
			req:  &trillian.GetMapLeavesRequest{MapId: mapTree.TreeId},
			specs: []quota.Spec{
				{Group: quota.User, Kind: quota.Read, User: user},
				{Group: quota.Tree, Kind: quota.Read, TreeID: mapTree.TreeId},
				{Group: quota.Global, Kind: quota.Read},
			},
			wantTokens: 1,
		},
		{
			desc: "emptyBatchRequest",
			req: &trillian.QueueLeavesRequest{
				LogId:  logTree.TreeId,
				Leaves: nil,
			},
		},
		{
			desc: "batchLogLeavesRequest",
			req: &trillian.QueueLeavesRequest{
				LogId:  logTree.TreeId,
				Leaves: []*trillian.LogLeaf{{}, {}, {}},
			},
			specs: []quota.Spec{
				{Group: quota.User, Kind: quota.Write, User: user},
				{Group: quota.Tree, Kind: quota.Write, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Write},
			},
			wantTokens: 3,
		},
		{
			desc: "batchMapLeavesRequest",
			req: &trillian.SetMapLeavesRequest{
				MapId:  mapTree.TreeId,
				Leaves: []*trillian.MapLeaf{{}, {}, {}, {}, {}},
			},
			specs: []quota.Spec{
				{Group: quota.User, Kind: quota.Write, User: user},
				{Group: quota.Tree, Kind: quota.Write, TreeID: mapTree.TreeId},
				{Group: quota.Global, Kind: quota.Write},
			},
			wantTokens: 5,
		},
		{
			desc: "quotaError",
			req:  &trillian.GetLatestSignedLogRootRequest{LogId: logTree.TreeId},
			specs: []quota.Spec{
				{Group: quota.User, Kind: quota.Read, User: user},
				{Group: quota.Tree, Kind: quota.Read, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Read},
			},
			getTokensErr: errors.New("not enough tokens"),
			wantCode:     codes.ResourceExhausted,
			wantTokens:   1,
		},
		{
			desc:   "quotaDryRunError",
			dryRun: true,
			req:    &trillian.GetLatestSignedLogRootRequest{LogId: logTree.TreeId},
			specs: []quota.Spec{
				{Group: quota.User, Kind: quota.Read, User: user},
				{Group: quota.Tree, Kind: quota.Read, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Read},
			},
			getTokensErr: errors.New("not enough tokens"),
			wantTokens:   1,
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		qm := quota.NewMockManager(ctrl)
		qm.EXPECT().GetUser(gomock.Any(), test.req).MaxTimes(1).Return(user)
		if test.wantTokens > 0 {
			qm.EXPECT().GetTokens(gomock.Any(), test.wantTokens, test.specs).Return(test.getTokensErr)
		}

		handler := &fakeHandler{resp: "ok"}
		intercept := New(admin, qm, test.dryRun, nil /* mf */)

		// resp and handler assertions are done by TestTrillianInterceptor_TreeInterception,
		// we're only concerned with the quota logic here.
		_, err := intercept.UnaryInterceptor(ctx, test.req, &grpc.UnaryServerInfo{}, handler.run)
		if s, ok := status.FromError(err); !ok || s.Code() != test.wantCode {
			t.Errorf("%v: UnaryInterceptor() returned err = %q, wantCode = %v", test.desc, err, test.wantCode)
		}
	}
}

func TestTrillianInterceptor_QuotaInterception_ReturnsTokens(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logTree := *testonly.LogTree
	logTree.TreeId = 10

	admin := storage.NewMockAdminStorage(ctrl)
	adminTX := storage.NewMockReadOnlyAdminTX(ctrl)
	admin.EXPECT().Snapshot(gomock.Any()).AnyTimes().Return(adminTX, nil)
	adminTX.EXPECT().GetTree(gomock.Any(), logTree.TreeId).AnyTimes().Return(&logTree, nil)
	adminTX.EXPECT().Close().AnyTimes().Return(nil)
	adminTX.EXPECT().Commit().AnyTimes().Return(nil)

	user := "llama"
	tests := []struct {
		desc                         string
		req, resp                    interface{}
		specs                        []quota.Spec
		handlerErr                   error
		wantGetTokens, wantPutTokens int
	}{
		{
			desc: "badRequest",
			req:  &trillian.GetLatestSignedLogRootRequest{LogId: logTree.TreeId},
			specs: []quota.Spec{
				{Group: quota.User, Kind: quota.Read, User: user},
				{Group: quota.Tree, Kind: quota.Read, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Read},
			},
			handlerErr:    errors.New("bad request"),
			wantGetTokens: 1,
			wantPutTokens: 1,
		},
		{
			desc: "newLeaf",
			req:  &trillian.QueueLeafRequest{LogId: logTree.TreeId, Leaf: &trillian.LogLeaf{}},
			resp: &trillian.QueueLeafResponse{QueuedLeaf: &trillian.QueuedLogLeaf{}},
			specs: []quota.Spec{
				{Group: quota.User, Kind: quota.Write, User: user},
				{Group: quota.Tree, Kind: quota.Write, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Write},
			},
			wantGetTokens: 1,
		},
		{
			desc: "duplicateLeaf",
			req:  &trillian.QueueLeafRequest{LogId: logTree.TreeId},
			resp: &trillian.QueueLeafResponse{
				QueuedLeaf: &trillian.QueuedLogLeaf{
					Status: status.New(codes.AlreadyExists, "duplicate leaf").Proto(),
				},
			},
			specs: []quota.Spec{
				{Group: quota.User, Kind: quota.Write, User: user},
				{Group: quota.Tree, Kind: quota.Write, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Write},
			},
			wantGetTokens: 1,
			wantPutTokens: 1,
		},
		{
			desc: "newLeaves",
			req: &trillian.QueueLeavesRequest{
				LogId:  logTree.TreeId,
				Leaves: []*trillian.LogLeaf{{}, {}, {}},
			},
			resp: &trillian.QueueLeavesResponse{
				QueuedLeaves: []*trillian.QueuedLogLeaf{{}, {}, {}}, // No explicit Status means OK
			},
			specs: []quota.Spec{
				{Group: quota.User, Kind: quota.Write, User: user},
				{Group: quota.Tree, Kind: quota.Write, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Write},
			},
			wantGetTokens: 3,
		},
		{
			desc: "duplicateLeaves",
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
				{Group: quota.User, Kind: quota.Write, User: user},
				{Group: quota.Tree, Kind: quota.Write, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Write},
			},
			wantGetTokens: 3,
			wantPutTokens: 2,
		},
		{
			desc: "badQueueLeavesRequest",
			req: &trillian.QueueLeavesRequest{
				LogId:  logTree.TreeId,
				Leaves: []*trillian.LogLeaf{{}, {}, {}},
			},
			specs: []quota.Spec{
				{Group: quota.User, Kind: quota.Write, User: user},
				{Group: quota.Tree, Kind: quota.Write, TreeID: logTree.TreeId},
				{Group: quota.Global, Kind: quota.Write},
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
		putTokensCh := make(chan bool, 1)
		wantDeadline := time.Now().Add(PutTokensTimeout)

		qm := quota.NewMockManager(ctrl)
		qm.EXPECT().GetUser(gomock.Any(), test.req).MaxTimes(1).Return(user)
		if test.wantGetTokens > 0 {
			qm.EXPECT().GetTokens(gomock.Any(), test.wantGetTokens, test.specs).Return(nil)
		}
		if test.wantPutTokens > 0 {
			qm.EXPECT().PutTokens(gomock.Any(), test.wantPutTokens, test.specs).Do(func(ctx context.Context, numTokens int, specs []quota.Spec) {
				switch d, ok := ctx.Deadline(); {
				case !ok:
					t.Errorf("%v: PutTokens() ctx has no deadline: %v", test.desc, ctx)
				case d.Before(wantDeadline):
					t.Errorf("%v: PutTokens() ctx deadline too short, got %v, want >= %v", test.desc, d, wantDeadline)
				}
				putTokensCh <- true
			}).Return(nil)
		}

		handler := &fakeHandler{resp: test.resp, err: test.handlerErr}
		intercept := New(admin, qm, false /* quotaDryRun */, nil /* mf */)

		if _, err := intercept.UnaryInterceptor(ctx, test.req, &grpc.UnaryServerInfo{}, handler.run); err != test.handlerErr {
			t.Errorf("%v: UnaryInterceptor() returned err = [%v], want = [%v]", test.desc, err, test.handlerErr)
		}

		// PutTokens may be delegated to a separate goroutine. Give it some time to complete.
		select {
		case <-putTokensCh:
			// OK
		case <-time.After(1 * time.Second):
			// No need to error here, gomock will fail if the call is missing.
		}
	}
}

func TestTrillianInterceptor_NotIntercepted(t *testing.T) {
	tests := []struct {
		req interface{}
	}{
		// Admin
		{req: &trillian.CreateTreeRequest{}},
		{req: &trillian.ListTreesRequest{}},
		// Quota
		{req: &quotapb.CreateConfigRequest{}},
		{req: &quotapb.DeleteConfigRequest{}},
		{req: &quotapb.GetConfigRequest{}},
		{req: &quotapb.ListConfigsRequest{}},
		{req: &quotapb.UpdateConfigRequest{}},
	}

	ctx := context.Background()
	for _, test := range tests {
		handler := &fakeHandler{}
		intercept := New(nil /* admin */, quota.Noop(), false /* quotaDryRun */, nil /* mf */)
		if _, err := intercept.UnaryInterceptor(ctx, test.req, &grpc.UnaryServerInfo{}, handler.run); err != nil {
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
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logTree := *testonly.LogTree
	logTree.TreeId = 10

	admin := storage.NewMockAdminStorage(ctrl)
	adminTX := storage.NewMockReadOnlyAdminTX(ctrl)
	admin.EXPECT().Snapshot(gomock.Any()).AnyTimes().Return(adminTX, nil)
	adminTX.EXPECT().GetTree(gomock.Any(), logTree.TreeId).AnyTimes().Return(&logTree, nil)
	adminTX.EXPECT().Close().AnyTimes().Return(nil)
	adminTX.EXPECT().Commit().AnyTimes().Return(nil)

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
		intercept := New(admin, qm, false /* quotaDryRun */, nil /* mf */)
		p := intercept.NewProcessor()

		_, err := p.Before(ctx, test.req)
		if gotErr := err != nil; gotErr != test.wantBeforeErr {
			t.Errorf("%v: Before() returned err = %v, wantErr = %v", test.desc, err, test.wantBeforeErr)
			continue
		}

		// Other TrillianInterceptor tests assert After side-effects more in-depth, silently
		// returning is good enough here.
		p.After(ctx, test.resp, test.handlerErr)
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
		if l := len(test.interceptors); l < test.wantCalled {
			t.Errorf("%v: len(interceptors) = %v, want >= %v", test.desc, l, test.wantCalled)
			continue
		}

		intercepts := []grpc.UnaryServerInterceptor{}
		for _, i := range test.interceptors {
			i.called = false
			intercepts = append(intercepts, i.run)
		}
		intercept := Combine(intercepts...)

		handler := &fakeHandler{resp: "response", err: test.handlerErr}
		resp, err := intercept(ctx, req, info, handler.run)
		if err != test.wantErr {
			t.Errorf("%v: err = %q, want = %q", test.desc, err, test.wantErr)
			continue
		}

		called := 0
		callsStopped := false
		for _, i := range test.interceptors {
			switch {
			case i.called:
				if callsStopped {
					t.Errorf("%v: interceptor called out of order: %v", test.desc, i)
				}
				called++
			case !i.called:
				// No calls should have happened from here on
				callsStopped = true
			}
		}
		if called != test.wantCalled {
			t.Errorf("%v: called %v interceptors, want = %v", test.desc, called, test.wantCalled)
		}

		// Assertions below this point assume that the handler was called (ie, all
		// interceptors succeeded).
		if err != nil && err != test.handlerErr {
			continue
		}

		if resp != handler.resp {
			t.Errorf("%v: resp = %v, want = %v", test.desc, resp, handler.resp)
		}

		// Chain the ctxs for all called interceptors and verify it got through to the
		// handler.
		wantCtx := ctx
		for _, i := range test.interceptors {
			h := &fakeHandler{resp: "ok"}
			i.called = false
			_, err = i.run(wantCtx, req, info, h.run)
			if err != nil {
				t.Fatalf("%v: unexpected handler failure: %v", test.desc, err)
			}
			wantCtx = h.ctx
		}
		if diff := pretty.Compare(handler.ctx, wantCtx); diff != "" {
			t.Errorf("%v: handler ctx diff:\n%v", test.desc, diff)
		}
	}
}

func TestErrorWrapper(t *testing.T) {
	badLlamaErr := terrors.Errorf(terrors.InvalidArgument, "Bad Llama")
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
		handler := fakeHandler{resp: test.resp, err: test.err}
		resp, err := ErrorWrapper(ctx, "req", &grpc.UnaryServerInfo{}, handler.run)
		if resp != test.resp {
			t.Errorf("%v: resp = %v, want = %v", test.desc, resp, test.resp)
		}
		if diff := pretty.Compare(err, test.wantErr); diff != "" {
			t.Errorf("%v: post-WrapErrors diff:\n%v", test.desc, diff)
		}
	}
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
