// Automatically generated by MockGen. DO NOT EDIT!
// Source: github.com/google/trillian (interfaces: TrillianLogClient,TrillianLogServer,TrillianMapClient,TrillianMapServer)

package trillian

import (
	gomock "github.com/golang/mock/gomock"
	context "golang.org/x/net/context"
	grpc "google.golang.org/grpc"
)

// Mock of TrillianLogClient interface
type MockTrillianLogClient struct {
	ctrl     *gomock.Controller
	recorder *_MockTrillianLogClientRecorder
}

// Recorder for MockTrillianLogClient (not exported)
type _MockTrillianLogClientRecorder struct {
	mock *MockTrillianLogClient
}

func NewMockTrillianLogClient(ctrl *gomock.Controller) *MockTrillianLogClient {
	mock := &MockTrillianLogClient{ctrl: ctrl}
	mock.recorder = &_MockTrillianLogClientRecorder{mock}
	return mock
}

func (_m *MockTrillianLogClient) EXPECT() *_MockTrillianLogClientRecorder {
	return _m.recorder
}

func (_m *MockTrillianLogClient) GetConsistencyProof(_param0 context.Context, _param1 *GetConsistencyProofRequest, _param2 ...grpc.CallOption) (*GetConsistencyProofResponse, error) {
	_s := []interface{}{_param0, _param1}
	for _, _x := range _param2 {
		_s = append(_s, _x)
	}
	ret := _m.ctrl.Call(_m, "GetConsistencyProof", _s...)
	ret0, _ := ret[0].(*GetConsistencyProofResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (_mr *_MockTrillianLogClientRecorder) GetConsistencyProof(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	_s := append([]interface{}{arg0, arg1}, arg2...)
	return _mr.mock.ctrl.RecordCall(_mr.mock, "GetConsistencyProof", _s...)
}

func (_m *MockTrillianLogClient) GetInclusionProof(_param0 context.Context, _param1 *GetInclusionProofRequest, _param2 ...grpc.CallOption) (*GetInclusionProofResponse, error) {
	_s := []interface{}{_param0, _param1}
	for _, _x := range _param2 {
		_s = append(_s, _x)
	}
	ret := _m.ctrl.Call(_m, "GetInclusionProof", _s...)
	ret0, _ := ret[0].(*GetInclusionProofResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (_mr *_MockTrillianLogClientRecorder) GetInclusionProof(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	_s := append([]interface{}{arg0, arg1}, arg2...)
	return _mr.mock.ctrl.RecordCall(_mr.mock, "GetInclusionProof", _s...)
}

func (_m *MockTrillianLogClient) GetInclusionProofByHash(_param0 context.Context, _param1 *GetInclusionProofByHashRequest, _param2 ...grpc.CallOption) (*GetInclusionProofByHashResponse, error) {
	_s := []interface{}{_param0, _param1}
	for _, _x := range _param2 {
		_s = append(_s, _x)
	}
	ret := _m.ctrl.Call(_m, "GetInclusionProofByHash", _s...)
	ret0, _ := ret[0].(*GetInclusionProofByHashResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (_mr *_MockTrillianLogClientRecorder) GetInclusionProofByHash(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	_s := append([]interface{}{arg0, arg1}, arg2...)
	return _mr.mock.ctrl.RecordCall(_mr.mock, "GetInclusionProofByHash", _s...)
}

func (_m *MockTrillianLogClient) GetLatestSignedLogRoot(_param0 context.Context, _param1 *GetLatestSignedLogRootRequest, _param2 ...grpc.CallOption) (*GetLatestSignedLogRootResponse, error) {
	_s := []interface{}{_param0, _param1}
	for _, _x := range _param2 {
		_s = append(_s, _x)
	}
	ret := _m.ctrl.Call(_m, "GetLatestSignedLogRoot", _s...)
	ret0, _ := ret[0].(*GetLatestSignedLogRootResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (_mr *_MockTrillianLogClientRecorder) GetLatestSignedLogRoot(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	_s := append([]interface{}{arg0, arg1}, arg2...)
	return _mr.mock.ctrl.RecordCall(_mr.mock, "GetLatestSignedLogRoot", _s...)
}

func (_m *MockTrillianLogClient) GetLeavesByHash(_param0 context.Context, _param1 *GetLeavesByHashRequest, _param2 ...grpc.CallOption) (*GetLeavesByHashResponse, error) {
	_s := []interface{}{_param0, _param1}
	for _, _x := range _param2 {
		_s = append(_s, _x)
	}
	ret := _m.ctrl.Call(_m, "GetLeavesByHash", _s...)
	ret0, _ := ret[0].(*GetLeavesByHashResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (_mr *_MockTrillianLogClientRecorder) GetLeavesByHash(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	_s := append([]interface{}{arg0, arg1}, arg2...)
	return _mr.mock.ctrl.RecordCall(_mr.mock, "GetLeavesByHash", _s...)
}

func (_m *MockTrillianLogClient) GetLeavesByIndex(_param0 context.Context, _param1 *GetLeavesByIndexRequest, _param2 ...grpc.CallOption) (*GetLeavesByIndexResponse, error) {
	_s := []interface{}{_param0, _param1}
	for _, _x := range _param2 {
		_s = append(_s, _x)
	}
	ret := _m.ctrl.Call(_m, "GetLeavesByIndex", _s...)
	ret0, _ := ret[0].(*GetLeavesByIndexResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (_mr *_MockTrillianLogClientRecorder) GetLeavesByIndex(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	_s := append([]interface{}{arg0, arg1}, arg2...)
	return _mr.mock.ctrl.RecordCall(_mr.mock, "GetLeavesByIndex", _s...)
}

func (_m *MockTrillianLogClient) GetSequencedLeafCount(_param0 context.Context, _param1 *GetSequencedLeafCountRequest, _param2 ...grpc.CallOption) (*GetSequencedLeafCountResponse, error) {
	_s := []interface{}{_param0, _param1}
	for _, _x := range _param2 {
		_s = append(_s, _x)
	}
	ret := _m.ctrl.Call(_m, "GetSequencedLeafCount", _s...)
	ret0, _ := ret[0].(*GetSequencedLeafCountResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (_mr *_MockTrillianLogClientRecorder) GetSequencedLeafCount(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	_s := append([]interface{}{arg0, arg1}, arg2...)
	return _mr.mock.ctrl.RecordCall(_mr.mock, "GetSequencedLeafCount", _s...)
}

func (_m *MockTrillianLogClient) QueueLeaves(_param0 context.Context, _param1 *QueueLeavesRequest, _param2 ...grpc.CallOption) (*QueueLeavesResponse, error) {
	_s := []interface{}{_param0, _param1}
	for _, _x := range _param2 {
		_s = append(_s, _x)
	}
	ret := _m.ctrl.Call(_m, "QueueLeaves", _s...)
	ret0, _ := ret[0].(*QueueLeavesResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (_mr *_MockTrillianLogClientRecorder) QueueLeaves(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	_s := append([]interface{}{arg0, arg1}, arg2...)
	return _mr.mock.ctrl.RecordCall(_mr.mock, "QueueLeaves", _s...)
}

// Mock of TrillianLogServer interface
type MockTrillianLogServer struct {
	ctrl     *gomock.Controller
	recorder *_MockTrillianLogServerRecorder
}

// Recorder for MockTrillianLogServer (not exported)
type _MockTrillianLogServerRecorder struct {
	mock *MockTrillianLogServer
}

func NewMockTrillianLogServer(ctrl *gomock.Controller) *MockTrillianLogServer {
	mock := &MockTrillianLogServer{ctrl: ctrl}
	mock.recorder = &_MockTrillianLogServerRecorder{mock}
	return mock
}

func (_m *MockTrillianLogServer) EXPECT() *_MockTrillianLogServerRecorder {
	return _m.recorder
}

func (_m *MockTrillianLogServer) GetConsistencyProof(_param0 context.Context, _param1 *GetConsistencyProofRequest) (*GetConsistencyProofResponse, error) {
	ret := _m.ctrl.Call(_m, "GetConsistencyProof", _param0, _param1)
	ret0, _ := ret[0].(*GetConsistencyProofResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (_mr *_MockTrillianLogServerRecorder) GetConsistencyProof(arg0, arg1 interface{}) *gomock.Call {
	return _mr.mock.ctrl.RecordCall(_mr.mock, "GetConsistencyProof", arg0, arg1)
}

func (_m *MockTrillianLogServer) GetInclusionProof(_param0 context.Context, _param1 *GetInclusionProofRequest) (*GetInclusionProofResponse, error) {
	ret := _m.ctrl.Call(_m, "GetInclusionProof", _param0, _param1)
	ret0, _ := ret[0].(*GetInclusionProofResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (_mr *_MockTrillianLogServerRecorder) GetInclusionProof(arg0, arg1 interface{}) *gomock.Call {
	return _mr.mock.ctrl.RecordCall(_mr.mock, "GetInclusionProof", arg0, arg1)
}

func (_m *MockTrillianLogServer) GetInclusionProofByHash(_param0 context.Context, _param1 *GetInclusionProofByHashRequest) (*GetInclusionProofByHashResponse, error) {
	ret := _m.ctrl.Call(_m, "GetInclusionProofByHash", _param0, _param1)
	ret0, _ := ret[0].(*GetInclusionProofByHashResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (_mr *_MockTrillianLogServerRecorder) GetInclusionProofByHash(arg0, arg1 interface{}) *gomock.Call {
	return _mr.mock.ctrl.RecordCall(_mr.mock, "GetInclusionProofByHash", arg0, arg1)
}

func (_m *MockTrillianLogServer) GetLatestSignedLogRoot(_param0 context.Context, _param1 *GetLatestSignedLogRootRequest) (*GetLatestSignedLogRootResponse, error) {
	ret := _m.ctrl.Call(_m, "GetLatestSignedLogRoot", _param0, _param1)
	ret0, _ := ret[0].(*GetLatestSignedLogRootResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (_mr *_MockTrillianLogServerRecorder) GetLatestSignedLogRoot(arg0, arg1 interface{}) *gomock.Call {
	return _mr.mock.ctrl.RecordCall(_mr.mock, "GetLatestSignedLogRoot", arg0, arg1)
}

func (_m *MockTrillianLogServer) GetLeavesByHash(_param0 context.Context, _param1 *GetLeavesByHashRequest) (*GetLeavesByHashResponse, error) {
	ret := _m.ctrl.Call(_m, "GetLeavesByHash", _param0, _param1)
	ret0, _ := ret[0].(*GetLeavesByHashResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (_mr *_MockTrillianLogServerRecorder) GetLeavesByHash(arg0, arg1 interface{}) *gomock.Call {
	return _mr.mock.ctrl.RecordCall(_mr.mock, "GetLeavesByHash", arg0, arg1)
}

func (_m *MockTrillianLogServer) GetLeavesByIndex(_param0 context.Context, _param1 *GetLeavesByIndexRequest) (*GetLeavesByIndexResponse, error) {
	ret := _m.ctrl.Call(_m, "GetLeavesByIndex", _param0, _param1)
	ret0, _ := ret[0].(*GetLeavesByIndexResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (_mr *_MockTrillianLogServerRecorder) GetLeavesByIndex(arg0, arg1 interface{}) *gomock.Call {
	return _mr.mock.ctrl.RecordCall(_mr.mock, "GetLeavesByIndex", arg0, arg1)
}

func (_m *MockTrillianLogServer) GetSequencedLeafCount(_param0 context.Context, _param1 *GetSequencedLeafCountRequest) (*GetSequencedLeafCountResponse, error) {
	ret := _m.ctrl.Call(_m, "GetSequencedLeafCount", _param0, _param1)
	ret0, _ := ret[0].(*GetSequencedLeafCountResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (_mr *_MockTrillianLogServerRecorder) GetSequencedLeafCount(arg0, arg1 interface{}) *gomock.Call {
	return _mr.mock.ctrl.RecordCall(_mr.mock, "GetSequencedLeafCount", arg0, arg1)
}

func (_m *MockTrillianLogServer) QueueLeaves(_param0 context.Context, _param1 *QueueLeavesRequest) (*QueueLeavesResponse, error) {
	ret := _m.ctrl.Call(_m, "QueueLeaves", _param0, _param1)
	ret0, _ := ret[0].(*QueueLeavesResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (_mr *_MockTrillianLogServerRecorder) QueueLeaves(arg0, arg1 interface{}) *gomock.Call {
	return _mr.mock.ctrl.RecordCall(_mr.mock, "QueueLeaves", arg0, arg1)
}

// Mock of TrillianMapClient interface
type MockTrillianMapClient struct {
	ctrl     *gomock.Controller
	recorder *_MockTrillianMapClientRecorder
}

// Recorder for MockTrillianMapClient (not exported)
type _MockTrillianMapClientRecorder struct {
	mock *MockTrillianMapClient
}

func NewMockTrillianMapClient(ctrl *gomock.Controller) *MockTrillianMapClient {
	mock := &MockTrillianMapClient{ctrl: ctrl}
	mock.recorder = &_MockTrillianMapClientRecorder{mock}
	return mock
}

func (_m *MockTrillianMapClient) EXPECT() *_MockTrillianMapClientRecorder {
	return _m.recorder
}

func (_m *MockTrillianMapClient) GetLeaves(_param0 context.Context, _param1 *GetMapLeavesRequest, _param2 ...grpc.CallOption) (*GetMapLeavesResponse, error) {
	_s := []interface{}{_param0, _param1}
	for _, _x := range _param2 {
		_s = append(_s, _x)
	}
	ret := _m.ctrl.Call(_m, "GetLeaves", _s...)
	ret0, _ := ret[0].(*GetMapLeavesResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (_mr *_MockTrillianMapClientRecorder) GetLeaves(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	_s := append([]interface{}{arg0, arg1}, arg2...)
	return _mr.mock.ctrl.RecordCall(_mr.mock, "GetLeaves", _s...)
}

func (_m *MockTrillianMapClient) GetSignedMapRoot(_param0 context.Context, _param1 *GetSignedMapRootRequest, _param2 ...grpc.CallOption) (*GetSignedMapRootResponse, error) {
	_s := []interface{}{_param0, _param1}
	for _, _x := range _param2 {
		_s = append(_s, _x)
	}
	ret := _m.ctrl.Call(_m, "GetSignedMapRoot", _s...)
	ret0, _ := ret[0].(*GetSignedMapRootResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (_mr *_MockTrillianMapClientRecorder) GetSignedMapRoot(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	_s := append([]interface{}{arg0, arg1}, arg2...)
	return _mr.mock.ctrl.RecordCall(_mr.mock, "GetSignedMapRoot", _s...)
}

func (_m *MockTrillianMapClient) SetLeaves(_param0 context.Context, _param1 *SetMapLeavesRequest, _param2 ...grpc.CallOption) (*SetMapLeavesResponse, error) {
	_s := []interface{}{_param0, _param1}
	for _, _x := range _param2 {
		_s = append(_s, _x)
	}
	ret := _m.ctrl.Call(_m, "SetLeaves", _s...)
	ret0, _ := ret[0].(*SetMapLeavesResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (_mr *_MockTrillianMapClientRecorder) SetLeaves(arg0, arg1 interface{}, arg2 ...interface{}) *gomock.Call {
	_s := append([]interface{}{arg0, arg1}, arg2...)
	return _mr.mock.ctrl.RecordCall(_mr.mock, "SetLeaves", _s...)
}

// Mock of TrillianMapServer interface
type MockTrillianMapServer struct {
	ctrl     *gomock.Controller
	recorder *_MockTrillianMapServerRecorder
}

// Recorder for MockTrillianMapServer (not exported)
type _MockTrillianMapServerRecorder struct {
	mock *MockTrillianMapServer
}

func NewMockTrillianMapServer(ctrl *gomock.Controller) *MockTrillianMapServer {
	mock := &MockTrillianMapServer{ctrl: ctrl}
	mock.recorder = &_MockTrillianMapServerRecorder{mock}
	return mock
}

func (_m *MockTrillianMapServer) EXPECT() *_MockTrillianMapServerRecorder {
	return _m.recorder
}

func (_m *MockTrillianMapServer) GetLeaves(_param0 context.Context, _param1 *GetMapLeavesRequest) (*GetMapLeavesResponse, error) {
	ret := _m.ctrl.Call(_m, "GetLeaves", _param0, _param1)
	ret0, _ := ret[0].(*GetMapLeavesResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (_mr *_MockTrillianMapServerRecorder) GetLeaves(arg0, arg1 interface{}) *gomock.Call {
	return _mr.mock.ctrl.RecordCall(_mr.mock, "GetLeaves", arg0, arg1)
}

func (_m *MockTrillianMapServer) GetSignedMapRoot(_param0 context.Context, _param1 *GetSignedMapRootRequest) (*GetSignedMapRootResponse, error) {
	ret := _m.ctrl.Call(_m, "GetSignedMapRoot", _param0, _param1)
	ret0, _ := ret[0].(*GetSignedMapRootResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (_mr *_MockTrillianMapServerRecorder) GetSignedMapRoot(arg0, arg1 interface{}) *gomock.Call {
	return _mr.mock.ctrl.RecordCall(_mr.mock, "GetSignedMapRoot", arg0, arg1)
}

func (_m *MockTrillianMapServer) SetLeaves(_param0 context.Context, _param1 *SetMapLeavesRequest) (*SetMapLeavesResponse, error) {
	ret := _m.ctrl.Call(_m, "SetLeaves", _param0, _param1)
	ret0, _ := ret[0].(*SetMapLeavesResponse)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (_mr *_MockTrillianMapServerRecorder) SetLeaves(arg0, arg1 interface{}) *gomock.Call {
	return _mr.mock.ctrl.RecordCall(_mr.mock, "SetLeaves", arg0, arg1)
}
