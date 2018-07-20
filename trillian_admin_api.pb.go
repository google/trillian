// Code generated by protoc-gen-go. DO NOT EDIT.
// source: trillian_admin_api.proto

package trillian // import "github.com/google/trillian"

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"
import keyspb "github.com/google/trillian/crypto/keyspb"
import _ "google.golang.org/genproto/googleapis/api/annotations"
import field_mask "google.golang.org/genproto/protobuf/field_mask"

import (
	context "golang.org/x/net/context"
	grpc "google.golang.org/grpc"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

// ListTrees request.
// No filters or pagination options are provided.
type ListTreesRequest struct {
	// If true, deleted trees are included in the response.
	ShowDeleted          bool     `protobuf:"varint,1,opt,name=show_deleted,json=showDeleted" json:"show_deleted,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ListTreesRequest) Reset()         { *m = ListTreesRequest{} }
func (m *ListTreesRequest) String() string { return proto.CompactTextString(m) }
func (*ListTreesRequest) ProtoMessage()    {}
func (*ListTreesRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_trillian_admin_api_f5651a76db46bf6e, []int{0}
}
func (m *ListTreesRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ListTreesRequest.Unmarshal(m, b)
}
func (m *ListTreesRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ListTreesRequest.Marshal(b, m, deterministic)
}
func (dst *ListTreesRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ListTreesRequest.Merge(dst, src)
}
func (m *ListTreesRequest) XXX_Size() int {
	return xxx_messageInfo_ListTreesRequest.Size(m)
}
func (m *ListTreesRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_ListTreesRequest.DiscardUnknown(m)
}

var xxx_messageInfo_ListTreesRequest proto.InternalMessageInfo

func (m *ListTreesRequest) GetShowDeleted() bool {
	if m != nil {
		return m.ShowDeleted
	}
	return false
}

// ListTrees response.
// No pagination is provided, all trees the requester has access to are
// returned.
type ListTreesResponse struct {
	// Trees matching the list request filters.
	Tree                 []*Tree  `protobuf:"bytes,1,rep,name=tree" json:"tree,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ListTreesResponse) Reset()         { *m = ListTreesResponse{} }
func (m *ListTreesResponse) String() string { return proto.CompactTextString(m) }
func (*ListTreesResponse) ProtoMessage()    {}
func (*ListTreesResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_trillian_admin_api_f5651a76db46bf6e, []int{1}
}
func (m *ListTreesResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ListTreesResponse.Unmarshal(m, b)
}
func (m *ListTreesResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ListTreesResponse.Marshal(b, m, deterministic)
}
func (dst *ListTreesResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ListTreesResponse.Merge(dst, src)
}
func (m *ListTreesResponse) XXX_Size() int {
	return xxx_messageInfo_ListTreesResponse.Size(m)
}
func (m *ListTreesResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_ListTreesResponse.DiscardUnknown(m)
}

var xxx_messageInfo_ListTreesResponse proto.InternalMessageInfo

func (m *ListTreesResponse) GetTree() []*Tree {
	if m != nil {
		return m.Tree
	}
	return nil
}

// GetTree request.
type GetTreeRequest struct {
	// ID of the tree to retrieve.
	TreeId               int64    `protobuf:"varint,1,opt,name=tree_id,json=treeId" json:"tree_id,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *GetTreeRequest) Reset()         { *m = GetTreeRequest{} }
func (m *GetTreeRequest) String() string { return proto.CompactTextString(m) }
func (*GetTreeRequest) ProtoMessage()    {}
func (*GetTreeRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_trillian_admin_api_f5651a76db46bf6e, []int{2}
}
func (m *GetTreeRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_GetTreeRequest.Unmarshal(m, b)
}
func (m *GetTreeRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_GetTreeRequest.Marshal(b, m, deterministic)
}
func (dst *GetTreeRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_GetTreeRequest.Merge(dst, src)
}
func (m *GetTreeRequest) XXX_Size() int {
	return xxx_messageInfo_GetTreeRequest.Size(m)
}
func (m *GetTreeRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_GetTreeRequest.DiscardUnknown(m)
}

var xxx_messageInfo_GetTreeRequest proto.InternalMessageInfo

func (m *GetTreeRequest) GetTreeId() int64 {
	if m != nil {
		return m.TreeId
	}
	return 0
}

// CreateTree request.
type CreateTreeRequest struct {
	// Tree to be created. See Tree and CreateTree for more details.
	Tree *Tree `protobuf:"bytes,1,opt,name=tree" json:"tree,omitempty"`
	// Describes how the tree's private key should be generated.
	// Only needs to be set if tree.private_key is not set.
	KeySpec              *keyspb.Specification `protobuf:"bytes,2,opt,name=key_spec,json=keySpec" json:"key_spec,omitempty"`
	XXX_NoUnkeyedLiteral struct{}              `json:"-"`
	XXX_unrecognized     []byte                `json:"-"`
	XXX_sizecache        int32                 `json:"-"`
}

func (m *CreateTreeRequest) Reset()         { *m = CreateTreeRequest{} }
func (m *CreateTreeRequest) String() string { return proto.CompactTextString(m) }
func (*CreateTreeRequest) ProtoMessage()    {}
func (*CreateTreeRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_trillian_admin_api_f5651a76db46bf6e, []int{3}
}
func (m *CreateTreeRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_CreateTreeRequest.Unmarshal(m, b)
}
func (m *CreateTreeRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_CreateTreeRequest.Marshal(b, m, deterministic)
}
func (dst *CreateTreeRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_CreateTreeRequest.Merge(dst, src)
}
func (m *CreateTreeRequest) XXX_Size() int {
	return xxx_messageInfo_CreateTreeRequest.Size(m)
}
func (m *CreateTreeRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_CreateTreeRequest.DiscardUnknown(m)
}

var xxx_messageInfo_CreateTreeRequest proto.InternalMessageInfo

func (m *CreateTreeRequest) GetTree() *Tree {
	if m != nil {
		return m.Tree
	}
	return nil
}

func (m *CreateTreeRequest) GetKeySpec() *keyspb.Specification {
	if m != nil {
		return m.KeySpec
	}
	return nil
}

// UpdateTree request.
type UpdateTreeRequest struct {
	// Tree to be updated.
	Tree *Tree `protobuf:"bytes,1,opt,name=tree" json:"tree,omitempty"`
	// Fields modified by the update request.
	// For example: "tree_state", "display_name", "description".
	UpdateMask           *field_mask.FieldMask `protobuf:"bytes,2,opt,name=update_mask,json=updateMask" json:"update_mask,omitempty"`
	XXX_NoUnkeyedLiteral struct{}              `json:"-"`
	XXX_unrecognized     []byte                `json:"-"`
	XXX_sizecache        int32                 `json:"-"`
}

func (m *UpdateTreeRequest) Reset()         { *m = UpdateTreeRequest{} }
func (m *UpdateTreeRequest) String() string { return proto.CompactTextString(m) }
func (*UpdateTreeRequest) ProtoMessage()    {}
func (*UpdateTreeRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_trillian_admin_api_f5651a76db46bf6e, []int{4}
}
func (m *UpdateTreeRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_UpdateTreeRequest.Unmarshal(m, b)
}
func (m *UpdateTreeRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_UpdateTreeRequest.Marshal(b, m, deterministic)
}
func (dst *UpdateTreeRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_UpdateTreeRequest.Merge(dst, src)
}
func (m *UpdateTreeRequest) XXX_Size() int {
	return xxx_messageInfo_UpdateTreeRequest.Size(m)
}
func (m *UpdateTreeRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_UpdateTreeRequest.DiscardUnknown(m)
}

var xxx_messageInfo_UpdateTreeRequest proto.InternalMessageInfo

func (m *UpdateTreeRequest) GetTree() *Tree {
	if m != nil {
		return m.Tree
	}
	return nil
}

func (m *UpdateTreeRequest) GetUpdateMask() *field_mask.FieldMask {
	if m != nil {
		return m.UpdateMask
	}
	return nil
}

// DeleteTree request.
type DeleteTreeRequest struct {
	// ID of the tree to delete.
	TreeId               int64    `protobuf:"varint,1,opt,name=tree_id,json=treeId" json:"tree_id,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *DeleteTreeRequest) Reset()         { *m = DeleteTreeRequest{} }
func (m *DeleteTreeRequest) String() string { return proto.CompactTextString(m) }
func (*DeleteTreeRequest) ProtoMessage()    {}
func (*DeleteTreeRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_trillian_admin_api_f5651a76db46bf6e, []int{5}
}
func (m *DeleteTreeRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_DeleteTreeRequest.Unmarshal(m, b)
}
func (m *DeleteTreeRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_DeleteTreeRequest.Marshal(b, m, deterministic)
}
func (dst *DeleteTreeRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_DeleteTreeRequest.Merge(dst, src)
}
func (m *DeleteTreeRequest) XXX_Size() int {
	return xxx_messageInfo_DeleteTreeRequest.Size(m)
}
func (m *DeleteTreeRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_DeleteTreeRequest.DiscardUnknown(m)
}

var xxx_messageInfo_DeleteTreeRequest proto.InternalMessageInfo

func (m *DeleteTreeRequest) GetTreeId() int64 {
	if m != nil {
		return m.TreeId
	}
	return 0
}

// UndeleteTree request.
type UndeleteTreeRequest struct {
	// ID of the tree to undelete.
	TreeId               int64    `protobuf:"varint,1,opt,name=tree_id,json=treeId" json:"tree_id,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *UndeleteTreeRequest) Reset()         { *m = UndeleteTreeRequest{} }
func (m *UndeleteTreeRequest) String() string { return proto.CompactTextString(m) }
func (*UndeleteTreeRequest) ProtoMessage()    {}
func (*UndeleteTreeRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_trillian_admin_api_f5651a76db46bf6e, []int{6}
}
func (m *UndeleteTreeRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_UndeleteTreeRequest.Unmarshal(m, b)
}
func (m *UndeleteTreeRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_UndeleteTreeRequest.Marshal(b, m, deterministic)
}
func (dst *UndeleteTreeRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_UndeleteTreeRequest.Merge(dst, src)
}
func (m *UndeleteTreeRequest) XXX_Size() int {
	return xxx_messageInfo_UndeleteTreeRequest.Size(m)
}
func (m *UndeleteTreeRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_UndeleteTreeRequest.DiscardUnknown(m)
}

var xxx_messageInfo_UndeleteTreeRequest proto.InternalMessageInfo

func (m *UndeleteTreeRequest) GetTreeId() int64 {
	if m != nil {
		return m.TreeId
	}
	return 0
}

func init() {
	proto.RegisterType((*ListTreesRequest)(nil), "trillian.ListTreesRequest")
	proto.RegisterType((*ListTreesResponse)(nil), "trillian.ListTreesResponse")
	proto.RegisterType((*GetTreeRequest)(nil), "trillian.GetTreeRequest")
	proto.RegisterType((*CreateTreeRequest)(nil), "trillian.CreateTreeRequest")
	proto.RegisterType((*UpdateTreeRequest)(nil), "trillian.UpdateTreeRequest")
	proto.RegisterType((*DeleteTreeRequest)(nil), "trillian.DeleteTreeRequest")
	proto.RegisterType((*UndeleteTreeRequest)(nil), "trillian.UndeleteTreeRequest")
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion4

// TrillianAdminClient is the client API for TrillianAdmin service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type TrillianAdminClient interface {
	// Lists all trees the requester has access to.
	ListTrees(ctx context.Context, in *ListTreesRequest, opts ...grpc.CallOption) (*ListTreesResponse, error)
	// Retrieves a tree by ID.
	GetTree(ctx context.Context, in *GetTreeRequest, opts ...grpc.CallOption) (*Tree, error)
	// Creates a new tree.
	// System-generated fields are not required and will be ignored if present,
	// e.g.: tree_id, create_time and update_time.
	// Returns the created tree, with all system-generated fields assigned.
	CreateTree(ctx context.Context, in *CreateTreeRequest, opts ...grpc.CallOption) (*Tree, error)
	// Updates a tree.
	// See Tree for details. Readonly fields cannot be updated.
	UpdateTree(ctx context.Context, in *UpdateTreeRequest, opts ...grpc.CallOption) (*Tree, error)
	// Soft-deletes a tree.
	// A soft-deleted tree may be undeleted for a certain period, after which
	// it'll be permanently deleted.
	DeleteTree(ctx context.Context, in *DeleteTreeRequest, opts ...grpc.CallOption) (*Tree, error)
	// Undeletes a soft-deleted a tree.
	// A soft-deleted tree may be undeleted for a certain period, after which
	// it'll be permanently deleted.
	UndeleteTree(ctx context.Context, in *UndeleteTreeRequest, opts ...grpc.CallOption) (*Tree, error)
}

type trillianAdminClient struct {
	cc *grpc.ClientConn
}

func NewTrillianAdminClient(cc *grpc.ClientConn) TrillianAdminClient {
	return &trillianAdminClient{cc}
}

func (c *trillianAdminClient) ListTrees(ctx context.Context, in *ListTreesRequest, opts ...grpc.CallOption) (*ListTreesResponse, error) {
	out := new(ListTreesResponse)
	err := c.cc.Invoke(ctx, "/trillian.TrillianAdmin/ListTrees", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *trillianAdminClient) GetTree(ctx context.Context, in *GetTreeRequest, opts ...grpc.CallOption) (*Tree, error) {
	out := new(Tree)
	err := c.cc.Invoke(ctx, "/trillian.TrillianAdmin/GetTree", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *trillianAdminClient) CreateTree(ctx context.Context, in *CreateTreeRequest, opts ...grpc.CallOption) (*Tree, error) {
	out := new(Tree)
	err := c.cc.Invoke(ctx, "/trillian.TrillianAdmin/CreateTree", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *trillianAdminClient) UpdateTree(ctx context.Context, in *UpdateTreeRequest, opts ...grpc.CallOption) (*Tree, error) {
	out := new(Tree)
	err := c.cc.Invoke(ctx, "/trillian.TrillianAdmin/UpdateTree", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *trillianAdminClient) DeleteTree(ctx context.Context, in *DeleteTreeRequest, opts ...grpc.CallOption) (*Tree, error) {
	out := new(Tree)
	err := c.cc.Invoke(ctx, "/trillian.TrillianAdmin/DeleteTree", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *trillianAdminClient) UndeleteTree(ctx context.Context, in *UndeleteTreeRequest, opts ...grpc.CallOption) (*Tree, error) {
	out := new(Tree)
	err := c.cc.Invoke(ctx, "/trillian.TrillianAdmin/UndeleteTree", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Server API for TrillianAdmin service

type TrillianAdminServer interface {
	// Lists all trees the requester has access to.
	ListTrees(context.Context, *ListTreesRequest) (*ListTreesResponse, error)
	// Retrieves a tree by ID.
	GetTree(context.Context, *GetTreeRequest) (*Tree, error)
	// Creates a new tree.
	// System-generated fields are not required and will be ignored if present,
	// e.g.: tree_id, create_time and update_time.
	// Returns the created tree, with all system-generated fields assigned.
	CreateTree(context.Context, *CreateTreeRequest) (*Tree, error)
	// Updates a tree.
	// See Tree for details. Readonly fields cannot be updated.
	UpdateTree(context.Context, *UpdateTreeRequest) (*Tree, error)
	// Soft-deletes a tree.
	// A soft-deleted tree may be undeleted for a certain period, after which
	// it'll be permanently deleted.
	DeleteTree(context.Context, *DeleteTreeRequest) (*Tree, error)
	// Undeletes a soft-deleted a tree.
	// A soft-deleted tree may be undeleted for a certain period, after which
	// it'll be permanently deleted.
	UndeleteTree(context.Context, *UndeleteTreeRequest) (*Tree, error)
}

func RegisterTrillianAdminServer(s *grpc.Server, srv TrillianAdminServer) {
	s.RegisterService(&_TrillianAdmin_serviceDesc, srv)
}

func _TrillianAdmin_ListTrees_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListTreesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TrillianAdminServer).ListTrees(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/trillian.TrillianAdmin/ListTrees",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TrillianAdminServer).ListTrees(ctx, req.(*ListTreesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _TrillianAdmin_GetTree_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetTreeRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TrillianAdminServer).GetTree(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/trillian.TrillianAdmin/GetTree",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TrillianAdminServer).GetTree(ctx, req.(*GetTreeRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _TrillianAdmin_CreateTree_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CreateTreeRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TrillianAdminServer).CreateTree(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/trillian.TrillianAdmin/CreateTree",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TrillianAdminServer).CreateTree(ctx, req.(*CreateTreeRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _TrillianAdmin_UpdateTree_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UpdateTreeRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TrillianAdminServer).UpdateTree(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/trillian.TrillianAdmin/UpdateTree",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TrillianAdminServer).UpdateTree(ctx, req.(*UpdateTreeRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _TrillianAdmin_DeleteTree_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DeleteTreeRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TrillianAdminServer).DeleteTree(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/trillian.TrillianAdmin/DeleteTree",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TrillianAdminServer).DeleteTree(ctx, req.(*DeleteTreeRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _TrillianAdmin_UndeleteTree_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UndeleteTreeRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TrillianAdminServer).UndeleteTree(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/trillian.TrillianAdmin/UndeleteTree",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TrillianAdminServer).UndeleteTree(ctx, req.(*UndeleteTreeRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _TrillianAdmin_serviceDesc = grpc.ServiceDesc{
	ServiceName: "trillian.TrillianAdmin",
	HandlerType: (*TrillianAdminServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "ListTrees",
			Handler:    _TrillianAdmin_ListTrees_Handler,
		},
		{
			MethodName: "GetTree",
			Handler:    _TrillianAdmin_GetTree_Handler,
		},
		{
			MethodName: "CreateTree",
			Handler:    _TrillianAdmin_CreateTree_Handler,
		},
		{
			MethodName: "UpdateTree",
			Handler:    _TrillianAdmin_UpdateTree_Handler,
		},
		{
			MethodName: "DeleteTree",
			Handler:    _TrillianAdmin_DeleteTree_Handler,
		},
		{
			MethodName: "UndeleteTree",
			Handler:    _TrillianAdmin_UndeleteTree_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "trillian_admin_api.proto",
}

func init() {
	proto.RegisterFile("trillian_admin_api.proto", fileDescriptor_trillian_admin_api_f5651a76db46bf6e)
}

var fileDescriptor_trillian_admin_api_f5651a76db46bf6e = []byte{
	// 548 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x9c, 0x54, 0xe1, 0x6a, 0x13, 0x4d,
	0x14, 0xfd, 0xb6, 0x2d, 0x6d, 0xbe, 0x9b, 0x1a, 0xcc, 0x94, 0x62, 0xba, 0x56, 0x8c, 0xa3, 0x42,
	0x8d, 0xb2, 0x6b, 0x23, 0x22, 0x54, 0xfc, 0xd1, 0x2a, 0x15, 0x41, 0x21, 0xac, 0x29, 0x82, 0x20,
	0xcb, 0x66, 0xf7, 0x26, 0x1d, 0x93, 0xec, 0xac, 0x3b, 0x13, 0x25, 0x88, 0x7f, 0x7c, 0x05, 0x5f,
	0xc6, 0xf7, 0xf0, 0x15, 0x7c, 0x10, 0x99, 0xd9, 0xd9, 0xee, 0xa6, 0xdb, 0x48, 0xf1, 0xd7, 0xee,
	0xdc, 0x73, 0xef, 0x3d, 0x77, 0xce, 0xb9, 0x0c, 0xb4, 0x64, 0xca, 0x26, 0x13, 0x16, 0xc4, 0x7e,
	0x10, 0x4d, 0x59, 0xec, 0x07, 0x09, 0x73, 0x92, 0x94, 0x4b, 0x4e, 0x6a, 0x39, 0x62, 0x37, 0xf2,
	0xbf, 0x0c, 0xb1, 0xed, 0x30, 0x9d, 0x27, 0x92, 0xbb, 0x63, 0x9c, 0x8b, 0x64, 0x60, 0x3e, 0x06,
	0xdb, 0x1d, 0x71, 0x3e, 0x9a, 0xa0, 0x1b, 0x24, 0xcc, 0x0d, 0xe2, 0x98, 0xcb, 0x40, 0x32, 0x1e,
	0x0b, 0x83, 0xb6, 0x0d, 0xaa, 0x4f, 0x83, 0xd9, 0xd0, 0x1d, 0x32, 0x9c, 0x44, 0xfe, 0x34, 0x10,
	0xe3, 0x2c, 0x83, 0x3e, 0x86, 0xab, 0xaf, 0x99, 0x90, 0xfd, 0x14, 0x51, 0x78, 0xf8, 0x69, 0x86,
	0x42, 0x92, 0x5b, 0xb0, 0x29, 0x4e, 0xf9, 0x17, 0x3f, 0xc2, 0x09, 0x4a, 0x8c, 0x5a, 0x56, 0xdb,
	0xda, 0xab, 0x79, 0x75, 0x15, 0x7b, 0x91, 0x85, 0xe8, 0x13, 0x68, 0x96, 0xca, 0x44, 0xc2, 0x63,
	0x81, 0x84, 0xc2, 0x9a, 0x4c, 0x11, 0x5b, 0x56, 0x7b, 0x75, 0xaf, 0xde, 0x6d, 0x38, 0x67, 0xd7,
	0x50, 0x69, 0x9e, 0xc6, 0xe8, 0x3d, 0x68, 0xbc, 0x44, 0x5d, 0x97, 0xb3, 0x5d, 0x83, 0x0d, 0x85,
	0xf8, 0x2c, 0x23, 0x5a, 0xf5, 0xd6, 0xd5, 0xf1, 0x55, 0x44, 0x19, 0x34, 0x9f, 0xa7, 0x18, 0x48,
	0x2c, 0x67, 0x17, 0x1c, 0xd6, 0x32, 0x0e, 0xf2, 0x10, 0x6a, 0x63, 0x9c, 0xfb, 0x22, 0xc1, 0xb0,
	0xb5, 0xa2, 0xf3, 0xb6, 0x1d, 0x23, 0xda, 0xdb, 0x04, 0x43, 0x36, 0x64, 0xa1, 0x56, 0xc9, 0xdb,
	0x18, 0xe3, 0x5c, 0x45, 0xa8, 0x84, 0xe6, 0x49, 0x12, 0xfd, 0x03, 0xd5, 0x53, 0xa8, 0xcf, 0x74,
	0xa1, 0xd6, 0xd4, 0xb0, 0xd9, 0x4e, 0x26, 0xbb, 0x93, 0xcb, 0xee, 0x1c, 0x2b, 0xd9, 0xdf, 0x04,
	0x62, 0xec, 0x41, 0x96, 0xae, 0xfe, 0xe9, 0x03, 0x68, 0x66, 0x7a, 0x5e, 0x4a, 0x0e, 0x07, 0xb6,
	0x4e, 0xe2, 0xe8, 0xd2, 0xf9, 0xdd, 0x9f, 0x6b, 0x70, 0xa5, 0x6f, 0x46, 0x3e, 0x54, 0xbb, 0x46,
	0x8e, 0xe1, 0xff, 0x33, 0xd3, 0x88, 0x5d, 0xdc, 0xe7, 0xfc, 0x02, 0xd8, 0xd7, 0x2f, 0xc4, 0x32,
	0x97, 0xe9, 0x7f, 0xe4, 0x1d, 0x6c, 0x18, 0x0f, 0x49, 0xab, 0xc8, 0x5c, 0xb4, 0xd5, 0x3e, 0xa7,
	0x17, 0xa5, 0xdf, 0x7f, 0xfd, 0xfe, 0xb1, 0xb2, 0x4b, 0x6c, 0xf7, 0xf3, 0xfe, 0x00, 0x65, 0xb0,
	0xef, 0xaa, 0x39, 0x85, 0xfb, 0xd5, 0x4c, 0xff, 0xac, 0xf3, 0x8d, 0xf4, 0x01, 0x0a, 0xc7, 0x49,
	0x69, 0x8a, 0xca, 0x1e, 0x54, 0xda, 0xef, 0xe8, 0xf6, 0x5b, 0xb4, 0xb1, 0xd8, 0xfe, 0xc0, 0xea,
	0x10, 0x04, 0x28, 0xcc, 0x2d, 0x77, 0xad, 0x58, 0x5e, 0xe9, 0xda, 0xd1, 0x5d, 0xef, 0x74, 0x6f,
	0x5e, 0x34, 0xb4, 0x53, 0x4c, 0xae, 0x68, 0x3e, 0x00, 0x14, 0x6e, 0x96, 0x69, 0x2a, 0x1e, 0x2f,
	0xd3, 0xa6, 0xf3, 0x37, 0x6d, 0x3e, 0xc2, 0x66, 0xd9, 0x7e, 0x72, 0xa3, 0x74, 0x8f, 0xea, 0x5a,
	0x54, 0x28, 0xee, 0x6b, 0x8a, 0xbb, 0x9d, 0xdb, 0xcb, 0x29, 0x0e, 0x66, 0xa6, 0xcf, 0x51, 0x0f,
	0x76, 0x42, 0x3e, 0xcd, 0xb7, 0x78, 0xf1, 0x35, 0x3a, 0xda, 0x5e, 0x58, 0xaa, 0xc3, 0x84, 0xf5,
	0x54, 0xb8, 0x67, 0xbd, 0xb7, 0x47, 0x4c, 0x9e, 0xce, 0x06, 0x4e, 0xc8, 0xa7, 0xae, 0x79, 0x77,
	0xf2, 0xd2, 0xc1, 0xba, 0xae, 0x7d, 0xf4, 0x27, 0x00, 0x00, 0xff, 0xff, 0x58, 0xc1, 0xa5, 0x85,
	0xff, 0x04, 0x00, 0x00,
}
