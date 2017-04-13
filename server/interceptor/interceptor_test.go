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

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	terrors "github.com/google/trillian/errors"
	serrors "github.com/google/trillian/server/errors"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/testonly"
	"github.com/google/trillian/trees"
	"github.com/kylelemons/godebug/pretty"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

func TestTreeInterceptor_UnaryInterceptor(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logTree := *testonly.LogTree
	logTree.TreeId = 10
	mapTree := *testonly.MapTree
	mapTree.TreeId = 11
	unknownTreeID := int64(999)

	admin := storage.NewMockAdminStorage(ctrl)
	adminTX := storage.NewMockReadOnlyAdminTX(ctrl)
	admin.EXPECT().Snapshot(gomock.Any()).AnyTimes().Return(adminTX, nil)
	adminTX.EXPECT().GetTree(gomock.Any(), logTree.TreeId).AnyTimes().Return(&logTree, nil)
	adminTX.EXPECT().GetTree(gomock.Any(), mapTree.TreeId).AnyTimes().Return(&mapTree, nil)
	adminTX.EXPECT().GetTree(gomock.Any(), unknownTreeID).AnyTimes().Return(nil, errors.New("not found"))
	adminTX.EXPECT().Close().AnyTimes().Return(nil)
	adminTX.EXPECT().Commit().AnyTimes().Return(nil)

	tests := []struct {
		desc       string
		req        interface{}
		fullMethod string
		handlerErr error
		wantErr    bool
		wantTree   *trillian.Tree
	}{
		{
			desc:       "rpcWithoutTree",
			req:        &trillian.CreateTreeRequest{},
			fullMethod: "/trillian.TrillianAdmin/CreateTree",
		},
		{
			desc:       "adminRPC",
			req:        &trillian.GetTreeRequest{TreeId: logTree.TreeId},
			fullMethod: "/trillian.TrillianAdmin/GetTree",
			wantTree:   &logTree,
		},
		{
			desc:       "logRPC",
			req:        &trillian.GetLatestSignedLogRootRequest{LogId: logTree.TreeId},
			fullMethod: "/trillian.TrillianLog/GetLatestSignedLogRoot",
			wantTree:   &logTree,
		},
		{
			desc:       "mapRPC",
			req:        &trillian.GetSignedMapRootRequest{MapId: mapTree.TreeId},
			fullMethod: "/trillian.TrillianMap/GetSignedMapRoot",
			wantTree:   &mapTree,
		},
		{
			desc:       "unknownRequest",
			req:        "not-a-request",
			fullMethod: "/trillian.TrillianLog/UnmappedRequest",
			wantErr:    true,
		},
		{
			desc:       "unknownTree",
			req:        &trillian.GetTreeRequest{TreeId: unknownTreeID},
			fullMethod: "/trillian.TrillianAdmin/GetTree",
			wantErr:    true,
		},
	}

	ctx := context.Background()
	intercept := TreeInterceptor{Admin: admin}
	for _, test := range tests {
		handler := &fakeHandler{resp: "handler response", err: test.handlerErr}

		resp, err := intercept.UnaryInterceptor(ctx, test.req, &grpc.UnaryServerInfo{FullMethod: test.fullMethod}, handler.run)
		if hasErr := err != nil && err != test.handlerErr; hasErr != test.wantErr {
			t.Errorf("%v: UnaryInterceptor() returned err = %q, wantErr = %v", test.desc, err, test.wantErr)
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

func TestGetRPCInfo(t *testing.T) {
	tests := []struct {
		desc                              string
		req                               interface{}
		fullMethod                        string
		wantID                            int64
		wantType                          trillian.TreeType
		wantNoTree, wantReadonly, wantErr bool
	}{
		{
			desc:       "noTree1",
			req:        &trillian.CreateTreeRequest{},
			fullMethod: "/trillian.TrillianAdmin/CreateTree",
			wantNoTree: true,
		},
		{
			desc:       "noTree2",
			req:        &trillian.ListTreesRequest{},
			fullMethod: "/trillian.TrillianAdmin/ListTrees",
			wantNoTree: true,
		},
		{
			desc:         "getAdminRequest",
			req:          &trillian.GetTreeRequest{TreeId: 10},
			fullMethod:   "/trillian.TrillianAdmin/GetTree",
			wantID:       10,
			wantReadonly: true,
		},
		{
			desc:       "rwTreeIDAdminRequest",
			req:        &trillian.DeleteTreeRequest{TreeId: 10},
			fullMethod: "/trillian.TrillianAdmin/DeleteTree",
			wantID:     10,
		},
		{
			desc:       "rwTreeAdminRequest",
			req:        &trillian.UpdateTreeRequest{Tree: &trillian.Tree{TreeId: 10}},
			fullMethod: "/trillian.TrillianAdmin/UpdateTree",
			wantID:     10,
		},
		{
			desc:         "getLogRequest",
			req:          &trillian.GetConsistencyProofRequest{LogId: 20},
			fullMethod:   "/trillian.TrillianLog/GetConsistencyProof",
			wantID:       20,
			wantType:     trillian.TreeType_LOG,
			wantReadonly: true,
		},
		{
			desc:       "rwLogRequest",
			req:        &trillian.QueueLeafRequest{LogId: 20},
			fullMethod: "/trillian.TrillianLog/QueueLeaf",
			wantID:     20,
			wantType:   trillian.TreeType_LOG,
		},
		{
			desc:         "getMapRequest",
			req:          &trillian.GetMapLeavesRequest{MapId: 30},
			fullMethod:   "/trillian.TrillianMap/GetMapLeaves",
			wantID:       30,
			wantType:     trillian.TreeType_MAP,
			wantReadonly: true,
		},
		{
			desc:       "rwMapRequest",
			req:        &trillian.SetMapLeavesRequest{MapId: 30},
			fullMethod: "/trillian.TrillianMap/SetMapLeaves",
			wantID:     30,
			wantType:   trillian.TreeType_MAP,
		},
		{
			desc:       "unknownRequestType",
			req:        "not-a-request",
			fullMethod: "/trillian.TrillianAdmin/GetTree",
			wantErr:    true,
		},
		{
			desc:       "malformedFullMethod",
			req:        &trillian.GetTreeRequest{TreeId: 40},
			fullMethod: "not-a-full-method",
			wantErr:    true,
		},
		{
			desc:       "unknownServiceName",
			req:        &trillian.GetTreeRequest{TreeId: 40},
			fullMethod: "/trillian.FooService/GetTree",
			wantErr:    true,
		},
	}
	for _, test := range tests {
		info, err := getRPCInfo(test.req, test.fullMethod)
		if hasErr := err != nil; hasErr != test.wantErr {
			t.Errorf("%v: getRPCInfo(%T) returned err = %v, wantErr = %v", test.desc, test.req, err, test.wantErr)
			continue
		} else if hasErr {
			continue
		}
		if got, want := info.doesNotHaveTree, test.wantNoTree; got != want {
			t.Errorf("%v: info.doesNotHaveTree = %v, want = %v", test.desc, got, want)
		}
		if got, want := info.treeID, test.wantID; got != want {
			t.Errorf("%v: info.treeID = %v, want = %v", test.desc, got, want)
		}
		wantOpts := &trees.GetOpts{TreeType: test.wantType, Readonly: test.wantReadonly}
		if diff := pretty.Compare(info.opts, wantOpts); diff != "" {
			t.Errorf("%v: info.opts diff:\n%v", test.desc, diff)
		}
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

func TestWrapErrors(t *testing.T) {
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
		i := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
			return test.resp, test.err
		}
		resp, err := WrapErrors(i)(ctx, "req", &grpc.UnaryServerInfo{}, nil /* handler */)
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
