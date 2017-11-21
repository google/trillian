// Code generated by protoc-gen-go. DO NOT EDIT.
// source: trillian_map_api.proto

package trillian

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"
import _ "google.golang.org/genproto/googleapis/api/annotations"
import google_protobuf "github.com/golang/protobuf/ptypes/any"

import (
	context "golang.org/x/net/context"
	grpc "google.golang.org/grpc"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// MapLeaf represents the data behind Map leaves.
type MapLeaf struct {
	// index is the location of this leaf.
	// All indexes for a given Map must contain a constant number of bits.
	Index []byte `protobuf:"bytes,1,opt,name=index,proto3" json:"index,omitempty"`
	// leaf_hash is the tree hash of leaf_value.  This does not need to be set
	// on SetMapLeavesRequest; the server will fill it in.
	LeafHash []byte `protobuf:"bytes,2,opt,name=leaf_hash,json=leafHash,proto3" json:"leaf_hash,omitempty"`
	// leaf_value is the data the tree commits to.
	LeafValue []byte `protobuf:"bytes,3,opt,name=leaf_value,json=leafValue,proto3" json:"leaf_value,omitempty"`
	// extra_data holds related contextual data, but is not covered by any hash.
	ExtraData []byte `protobuf:"bytes,4,opt,name=extra_data,json=extraData,proto3" json:"extra_data,omitempty"`
}

func (m *MapLeaf) Reset()                    { *m = MapLeaf{} }
func (m *MapLeaf) String() string            { return proto.CompactTextString(m) }
func (*MapLeaf) ProtoMessage()               {}
func (*MapLeaf) Descriptor() ([]byte, []int) { return fileDescriptor1, []int{0} }

func (m *MapLeaf) GetIndex() []byte {
	if m != nil {
		return m.Index
	}
	return nil
}

func (m *MapLeaf) GetLeafHash() []byte {
	if m != nil {
		return m.LeafHash
	}
	return nil
}

func (m *MapLeaf) GetLeafValue() []byte {
	if m != nil {
		return m.LeafValue
	}
	return nil
}

func (m *MapLeaf) GetExtraData() []byte {
	if m != nil {
		return m.ExtraData
	}
	return nil
}

type MapLeafInclusion struct {
	Leaf      *MapLeaf `protobuf:"bytes,1,opt,name=leaf" json:"leaf,omitempty"`
	Inclusion [][]byte `protobuf:"bytes,2,rep,name=inclusion,proto3" json:"inclusion,omitempty"`
}

func (m *MapLeafInclusion) Reset()                    { *m = MapLeafInclusion{} }
func (m *MapLeafInclusion) String() string            { return proto.CompactTextString(m) }
func (*MapLeafInclusion) ProtoMessage()               {}
func (*MapLeafInclusion) Descriptor() ([]byte, []int) { return fileDescriptor1, []int{1} }

func (m *MapLeafInclusion) GetLeaf() *MapLeaf {
	if m != nil {
		return m.Leaf
	}
	return nil
}

func (m *MapLeafInclusion) GetInclusion() [][]byte {
	if m != nil {
		return m.Inclusion
	}
	return nil
}

type GetMapLeavesRequest struct {
	MapId int64    `protobuf:"varint,1,opt,name=map_id,json=mapId" json:"map_id,omitempty"`
	Index [][]byte `protobuf:"bytes,2,rep,name=index,proto3" json:"index,omitempty"`
	// A negative revision indicates that the most recent published revision should be used.
	// TODO(phad): 'revision' is deprecated; use GetMapLeavesByRevision instead.
	// When 'revision' is removed GetMapLeaves will only return the most recently
	// published revision.
	Revision int64 `protobuf:"varint,3,opt,name=revision" json:"revision,omitempty"`
}

func (m *GetMapLeavesRequest) Reset()                    { *m = GetMapLeavesRequest{} }
func (m *GetMapLeavesRequest) String() string            { return proto.CompactTextString(m) }
func (*GetMapLeavesRequest) ProtoMessage()               {}
func (*GetMapLeavesRequest) Descriptor() ([]byte, []int) { return fileDescriptor1, []int{2} }

func (m *GetMapLeavesRequest) GetMapId() int64 {
	if m != nil {
		return m.MapId
	}
	return 0
}

func (m *GetMapLeavesRequest) GetIndex() [][]byte {
	if m != nil {
		return m.Index
	}
	return nil
}

func (m *GetMapLeavesRequest) GetRevision() int64 {
	if m != nil {
		return m.Revision
	}
	return 0
}

// This message replaces the current implementation of GetMapLeavesRequest
// with the difference that revision must be >=0.
type GetMapLeavesByRevisionRequest struct {
	MapId    int64    `protobuf:"varint,1,opt,name=map_id,json=mapId" json:"map_id,omitempty"`
	Index    [][]byte `protobuf:"bytes,2,rep,name=index,proto3" json:"index,omitempty"`
	Revision uint64   `protobuf:"varint,3,opt,name=revision" json:"revision,omitempty"`
}

func (m *GetMapLeavesByRevisionRequest) Reset()                    { *m = GetMapLeavesByRevisionRequest{} }
func (m *GetMapLeavesByRevisionRequest) String() string            { return proto.CompactTextString(m) }
func (*GetMapLeavesByRevisionRequest) ProtoMessage()               {}
func (*GetMapLeavesByRevisionRequest) Descriptor() ([]byte, []int) { return fileDescriptor1, []int{3} }

func (m *GetMapLeavesByRevisionRequest) GetMapId() int64 {
	if m != nil {
		return m.MapId
	}
	return 0
}

func (m *GetMapLeavesByRevisionRequest) GetIndex() [][]byte {
	if m != nil {
		return m.Index
	}
	return nil
}

func (m *GetMapLeavesByRevisionRequest) GetRevision() uint64 {
	if m != nil {
		return m.Revision
	}
	return 0
}

type GetMapLeavesResponse struct {
	MapLeafInclusion []*MapLeafInclusion `protobuf:"bytes,2,rep,name=map_leaf_inclusion,json=mapLeafInclusion" json:"map_leaf_inclusion,omitempty"`
	MapRoot          *SignedMapRoot      `protobuf:"bytes,3,opt,name=map_root,json=mapRoot" json:"map_root,omitempty"`
}

func (m *GetMapLeavesResponse) Reset()                    { *m = GetMapLeavesResponse{} }
func (m *GetMapLeavesResponse) String() string            { return proto.CompactTextString(m) }
func (*GetMapLeavesResponse) ProtoMessage()               {}
func (*GetMapLeavesResponse) Descriptor() ([]byte, []int) { return fileDescriptor1, []int{4} }

func (m *GetMapLeavesResponse) GetMapLeafInclusion() []*MapLeafInclusion {
	if m != nil {
		return m.MapLeafInclusion
	}
	return nil
}

func (m *GetMapLeavesResponse) GetMapRoot() *SignedMapRoot {
	if m != nil {
		return m.MapRoot
	}
	return nil
}

type SetMapLeavesRequest struct {
	MapId  int64      `protobuf:"varint,1,opt,name=map_id,json=mapId" json:"map_id,omitempty"`
	Leaves []*MapLeaf `protobuf:"bytes,2,rep,name=leaves" json:"leaves,omitempty"`
	// Metadata that the Map should associate with the new Map root after
	// incorporating the leaf changes.  The metadata will be reflected in the
	// Map Root returned in the map's SetLeaves response.
	// Map personalities should use metadata to persist any state needed later
	// to continue mapping from an external data source.
	Metadata *google_protobuf.Any `protobuf:"bytes,4,opt,name=metadata" json:"metadata,omitempty"`
}

func (m *SetMapLeavesRequest) Reset()                    { *m = SetMapLeavesRequest{} }
func (m *SetMapLeavesRequest) String() string            { return proto.CompactTextString(m) }
func (*SetMapLeavesRequest) ProtoMessage()               {}
func (*SetMapLeavesRequest) Descriptor() ([]byte, []int) { return fileDescriptor1, []int{5} }

func (m *SetMapLeavesRequest) GetMapId() int64 {
	if m != nil {
		return m.MapId
	}
	return 0
}

func (m *SetMapLeavesRequest) GetLeaves() []*MapLeaf {
	if m != nil {
		return m.Leaves
	}
	return nil
}

func (m *SetMapLeavesRequest) GetMetadata() *google_protobuf.Any {
	if m != nil {
		return m.Metadata
	}
	return nil
}

type SetMapLeavesResponse struct {
	MapRoot *SignedMapRoot `protobuf:"bytes,2,opt,name=map_root,json=mapRoot" json:"map_root,omitempty"`
}

func (m *SetMapLeavesResponse) Reset()                    { *m = SetMapLeavesResponse{} }
func (m *SetMapLeavesResponse) String() string            { return proto.CompactTextString(m) }
func (*SetMapLeavesResponse) ProtoMessage()               {}
func (*SetMapLeavesResponse) Descriptor() ([]byte, []int) { return fileDescriptor1, []int{6} }

func (m *SetMapLeavesResponse) GetMapRoot() *SignedMapRoot {
	if m != nil {
		return m.MapRoot
	}
	return nil
}

type GetSignedMapRootRequest struct {
	MapId int64 `protobuf:"varint,1,opt,name=map_id,json=mapId" json:"map_id,omitempty"`
}

func (m *GetSignedMapRootRequest) Reset()                    { *m = GetSignedMapRootRequest{} }
func (m *GetSignedMapRootRequest) String() string            { return proto.CompactTextString(m) }
func (*GetSignedMapRootRequest) ProtoMessage()               {}
func (*GetSignedMapRootRequest) Descriptor() ([]byte, []int) { return fileDescriptor1, []int{7} }

func (m *GetSignedMapRootRequest) GetMapId() int64 {
	if m != nil {
		return m.MapId
	}
	return 0
}

type GetSignedMapRootByRevisionRequest struct {
	MapId    int64 `protobuf:"varint,1,opt,name=map_id,json=mapId" json:"map_id,omitempty"`
	Revision int64 `protobuf:"varint,2,opt,name=revision" json:"revision,omitempty"`
}

func (m *GetSignedMapRootByRevisionRequest) Reset()         { *m = GetSignedMapRootByRevisionRequest{} }
func (m *GetSignedMapRootByRevisionRequest) String() string { return proto.CompactTextString(m) }
func (*GetSignedMapRootByRevisionRequest) ProtoMessage()    {}
func (*GetSignedMapRootByRevisionRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor1, []int{8}
}

func (m *GetSignedMapRootByRevisionRequest) GetMapId() int64 {
	if m != nil {
		return m.MapId
	}
	return 0
}

func (m *GetSignedMapRootByRevisionRequest) GetRevision() int64 {
	if m != nil {
		return m.Revision
	}
	return 0
}

type GetSignedMapRootResponse struct {
	MapRoot *SignedMapRoot `protobuf:"bytes,2,opt,name=map_root,json=mapRoot" json:"map_root,omitempty"`
}

func (m *GetSignedMapRootResponse) Reset()                    { *m = GetSignedMapRootResponse{} }
func (m *GetSignedMapRootResponse) String() string            { return proto.CompactTextString(m) }
func (*GetSignedMapRootResponse) ProtoMessage()               {}
func (*GetSignedMapRootResponse) Descriptor() ([]byte, []int) { return fileDescriptor1, []int{9} }

func (m *GetSignedMapRootResponse) GetMapRoot() *SignedMapRoot {
	if m != nil {
		return m.MapRoot
	}
	return nil
}

func init() {
	proto.RegisterType((*MapLeaf)(nil), "trillian.MapLeaf")
	proto.RegisterType((*MapLeafInclusion)(nil), "trillian.MapLeafInclusion")
	proto.RegisterType((*GetMapLeavesRequest)(nil), "trillian.GetMapLeavesRequest")
	proto.RegisterType((*GetMapLeavesByRevisionRequest)(nil), "trillian.GetMapLeavesByRevisionRequest")
	proto.RegisterType((*GetMapLeavesResponse)(nil), "trillian.GetMapLeavesResponse")
	proto.RegisterType((*SetMapLeavesRequest)(nil), "trillian.SetMapLeavesRequest")
	proto.RegisterType((*SetMapLeavesResponse)(nil), "trillian.SetMapLeavesResponse")
	proto.RegisterType((*GetSignedMapRootRequest)(nil), "trillian.GetSignedMapRootRequest")
	proto.RegisterType((*GetSignedMapRootByRevisionRequest)(nil), "trillian.GetSignedMapRootByRevisionRequest")
	proto.RegisterType((*GetSignedMapRootResponse)(nil), "trillian.GetSignedMapRootResponse")
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion4

// Client API for TrillianMap service

type TrillianMapClient interface {
	// GetLeaves returns an inclusion proof for each index requested.
	// For indexes that do not exist, the inclusion proof will use nil for the empty leaf value.
	GetLeaves(ctx context.Context, in *GetMapLeavesRequest, opts ...grpc.CallOption) (*GetMapLeavesResponse, error)
	GetLeavesByRevision(ctx context.Context, in *GetMapLeavesByRevisionRequest, opts ...grpc.CallOption) (*GetMapLeavesResponse, error)
	SetLeaves(ctx context.Context, in *SetMapLeavesRequest, opts ...grpc.CallOption) (*SetMapLeavesResponse, error)
	GetSignedMapRoot(ctx context.Context, in *GetSignedMapRootRequest, opts ...grpc.CallOption) (*GetSignedMapRootResponse, error)
	GetSignedMapRootByRevision(ctx context.Context, in *GetSignedMapRootByRevisionRequest, opts ...grpc.CallOption) (*GetSignedMapRootResponse, error)
}

type trillianMapClient struct {
	cc *grpc.ClientConn
}

func NewTrillianMapClient(cc *grpc.ClientConn) TrillianMapClient {
	return &trillianMapClient{cc}
}

func (c *trillianMapClient) GetLeaves(ctx context.Context, in *GetMapLeavesRequest, opts ...grpc.CallOption) (*GetMapLeavesResponse, error) {
	out := new(GetMapLeavesResponse)
	err := grpc.Invoke(ctx, "/trillian.TrillianMap/GetLeaves", in, out, c.cc, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *trillianMapClient) GetLeavesByRevision(ctx context.Context, in *GetMapLeavesByRevisionRequest, opts ...grpc.CallOption) (*GetMapLeavesResponse, error) {
	out := new(GetMapLeavesResponse)
	err := grpc.Invoke(ctx, "/trillian.TrillianMap/GetLeavesByRevision", in, out, c.cc, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *trillianMapClient) SetLeaves(ctx context.Context, in *SetMapLeavesRequest, opts ...grpc.CallOption) (*SetMapLeavesResponse, error) {
	out := new(SetMapLeavesResponse)
	err := grpc.Invoke(ctx, "/trillian.TrillianMap/SetLeaves", in, out, c.cc, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *trillianMapClient) GetSignedMapRoot(ctx context.Context, in *GetSignedMapRootRequest, opts ...grpc.CallOption) (*GetSignedMapRootResponse, error) {
	out := new(GetSignedMapRootResponse)
	err := grpc.Invoke(ctx, "/trillian.TrillianMap/GetSignedMapRoot", in, out, c.cc, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *trillianMapClient) GetSignedMapRootByRevision(ctx context.Context, in *GetSignedMapRootByRevisionRequest, opts ...grpc.CallOption) (*GetSignedMapRootResponse, error) {
	out := new(GetSignedMapRootResponse)
	err := grpc.Invoke(ctx, "/trillian.TrillianMap/GetSignedMapRootByRevision", in, out, c.cc, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Server API for TrillianMap service

type TrillianMapServer interface {
	// GetLeaves returns an inclusion proof for each index requested.
	// For indexes that do not exist, the inclusion proof will use nil for the empty leaf value.
	GetLeaves(context.Context, *GetMapLeavesRequest) (*GetMapLeavesResponse, error)
	GetLeavesByRevision(context.Context, *GetMapLeavesByRevisionRequest) (*GetMapLeavesResponse, error)
	SetLeaves(context.Context, *SetMapLeavesRequest) (*SetMapLeavesResponse, error)
	GetSignedMapRoot(context.Context, *GetSignedMapRootRequest) (*GetSignedMapRootResponse, error)
	GetSignedMapRootByRevision(context.Context, *GetSignedMapRootByRevisionRequest) (*GetSignedMapRootResponse, error)
}

func RegisterTrillianMapServer(s *grpc.Server, srv TrillianMapServer) {
	s.RegisterService(&_TrillianMap_serviceDesc, srv)
}

func _TrillianMap_GetLeaves_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetMapLeavesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TrillianMapServer).GetLeaves(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/trillian.TrillianMap/GetLeaves",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TrillianMapServer).GetLeaves(ctx, req.(*GetMapLeavesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _TrillianMap_GetLeavesByRevision_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetMapLeavesByRevisionRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TrillianMapServer).GetLeavesByRevision(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/trillian.TrillianMap/GetLeavesByRevision",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TrillianMapServer).GetLeavesByRevision(ctx, req.(*GetMapLeavesByRevisionRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _TrillianMap_SetLeaves_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SetMapLeavesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TrillianMapServer).SetLeaves(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/trillian.TrillianMap/SetLeaves",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TrillianMapServer).SetLeaves(ctx, req.(*SetMapLeavesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _TrillianMap_GetSignedMapRoot_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetSignedMapRootRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TrillianMapServer).GetSignedMapRoot(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/trillian.TrillianMap/GetSignedMapRoot",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TrillianMapServer).GetSignedMapRoot(ctx, req.(*GetSignedMapRootRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _TrillianMap_GetSignedMapRootByRevision_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetSignedMapRootByRevisionRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TrillianMapServer).GetSignedMapRootByRevision(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/trillian.TrillianMap/GetSignedMapRootByRevision",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TrillianMapServer).GetSignedMapRootByRevision(ctx, req.(*GetSignedMapRootByRevisionRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _TrillianMap_serviceDesc = grpc.ServiceDesc{
	ServiceName: "trillian.TrillianMap",
	HandlerType: (*TrillianMapServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetLeaves",
			Handler:    _TrillianMap_GetLeaves_Handler,
		},
		{
			MethodName: "GetLeavesByRevision",
			Handler:    _TrillianMap_GetLeavesByRevision_Handler,
		},
		{
			MethodName: "SetLeaves",
			Handler:    _TrillianMap_SetLeaves_Handler,
		},
		{
			MethodName: "GetSignedMapRoot",
			Handler:    _TrillianMap_GetSignedMapRoot_Handler,
		},
		{
			MethodName: "GetSignedMapRootByRevision",
			Handler:    _TrillianMap_GetSignedMapRootByRevision_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "trillian_map_api.proto",
}

func init() { proto.RegisterFile("trillian_map_api.proto", fileDescriptor1) }

var fileDescriptor1 = []byte{
	// 651 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xac, 0x55, 0xcb, 0x6e, 0xd3, 0x40,
	0x14, 0xc5, 0x49, 0xda, 0x26, 0x37, 0x08, 0x85, 0x69, 0xa0, 0xae, 0x69, 0x51, 0x6b, 0x54, 0x95,
	0xaa, 0x92, 0xdd, 0x9a, 0x1d, 0xbb, 0x56, 0x48, 0x7d, 0xa8, 0xad, 0x2a, 0x1b, 0x15, 0x89, 0x05,
	0xe1, 0xa6, 0x99, 0x26, 0x23, 0xd9, 0x1e, 0x13, 0x4f, 0xa2, 0x96, 0xaa, 0x1b, 0x16, 0x6c, 0x59,
	0xc0, 0x9a, 0xbf, 0xe0, 0x4b, 0xf8, 0x05, 0x3e, 0x04, 0x79, 0xfc, 0x68, 0x1e, 0x4e, 0x14, 0x21,
	0x76, 0x9e, 0x7b, 0xee, 0xe3, 0xdc, 0x33, 0x67, 0x64, 0x78, 0x2a, 0xba, 0xcc, 0x75, 0x19, 0xfa,
	0x0d, 0x0f, 0x83, 0x06, 0x06, 0xcc, 0x08, 0xba, 0x5c, 0x70, 0x52, 0x4e, 0xe3, 0xda, 0xa3, 0xf4,
	0x2b, 0x46, 0xb4, 0x95, 0x36, 0xe7, 0x6d, 0x97, 0x9a, 0x18, 0x30, 0x13, 0x7d, 0x9f, 0x0b, 0x14,
	0x8c, 0xfb, 0x61, 0x82, 0x2e, 0x27, 0xa8, 0x3c, 0x35, 0x7b, 0x57, 0x26, 0xfa, 0x37, 0x31, 0xa4,
	0x7f, 0x86, 0x85, 0x53, 0x0c, 0x4e, 0x28, 0x5e, 0x91, 0x3a, 0xcc, 0x31, 0xbf, 0x45, 0xaf, 0x55,
	0x65, 0x4d, 0x79, 0xf9, 0xd0, 0x8e, 0x0f, 0xe4, 0x19, 0x54, 0x5c, 0x8a, 0x57, 0x8d, 0x0e, 0x86,
	0x1d, 0xb5, 0x20, 0x91, 0x72, 0x14, 0x38, 0xc4, 0xb0, 0x43, 0x56, 0x01, 0x24, 0xd8, 0x47, 0xb7,
	0x47, 0xd5, 0xa2, 0x44, 0x65, 0xfa, 0x45, 0x14, 0x88, 0x60, 0x7a, 0x2d, 0xba, 0xd8, 0x68, 0xa1,
	0x40, 0xb5, 0x14, 0xc3, 0x32, 0xf2, 0x06, 0x05, 0xea, 0xef, 0xa0, 0x96, 0xcc, 0x3e, 0xf2, 0x2f,
	0xdd, 0x5e, 0xc8, 0xb8, 0x4f, 0x36, 0xa0, 0x14, 0xd5, 0x4b, 0x0e, 0x55, 0xeb, 0xb1, 0x91, 0xed,
	0x99, 0x64, 0xda, 0x12, 0x26, 0x2b, 0x50, 0x61, 0x69, 0x8d, 0x5a, 0x58, 0x2b, 0x46, 0x8d, 0xb3,
	0x80, 0xfe, 0x01, 0x16, 0x0f, 0xa8, 0x88, 0x2b, 0xfa, 0x34, 0xb4, 0xe9, 0xa7, 0x1e, 0x0d, 0x05,
	0x79, 0x02, 0xf3, 0x91, 0x9e, 0xac, 0x25, 0xbb, 0x17, 0xed, 0x39, 0x0f, 0x83, 0xa3, 0xd6, 0xfd,
	0xde, 0x71, 0x9f, 0x64, 0x6f, 0x0d, 0xca, 0x5d, 0xda, 0x67, 0x72, 0x40, 0x51, 0xa6, 0x67, 0x67,
	0xbd, 0x03, 0xab, 0x83, 0xfd, 0xf7, 0x6f, 0xec, 0x04, 0xf9, 0x2f, 0x93, 0x4a, 0x03, 0x93, 0x7e,
	0x28, 0x50, 0x1f, 0x5e, 0x25, 0x0c, 0xb8, 0x1f, 0x52, 0x72, 0x08, 0x24, 0x9a, 0x20, 0xd5, 0x1f,
	0x56, 0xa2, 0x6a, 0x69, 0x63, 0xaa, 0x65, 0xfa, 0xda, 0x35, 0x6f, 0x54, 0x71, 0x0b, 0xca, 0x51,
	0xa7, 0x2e, 0xe7, 0x42, 0x8e, 0xaf, 0x5a, 0x4b, 0xf7, 0xf5, 0x0e, 0x6b, 0xfb, 0xb4, 0x75, 0x8a,
	0x81, 0xcd, 0xb9, 0xb0, 0x17, 0xbc, 0xf8, 0x43, 0xff, 0xa6, 0xc0, 0xa2, 0x33, 0xbb, 0xc2, 0x5b,
	0x30, 0xef, 0xca, 0xbc, 0x84, 0x60, 0xce, 0xb5, 0x26, 0x09, 0x64, 0x07, 0xca, 0x1e, 0x15, 0x98,
	0x19, 0xa6, 0x6a, 0xd5, 0x8d, 0xd8, 0xbd, 0x46, 0xea, 0x5e, 0x63, 0xcf, 0xbf, 0xb1, 0xb3, 0xac,
	0xe3, 0x52, 0xb9, 0x58, 0x2b, 0xe9, 0xc7, 0x50, 0x77, 0xf2, 0x74, 0x1a, 0xdc, 0xae, 0x30, 0xe3,
	0x76, 0x3b, 0xb0, 0x74, 0x40, 0xc5, 0x30, 0x38, 0x75, 0x41, 0xfd, 0x02, 0xd6, 0x47, 0x2b, 0x66,
	0x36, 0xc5, 0xe0, 0xf5, 0x17, 0x46, 0x8c, 0x76, 0x06, 0xea, 0x38, 0x93, 0x7f, 0xdf, 0xcc, 0xfa,
	0x55, 0x82, 0xea, 0xdb, 0x24, 0xe7, 0x14, 0x03, 0x72, 0x02, 0x95, 0x03, 0x2a, 0x62, 0xc9, 0xc8,
	0xea, 0x7d, 0x79, 0xce, 0xeb, 0xd1, 0x9e, 0x4f, 0x82, 0x63, 0x3e, 0xfa, 0x03, 0xf2, 0x51, 0x3e,
	0xbb, 0xd1, 0x37, 0x41, 0x36, 0xf3, 0x0b, 0xc7, 0x04, 0x9a, 0x61, 0xc2, 0x09, 0x54, 0x9c, 0x3c,
	0xbe, 0xce, 0x74, 0xbe, 0x4e, 0x7e, 0xb7, 0xaf, 0x0a, 0xd4, 0x46, 0xe5, 0x25, 0xeb, 0x43, 0x24,
	0xf2, 0x4c, 0xa0, 0xe9, 0xd3, 0x52, 0x92, 0xee, 0xdb, 0x5f, 0x7e, 0xff, 0xf9, 0x5e, 0xd8, 0x20,
	0x2f, 0xcc, 0xfe, 0x6e, 0x93, 0x0a, 0xdc, 0x35, 0x3d, 0x0c, 0x42, 0xf3, 0x36, 0x76, 0xc0, 0x9d,
	0x19, 0x5d, 0x5b, 0xf8, 0xda, 0x45, 0x11, 0x39, 0xe3, 0xa7, 0x02, 0xda, 0x64, 0xff, 0x90, 0xed,
	0xc9, 0xf3, 0xc6, 0x45, 0x9c, 0x85, 0x9c, 0x29, 0xc9, 0x6d, 0x91, 0xcd, 0x69, 0xe4, 0xcc, 0xdb,
	0xd4, 0x86, 0x77, 0xfb, 0x67, 0xb0, 0x7c, 0xc9, 0xbd, 0xf4, 0x21, 0x0e, 0xff, 0x7b, 0xf6, 0x17,
	0x07, 0x1c, 0xb5, 0x17, 0xb0, 0xf3, 0x28, 0x78, 0xae, 0xbc, 0xd7, 0xda, 0x4c, 0x74, 0x7a, 0x4d,
	0xe3, 0x92, 0x7b, 0x66, 0xf2, 0xff, 0x49, 0x0b, 0x9b, 0xf3, 0xb2, 0xf2, 0xd5, 0xdf, 0x00, 0x00,
	0x00, 0xff, 0xff, 0x25, 0x04, 0xb7, 0x38, 0xe9, 0x06, 0x00, 0x00,
}
