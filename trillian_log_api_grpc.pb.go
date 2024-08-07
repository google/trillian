// Copyright 2016 Google LLC. All Rights Reserved.
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

// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.5.1
// - protoc             v3.20.1
// source: trillian_log_api.proto

package trillian

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.64.0 or later.
const _ = grpc.SupportPackageIsVersion9

const (
	TrillianLog_QueueLeaf_FullMethodName               = "/trillian.TrillianLog/QueueLeaf"
	TrillianLog_GetInclusionProof_FullMethodName       = "/trillian.TrillianLog/GetInclusionProof"
	TrillianLog_GetInclusionProofByHash_FullMethodName = "/trillian.TrillianLog/GetInclusionProofByHash"
	TrillianLog_GetConsistencyProof_FullMethodName     = "/trillian.TrillianLog/GetConsistencyProof"
	TrillianLog_GetLatestSignedLogRoot_FullMethodName  = "/trillian.TrillianLog/GetLatestSignedLogRoot"
	TrillianLog_GetEntryAndProof_FullMethodName        = "/trillian.TrillianLog/GetEntryAndProof"
	TrillianLog_InitLog_FullMethodName                 = "/trillian.TrillianLog/InitLog"
	TrillianLog_AddSequencedLeaves_FullMethodName      = "/trillian.TrillianLog/AddSequencedLeaves"
	TrillianLog_GetLeavesByRange_FullMethodName        = "/trillian.TrillianLog/GetLeavesByRange"
)

// TrillianLogClient is the client API for TrillianLog service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
//
// The TrillianLog service provides access to an append-only Log data structure
// as described in the [Verifiable Data
// Structures](docs/papers/VerifiableDataStructures.pdf) paper.
//
// The API supports adding new entries to the Merkle tree for a specific Log
// instance (identified by its log_id) in two modes:
//   - For a normal log, new leaf entries are queued up for subsequent
//     inclusion in the log, and the leaves are assigned consecutive leaf_index
//     values as part of that integration process.
//   - For a 'pre-ordered log', new entries have an already-defined leaf
//     ordering, and leaves are only integrated into the Merkle tree when a
//     contiguous range of leaves is available.
//
// The API also supports read operations to retrieve leaf contents, and to
// provide cryptographic proofs of leaf inclusion and of the append-only nature
// of the Log.
//
// Each API request also includes a charge_to field, which allows API users
// to provide quota identifiers that should be "charged" for each API request
// (and potentially rejected with codes.ResourceExhausted).
//
// Various operations on the API also allows for 'server skew', which can occur
// when different API requests happen to be handled by different server instances
// that may not all be up to date.  An API request that is relative to a specific
// tree size may reach a server instance that is not yet aware of this tree size;
// in this case the server will typically return an OK response that contains:
//   - a signed log root that indicates the tree size that it is aware of
//   - an empty response otherwise.
type TrillianLogClient interface {
	// QueueLeaf adds a single leaf to the queue of pending leaves for a normal
	// log.
	QueueLeaf(ctx context.Context, in *QueueLeafRequest, opts ...grpc.CallOption) (*QueueLeafResponse, error)
	// GetInclusionProof returns an inclusion proof for a leaf with a given index
	// in a particular tree.
	//
	// If the requested tree_size is larger than the server is aware of, the
	// response will include the latest known log root and an empty proof.
	GetInclusionProof(ctx context.Context, in *GetInclusionProofRequest, opts ...grpc.CallOption) (*GetInclusionProofResponse, error)
	// GetInclusionProofByHash returns an inclusion proof for any leaves that have
	// the given Merkle hash in a particular tree.
	//
	// If any of the leaves that match the given Merkle has have a leaf index that
	// is beyond the requested tree size, the corresponding proof entry will be empty.
	GetInclusionProofByHash(ctx context.Context, in *GetInclusionProofByHashRequest, opts ...grpc.CallOption) (*GetInclusionProofByHashResponse, error)
	// GetConsistencyProof returns a consistency proof between different sizes of
	// a particular tree.
	//
	// If the requested tree size is larger than the server is aware of,
	// the response will include the latest known log root and an empty proof.
	GetConsistencyProof(ctx context.Context, in *GetConsistencyProofRequest, opts ...grpc.CallOption) (*GetConsistencyProofResponse, error)
	// GetLatestSignedLogRoot returns the latest log root for a given tree,
	// and optionally also includes a consistency proof from an earlier tree size
	// to the new size of the tree.
	//
	// If the earlier tree size is larger than the server is aware of,
	// an InvalidArgument error is returned.
	GetLatestSignedLogRoot(ctx context.Context, in *GetLatestSignedLogRootRequest, opts ...grpc.CallOption) (*GetLatestSignedLogRootResponse, error)
	// GetEntryAndProof returns a log leaf and the corresponding inclusion proof
	// to a specified tree size, for a given leaf index in a particular tree.
	//
	// If the requested tree size is unavailable but the leaf is
	// in scope for the current tree, the returned proof will be for the
	// current tree size rather than the requested tree size.
	GetEntryAndProof(ctx context.Context, in *GetEntryAndProofRequest, opts ...grpc.CallOption) (*GetEntryAndProofResponse, error)
	// InitLog initializes a particular tree, creating the initial signed log
	// root (which will be of size 0).
	InitLog(ctx context.Context, in *InitLogRequest, opts ...grpc.CallOption) (*InitLogResponse, error)
	// AddSequencedLeaves adds a batch of leaves with assigned sequence numbers
	// to a pre-ordered log.  The indices of the provided leaves must be contiguous.
	AddSequencedLeaves(ctx context.Context, in *AddSequencedLeavesRequest, opts ...grpc.CallOption) (*AddSequencedLeavesResponse, error)
	// GetLeavesByRange returns a batch of leaves whose leaf indices are in a
	// sequential range.
	GetLeavesByRange(ctx context.Context, in *GetLeavesByRangeRequest, opts ...grpc.CallOption) (*GetLeavesByRangeResponse, error)
}

