// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/google/trillian/crypto/keys (interfaces: SignerFactory)

package keys

import (
	context "context"
	crypto "crypto"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	proto "github.com/golang/protobuf/proto"
	keyspb "github.com/google/trillian/crypto/keyspb"
)

// MockSignerFactory is a mock of SignerFactory interface
type MockSignerFactory struct {
	ctrl     *gomock.Controller
	recorder *MockSignerFactoryMockRecorder
}

// MockSignerFactoryMockRecorder is the mock recorder for MockSignerFactory
type MockSignerFactoryMockRecorder struct {
	mock *MockSignerFactory
}

// NewMockSignerFactory creates a new mock instance
func NewMockSignerFactory(ctrl *gomock.Controller) *MockSignerFactory {
	mock := &MockSignerFactory{ctrl: ctrl}
	mock.recorder = &MockSignerFactoryMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (_m *MockSignerFactory) EXPECT() *MockSignerFactoryMockRecorder {
	return _m.recorder
}

// Generate mocks base method
func (_m *MockSignerFactory) Generate(_param0 context.Context, _param1 *keyspb.Specification) (proto.Message, error) {
	ret := _m.ctrl.Call(_m, "Generate", _param0, _param1)
	ret0, _ := ret[0].(proto.Message)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Generate indicates an expected call of Generate
func (_mr *MockSignerFactoryMockRecorder) Generate(arg0, arg1 interface{}) *gomock.Call {
	return _mr.mock.ctrl.RecordCallWithMethodType(_mr.mock, "Generate", reflect.TypeOf((*MockSignerFactory)(nil).Generate), arg0, arg1)
}

// NewSigner mocks base method
func (_m *MockSignerFactory) NewSigner(_param0 context.Context, _param1 proto.Message) (crypto.Signer, error) {
	ret := _m.ctrl.Call(_m, "NewSigner", _param0, _param1)
	ret0, _ := ret[0].(crypto.Signer)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// NewSigner indicates an expected call of NewSigner
func (_mr *MockSignerFactoryMockRecorder) NewSigner(arg0, arg1 interface{}) *gomock.Call {
	return _mr.mock.ctrl.RecordCallWithMethodType(_mr.mock, "NewSigner", reflect.TypeOf((*MockSignerFactory)(nil).NewSigner), arg0, arg1)
}
