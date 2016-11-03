// Automatically generated by MockGen. DO NOT EDIT!
// Source: github.com/google/trillian/server (interfaces: LogOperation)

package server

import (
	gomock "github.com/golang/mock/gomock"
)

// Mock of LogOperation interface
type MockLogOperation struct {
	ctrl     *gomock.Controller
	recorder *_MockLogOperationRecorder
}

// Recorder for MockLogOperation (not exported)
type _MockLogOperationRecorder struct {
	mock *MockLogOperation
}

func NewMockLogOperation(ctrl *gomock.Controller) *MockLogOperation {
	mock := &MockLogOperation{ctrl: ctrl}
	mock.recorder = &_MockLogOperationRecorder{mock}
	return mock
}

func (_m *MockLogOperation) EXPECT() *_MockLogOperationRecorder {
	return _m.recorder
}

func (_m *MockLogOperation) ExecutePass(_param0 []int64, _param1 LogOperationManagerContext) bool {
	ret := _m.ctrl.Call(_m, "ExecutePass", _param0, _param1)
	ret0, _ := ret[0].(bool)
	return ret0
}

func (_mr *_MockLogOperationRecorder) ExecutePass(arg0, arg1 interface{}) *gomock.Call {
	return _mr.mock.ctrl.RecordCall(_mr.mock, "ExecutePass", arg0, arg1)
}

func (_m *MockLogOperation) Name() string {
	ret := _m.ctrl.Call(_m, "Name")
	ret0, _ := ret[0].(string)
	return ret0
}

func (_mr *_MockLogOperationRecorder) Name() *gomock.Call {
	return _mr.mock.ctrl.RecordCall(_mr.mock, "Name")
}