type trillianLogClient struct {
	cc grpc.ClientConnInterface
}

func NewTrillianLogClient(cc grpc.ClientConnInterface) TrillianLogClient {
	return &trillianLogClient{cc}
}

func (c *trillianLogClient) QueueLeaf(ctx context.Context, in *QueueLeafRequest, opts ...grpc.CallOption) (*QueueLeafResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(QueueLeafResponse)
	err := c.cc.Invoke(ctx, TrillianLog_QueueLeaf_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *trillianLogClient) GetInclusionProof(ctx context.Context, in *GetInclusionProofRequest, opts ...grpc.CallOption) (*GetInclusionProofResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetInclusionProofResponse)
	err := c.cc.Invoke(ctx, TrillianLog_GetInclusionProof_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *trillianLogClient) GetInclusionProofByHash(ctx context.Context, in *GetInclusionProofByHashRequest, opts ...grpc.CallOption) (*GetInclusionProofByHashResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetInclusionProofByHashResponse)
	err := c.cc.Invoke(ctx, TrillianLog_GetInclusionProofByHash_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *trillianLogClient) GetConsistencyProof(ctx context.Context, in *GetConsistencyProofRequest, opts ...grpc.CallOption) (*GetConsistencyProofResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetConsistencyProofResponse)
	err := c.cc.Invoke(ctx, TrillianLog_GetConsistencyProof_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *trillianLogClient) GetLatestSignedLogRoot(ctx context.Context, in *GetLatestSignedLogRootRequest, opts ...grpc.CallOption) (*GetLatestSignedLogRootResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetLatestSignedLogRootResponse)
	err := c.cc.Invoke(ctx, TrillianLog_GetLatestSignedLogRoot_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *trillianLogClient) GetEntryAndProof(ctx context.Context, in *GetEntryAndProofRequest, opts ...grpc.CallOption) (*GetEntryAndProofResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetEntryAndProofResponse)
	err := c.cc.Invoke(ctx, TrillianLog_GetEntryAndProof_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *trillianLogClient) InitLog(ctx context.Context, in *InitLogRequest, opts ...grpc.CallOption) (*InitLogResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(InitLogResponse)
	err := c.cc.Invoke(ctx, TrillianLog_InitLog_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *trillianLogClient) AddSequencedLeaves(ctx context.Context, in *AddSequencedLeavesRequest, opts ...grpc.CallOption) (*AddSequencedLeavesResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(AddSequencedLeavesResponse)
	err := c.cc.Invoke(ctx, TrillianLog_AddSequencedLeaves_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *trillianLogClient) GetLeavesByRange(ctx context.Context, in *GetLeavesByRangeRequest, opts ...grpc.CallOption) (*GetLeavesByRangeResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetLeavesByRangeResponse)
	err := c.cc.Invoke(ctx, TrillianLog_GetLeavesByRange_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// TrillianLogServer is the server API for TrillianLog service.
// All implementations should embed UnimplementedTrillianLogServer
// for forward compatibility.
//
// The TrillianLog service provides access to an append-only Log data structure
// as described in the [Verifiable Data
// Structures](docs/papers/VerifiableDataStructures.pdf) paper.
//
// The API supports adding new entries to the Merkle tree for a specific Log
// instance (identified by its log_id) in two modes:
//   - For a normal log, new leaf entries are queued up for subsequent
//     inclusion in the log, and the leaves are assigned consecutive leaf_index
//     values as part of that integration process.
//   - For a 'pre-ordered log', new entries have an already-defined leaf
//     ordering, and leaves are only integrated into the Merkle tree when a
//     contiguous range of leaves is available.
//
// The API also supports read operations to retrieve leaf contents, and to
// provide cryptographic proofs of leaf inclusion and of the append-only nature
// of the Log.
//
// Each API request also includes a charge_to field, which allows API users
// to provide quota identifiers that should be "charged" for each API request
// (and potentially rejected with codes.ResourceExhausted).
//
// Various operations on the API also allows for 'server skew', which can occur
// when different API requests happen to be handled by different server instances
// that may not all be up to date.  An API request that is relative to a specific
// tree size may reach a server instance that is not yet aware of this tree size;
// in this case the server will typically return an OK response that contains:
//   - a signed log root that indicates the tree size that it is aware of
//   - an empty response otherwise.
type TrillianLogServer interface {
	// QueueLeaf adds a single leaf to the queue of pending leaves for a normal
	// log.
	QueueLeaf(context.Context, *QueueLeafRequest) (*QueueLeafResponse, error)
	// GetInclusionProof returns an inclusion proof for a leaf with a given index
	// in a particular tree.
	//
	// If the requested tree_size is larger than the server is aware of, the
	// response will include the latest known log root and an empty proof.
	GetInclusionProof(context.Context, *GetInclusionProofRequest) (*GetInclusionProofResponse, error)
	// GetInclusionProofByHash returns an inclusion proof for any leaves that have
	// the given Merkle hash in a particular tree.
	//
	// If any of the leaves that match the given Merkle has have a leaf index that
	// is beyond the requested tree size, the corresponding proof entry will be empty.
	GetInclusionProofByHash(context.Context, *GetInclusionProofByHashRequest) (*GetInclusionProofByHashResponse, error)
	// GetConsistencyProof returns a consistency proof between different sizes of
	// a particular tree.
	//
	// If the requested tree size is larger than the server is aware of,
	// the response will include the latest known log root and an empty proof.
	GetConsistencyProof(context.Context, *GetConsistencyProofRequest) (*GetConsistencyProofResponse, error)
	// GetLatestSignedLogRoot returns the latest log root for a given tree,
	// and optionally also includes a consistency proof from an earlier tree size
	// to the new size of the tree.
	//
	// If the earlier tree size is larger than the server is aware of,
	// an InvalidArgument error is returned.
	GetLatestSignedLogRoot(context.Context, *GetLatestSignedLogRootRequest) (*GetLatestSignedLogRootResponse, error)
	// GetEntryAndProof returns a log leaf and the corresponding inclusion proof
	// to a specified tree size, for a given leaf index in a particular tree.
	//
	// If the requested tree size is unavailable but the leaf is
	// in scope for the current tree, the returned proof will be for the
	// current tree size rather than the requested tree size.
	GetEntryAndProof(context.Context, *GetEntryAndProofRequest) (*GetEntryAndProofResponse, error)
	// InitLog initializes a particular tree, creating the initial signed log
	// root (which will be of size 0).
	InitLog(context.Context, *InitLogRequest) (*InitLogResponse, error)
	// AddSequencedLeaves adds a batch of leaves with assigned sequence numbers
	// to a pre-ordered log.  The indices of the provided leaves must be contiguous.
	AddSequencedLeaves(context.Context, *AddSequencedLeavesRequest) (*AddSequencedLeavesResponse, error)
	// GetLeavesByRange returns a batch of leaves whose leaf indices are in a
	// sequential range.
	GetLeavesByRange(context.Context, *GetLeavesByRangeRequest) (*GetLeavesByRangeResponse, error)
}

// UnimplementedTrillianLogServer should be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedTrillianLogServer struct{}

func (UnimplementedTrillianLogServer) QueueLeaf(context.Context, *QueueLeafRequest) (*QueueLeafResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method QueueLeaf not implemented")
}
func (UnimplementedTrillianLogServer) GetInclusionProof(context.Context, *GetInclusionProofRequest) (*GetInclusionProofResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetInclusionProof not implemented")
}
func (UnimplementedTrillianLogServer) GetInclusionProofByHash(context.Context, *GetInclusionProofByHashRequest) (*GetInclusionProofByHashResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetInclusionProofByHash not implemented")
}
func (UnimplementedTrillianLogServer) GetConsistencyProof(context.Context, *GetConsistencyProofRequest) (*GetConsistencyProofResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetConsistencyProof not implemented")
}
func (UnimplementedTrillianLogServer) GetLatestSignedLogRoot(context.Context, *GetLatestSignedLogRootRequest) (*GetLatestSignedLogRootResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetLatestSignedLogRoot not implemented")
}
func (UnimplementedTrillianLogServer) GetEntryAndProof(context.Context, *GetEntryAndProofRequest) (*GetEntryAndProofResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetEntryAndProof not implemented")
}
func (UnimplementedTrillianLogServer) InitLog(context.Context, *InitLogRequest) (*InitLogResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method InitLog not implemented")
}
func (UnimplementedTrillianLogServer) AddSequencedLeaves(context.Context, *AddSequencedLeavesRequest) (*AddSequencedLeavesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method AddSequencedLeaves not implemented")
}
func (UnimplementedTrillianLogServer) GetLeavesByRange(context.Context, *GetLeavesByRangeRequest) (*GetLeavesByRangeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetLeavesByRange not implemented")
}
func (UnimplementedTrillianLogServer) testEmbeddedByValue() {}

// UnsafeTrillianLogServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to TrillianLogServer will
// result in compilation errors.
type UnsafeTrillianLogServer interface {
	mustEmbedUnimplementedTrillianLogServer()
}

func RegisterTrillianLogServer(s grpc.ServiceRegistrar, srv TrillianLogServer) {
	// If the following call pancis, it indicates UnimplementedTrillianLogServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&TrillianLog_ServiceDesc, srv)
}

func _TrillianLog_QueueLeaf_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(QueueLeafRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TrillianLogServer).QueueLeaf(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: TrillianLog_QueueLeaf_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TrillianLogServer).QueueLeaf(ctx, req.(*QueueLeafRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _TrillianLog_GetInclusionProof_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetInclusionProofRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TrillianLogServer).GetInclusionProof(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: TrillianLog_GetInclusionProof_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TrillianLogServer).GetInclusionProof(ctx, req.(*GetInclusionProofRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _TrillianLog_GetInclusionProofByHash_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetInclusionProofByHashRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TrillianLogServer).GetInclusionProofByHash(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: TrillianLog_GetInclusionProofByHash_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TrillianLogServer).GetInclusionProofByHash(ctx, req.(*GetInclusionProofByHashRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _TrillianLog_GetConsistencyProof_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetConsistencyProofRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TrillianLogServer).GetConsistencyProof(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: TrillianLog_GetConsistencyProof_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TrillianLogServer).GetConsistencyProof(ctx, req.(*GetConsistencyProofRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _TrillianLog_GetLatestSignedLogRoot_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetLatestSignedLogRootRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TrillianLogServer).GetLatestSignedLogRoot(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: TrillianLog_GetLatestSignedLogRoot_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TrillianLogServer).GetLatestSignedLogRoot(ctx, req.(*GetLatestSignedLogRootRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _TrillianLog_GetEntryAndProof_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetEntryAndProofRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TrillianLogServer).GetEntryAndProof(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: TrillianLog_GetEntryAndProof_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TrillianLogServer).GetEntryAndProof(ctx, req.(*GetEntryAndProofRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _TrillianLog_InitLog_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(InitLogRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TrillianLogServer).InitLog(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: TrillianLog_InitLog_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TrillianLogServer).InitLog(ctx, req.(*InitLogRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _TrillianLog_AddSequencedLeaves_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(AddSequencedLeavesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TrillianLogServer).AddSequencedLeaves(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: TrillianLog_AddSequencedLeaves_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TrillianLogServer).AddSequencedLeaves(ctx, req.(*AddSequencedLeavesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _TrillianLog_GetLeavesByRange_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetLeavesByRangeRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TrillianLogServer).GetLeavesByRange(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: TrillianLog_GetLeavesByRange_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TrillianLogServer).GetLeavesByRange(ctx, req.(*GetLeavesByRangeRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// TrillianLog_ServiceDesc is the grpc.ServiceDesc for TrillianLog service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var TrillianLog_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "trillian.TrillianLog",
	HandlerType: (*TrillianLogServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "QueueLeaf",
			Handler:    _TrillianLog_QueueLeaf_Handler,
		},
		{
			MethodName: "GetInclusionProof",
			Handler:    _TrillianLog_GetInclusionProof_Handler,
		},
		{
			MethodName: "GetInclusionProofByHash",
			Handler:    _TrillianLog_GetInclusionProofByHash_Handler,
		},
		{
			MethodName: "GetConsistencyProof",
			Handler:    _TrillianLog_GetConsistencyProof_Handler,
		},
		{
			MethodName: "GetLatestSignedLogRoot",
			Handler:    _TrillianLog_GetLatestSignedLogRoot_Handler,
		},
		{
			MethodName: "GetEntryAndProof",
			Handler:    _TrillianLog_GetEntryAndProof_Handler,
		},
		{
			MethodName: "InitLog",
			Handler:    _TrillianLog_InitLog_Handler,
		},
		{
			MethodName: "AddSequencedLeaves",
			Handler:    _TrillianLog_AddSequencedLeaves_Handler,
		},
		{
			MethodName: "GetLeavesByRange",
			Handler:    _TrillianLog_GetLeavesByRange_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "trillian_log_api.proto",
}
