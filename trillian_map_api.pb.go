// Code generated by protoc-gen-go. DO NOT EDIT.
// source: trillian_map_api.proto

package trillian

import (
	context "context"
	fmt "fmt"
	proto "github.com/golang/protobuf/proto"
	_ "google.golang.org/genproto/googleapis/api/annotations"
	grpc "google.golang.org/grpc"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion3 // please upgrade the proto package

// MapLeaf represents the data behind Map leaves.
type MapLeaf struct {
	// index is the location of this leaf.
	// All indexes for a given Map must contain a constant number of bits.
	// These are not numeric indices. Note that this is typically derived using a
	// hash and thus the length of all indices in the map will match the number
	// of bits in the hash function. Map entries do not have a well defined
	// ordering and it's not possible to sequentially iterate over them.
	Index []byte `protobuf:"bytes,1,opt,name=index,proto3" json:"index,omitempty"`
	// leaf_hash is the tree hash of leaf_value.  This does not need to be set
	// on SetMapLeavesRequest; the server will fill it in.
	// For an empty leaf (len(leaf_value)==0), there may be two possible values
	// for this hash:
	//  - If the leaf has never been set, it counts as an empty subtree and
	//    a nil value is used.
	//  - If the leaf has been explicitly set to a zero-length entry, it no
	//    longer counts as empty and the value of hasher.HashLeaf(index, nil)
	//    will be used.
	LeafHash []byte `protobuf:"bytes,2,opt,name=leaf_hash,json=leafHash,proto3" json:"leaf_hash,omitempty"`
	// leaf_value is the data the tree commits to.
	LeafValue []byte `protobuf:"bytes,3,opt,name=leaf_value,json=leafValue,proto3" json:"leaf_value,omitempty"`
	// extra_data holds related contextual data, but is not covered by any hash.
	ExtraData            []byte   `protobuf:"bytes,4,opt,name=extra_data,json=extraData,proto3" json:"extra_data,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *MapLeaf) Reset()         { *m = MapLeaf{} }
func (m *MapLeaf) String() string { return proto.CompactTextString(m) }
func (*MapLeaf) ProtoMessage()    {}
func (*MapLeaf) Descriptor() ([]byte, []int) {
	return fileDescriptor_28d34dfba22a7ce2, []int{0}
}

func (m *MapLeaf) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_MapLeaf.Unmarshal(m, b)
}
func (m *MapLeaf) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_MapLeaf.Marshal(b, m, deterministic)
}
func (m *MapLeaf) XXX_Merge(src proto.Message) {
	xxx_messageInfo_MapLeaf.Merge(m, src)
}
func (m *MapLeaf) XXX_Size() int {
	return xxx_messageInfo_MapLeaf.Size(m)
}
func (m *MapLeaf) XXX_DiscardUnknown() {
	xxx_messageInfo_MapLeaf.DiscardUnknown(m)
}

var xxx_messageInfo_MapLeaf proto.InternalMessageInfo

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
	Leaf *MapLeaf `protobuf:"bytes,1,opt,name=leaf,proto3" json:"leaf,omitempty"`
	// inclusion holds the inclusion proof for this leaf in the map root. It
	// holds one entry for each level of the tree; combining each of these in
	// turn with the leaf's hash (according to the tree's hash strategy)
	// reproduces the root hash.  A nil entry for a particular level indicates
	// that the node in question has an empty subtree beneath it (and so its
	// associated hash value is hasher.HashEmpty(index, height) rather than
	// hasher.HashChildren(l_hash, r_hash)).
	Inclusion            [][]byte `protobuf:"bytes,2,rep,name=inclusion,proto3" json:"inclusion,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *MapLeafInclusion) Reset()         { *m = MapLeafInclusion{} }
func (m *MapLeafInclusion) String() string { return proto.CompactTextString(m) }
func (*MapLeafInclusion) ProtoMessage()    {}
func (*MapLeafInclusion) Descriptor() ([]byte, []int) {
	return fileDescriptor_28d34dfba22a7ce2, []int{1}
}

func (m *MapLeafInclusion) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_MapLeafInclusion.Unmarshal(m, b)
}
func (m *MapLeafInclusion) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_MapLeafInclusion.Marshal(b, m, deterministic)
}
func (m *MapLeafInclusion) XXX_Merge(src proto.Message) {
	xxx_messageInfo_MapLeafInclusion.Merge(m, src)
}
func (m *MapLeafInclusion) XXX_Size() int {
	return xxx_messageInfo_MapLeafInclusion.Size(m)
}
func (m *MapLeafInclusion) XXX_DiscardUnknown() {
	xxx_messageInfo_MapLeafInclusion.DiscardUnknown(m)
}

var xxx_messageInfo_MapLeafInclusion proto.InternalMessageInfo

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
	MapId                int64    `protobuf:"varint,1,opt,name=map_id,json=mapId,proto3" json:"map_id,omitempty"`
	Index                [][]byte `protobuf:"bytes,2,rep,name=index,proto3" json:"index,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *GetMapLeavesRequest) Reset()         { *m = GetMapLeavesRequest{} }
func (m *GetMapLeavesRequest) String() string { return proto.CompactTextString(m) }
func (*GetMapLeavesRequest) ProtoMessage()    {}
func (*GetMapLeavesRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_28d34dfba22a7ce2, []int{2}
}

func (m *GetMapLeavesRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_GetMapLeavesRequest.Unmarshal(m, b)
}
func (m *GetMapLeavesRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_GetMapLeavesRequest.Marshal(b, m, deterministic)
}
func (m *GetMapLeavesRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_GetMapLeavesRequest.Merge(m, src)
}
func (m *GetMapLeavesRequest) XXX_Size() int {
	return xxx_messageInfo_GetMapLeavesRequest.Size(m)
}
func (m *GetMapLeavesRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_GetMapLeavesRequest.DiscardUnknown(m)
}

var xxx_messageInfo_GetMapLeavesRequest proto.InternalMessageInfo

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

// This message replaces the current implementation of GetMapLeavesRequest
// with the difference that revision must be >=0.
type GetMapLeavesByRevisionRequest struct {
	MapId int64    `protobuf:"varint,1,opt,name=map_id,json=mapId,proto3" json:"map_id,omitempty"`
	Index [][]byte `protobuf:"bytes,2,rep,name=index,proto3" json:"index,omitempty"`
	// revision >= 0.
	Revision             int64    `protobuf:"varint,3,opt,name=revision,proto3" json:"revision,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *GetMapLeavesByRevisionRequest) Reset()         { *m = GetMapLeavesByRevisionRequest{} }
func (m *GetMapLeavesByRevisionRequest) String() string { return proto.CompactTextString(m) }
func (*GetMapLeavesByRevisionRequest) ProtoMessage()    {}
func (*GetMapLeavesByRevisionRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_28d34dfba22a7ce2, []int{3}
}

func (m *GetMapLeavesByRevisionRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_GetMapLeavesByRevisionRequest.Unmarshal(m, b)
}
func (m *GetMapLeavesByRevisionRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_GetMapLeavesByRevisionRequest.Marshal(b, m, deterministic)
}
func (m *GetMapLeavesByRevisionRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_GetMapLeavesByRevisionRequest.Merge(m, src)
}
func (m *GetMapLeavesByRevisionRequest) XXX_Size() int {
	return xxx_messageInfo_GetMapLeavesByRevisionRequest.Size(m)
}
func (m *GetMapLeavesByRevisionRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_GetMapLeavesByRevisionRequest.DiscardUnknown(m)
}

var xxx_messageInfo_GetMapLeavesByRevisionRequest proto.InternalMessageInfo

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

func (m *GetMapLeavesByRevisionRequest) GetRevision() int64 {
	if m != nil {
		return m.Revision
	}
	return 0
}

type GetMapLeavesResponse struct {
	MapLeafInclusion     []*MapLeafInclusion `protobuf:"bytes,2,rep,name=map_leaf_inclusion,json=mapLeafInclusion,proto3" json:"map_leaf_inclusion,omitempty"`
	MapRoot              *SignedMapRoot      `protobuf:"bytes,3,opt,name=map_root,json=mapRoot,proto3" json:"map_root,omitempty"`
	XXX_NoUnkeyedLiteral struct{}            `json:"-"`
	XXX_unrecognized     []byte              `json:"-"`
	XXX_sizecache        int32               `json:"-"`
}

func (m *GetMapLeavesResponse) Reset()         { *m = GetMapLeavesResponse{} }
func (m *GetMapLeavesResponse) String() string { return proto.CompactTextString(m) }
func (*GetMapLeavesResponse) ProtoMessage()    {}
func (*GetMapLeavesResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_28d34dfba22a7ce2, []int{4}
}

func (m *GetMapLeavesResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_GetMapLeavesResponse.Unmarshal(m, b)
}
func (m *GetMapLeavesResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_GetMapLeavesResponse.Marshal(b, m, deterministic)
}
func (m *GetMapLeavesResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_GetMapLeavesResponse.Merge(m, src)
}
func (m *GetMapLeavesResponse) XXX_Size() int {
	return xxx_messageInfo_GetMapLeavesResponse.Size(m)
}
func (m *GetMapLeavesResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_GetMapLeavesResponse.DiscardUnknown(m)
}

var xxx_messageInfo_GetMapLeavesResponse proto.InternalMessageInfo

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
	MapId int64 `protobuf:"varint,1,opt,name=map_id,json=mapId,proto3" json:"map_id,omitempty"`
	// The leaves being set must have unique Index values within the request.
	Leaves   []*MapLeaf `protobuf:"bytes,2,rep,name=leaves,proto3" json:"leaves,omitempty"`
	Metadata []byte     `protobuf:"bytes,5,opt,name=metadata,proto3" json:"metadata,omitempty"`
	// The map revision to associate the leaves with. The request will fail if
	// this revision already exists, does not match the current write revision, or
	// is negative. If revision = 0 then the leaves will be written to the current
	// write revision.
	Revision             int64    `protobuf:"varint,6,opt,name=revision,proto3" json:"revision,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *SetMapLeavesRequest) Reset()         { *m = SetMapLeavesRequest{} }
func (m *SetMapLeavesRequest) String() string { return proto.CompactTextString(m) }
func (*SetMapLeavesRequest) ProtoMessage()    {}
func (*SetMapLeavesRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_28d34dfba22a7ce2, []int{5}
}

func (m *SetMapLeavesRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SetMapLeavesRequest.Unmarshal(m, b)
}
func (m *SetMapLeavesRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SetMapLeavesRequest.Marshal(b, m, deterministic)
}
func (m *SetMapLeavesRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SetMapLeavesRequest.Merge(m, src)
}
func (m *SetMapLeavesRequest) XXX_Size() int {
	return xxx_messageInfo_SetMapLeavesRequest.Size(m)
}
func (m *SetMapLeavesRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_SetMapLeavesRequest.DiscardUnknown(m)
}

var xxx_messageInfo_SetMapLeavesRequest proto.InternalMessageInfo

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

func (m *SetMapLeavesRequest) GetMetadata() []byte {
	if m != nil {
		return m.Metadata
	}
	return nil
}

func (m *SetMapLeavesRequest) GetRevision() int64 {
	if m != nil {
		return m.Revision
	}
	return 0
}

type SetMapLeavesResponse struct {
	MapRoot              *SignedMapRoot `protobuf:"bytes,2,opt,name=map_root,json=mapRoot,proto3" json:"map_root,omitempty"`
	XXX_NoUnkeyedLiteral struct{}       `json:"-"`
	XXX_unrecognized     []byte         `json:"-"`
	XXX_sizecache        int32          `json:"-"`
}

func (m *SetMapLeavesResponse) Reset()         { *m = SetMapLeavesResponse{} }
func (m *SetMapLeavesResponse) String() string { return proto.CompactTextString(m) }
func (*SetMapLeavesResponse) ProtoMessage()    {}
func (*SetMapLeavesResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_28d34dfba22a7ce2, []int{6}
}

func (m *SetMapLeavesResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SetMapLeavesResponse.Unmarshal(m, b)
}
func (m *SetMapLeavesResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SetMapLeavesResponse.Marshal(b, m, deterministic)
}
func (m *SetMapLeavesResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SetMapLeavesResponse.Merge(m, src)
}
func (m *SetMapLeavesResponse) XXX_Size() int {
	return xxx_messageInfo_SetMapLeavesResponse.Size(m)
}
func (m *SetMapLeavesResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_SetMapLeavesResponse.DiscardUnknown(m)
}

var xxx_messageInfo_SetMapLeavesResponse proto.InternalMessageInfo

func (m *SetMapLeavesResponse) GetMapRoot() *SignedMapRoot {
	if m != nil {
		return m.MapRoot
	}
	return nil
}

type GetSignedMapRootRequest struct {
	MapId                int64    `protobuf:"varint,1,opt,name=map_id,json=mapId,proto3" json:"map_id,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *GetSignedMapRootRequest) Reset()         { *m = GetSignedMapRootRequest{} }
func (m *GetSignedMapRootRequest) String() string { return proto.CompactTextString(m) }
func (*GetSignedMapRootRequest) ProtoMessage()    {}
func (*GetSignedMapRootRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_28d34dfba22a7ce2, []int{7}
}

func (m *GetSignedMapRootRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_GetSignedMapRootRequest.Unmarshal(m, b)
}
func (m *GetSignedMapRootRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_GetSignedMapRootRequest.Marshal(b, m, deterministic)
}
func (m *GetSignedMapRootRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_GetSignedMapRootRequest.Merge(m, src)
}
func (m *GetSignedMapRootRequest) XXX_Size() int {
	return xxx_messageInfo_GetSignedMapRootRequest.Size(m)
}
func (m *GetSignedMapRootRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_GetSignedMapRootRequest.DiscardUnknown(m)
}

var xxx_messageInfo_GetSignedMapRootRequest proto.InternalMessageInfo

func (m *GetSignedMapRootRequest) GetMapId() int64 {
	if m != nil {
		return m.MapId
	}
	return 0
}

type GetSignedMapRootByRevisionRequest struct {
	MapId                int64    `protobuf:"varint,1,opt,name=map_id,json=mapId,proto3" json:"map_id,omitempty"`
	Revision             int64    `protobuf:"varint,2,opt,name=revision,proto3" json:"revision,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *GetSignedMapRootByRevisionRequest) Reset()         { *m = GetSignedMapRootByRevisionRequest{} }
func (m *GetSignedMapRootByRevisionRequest) String() string { return proto.CompactTextString(m) }
func (*GetSignedMapRootByRevisionRequest) ProtoMessage()    {}
func (*GetSignedMapRootByRevisionRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_28d34dfba22a7ce2, []int{8}
}

func (m *GetSignedMapRootByRevisionRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_GetSignedMapRootByRevisionRequest.Unmarshal(m, b)
}
func (m *GetSignedMapRootByRevisionRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_GetSignedMapRootByRevisionRequest.Marshal(b, m, deterministic)
}
func (m *GetSignedMapRootByRevisionRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_GetSignedMapRootByRevisionRequest.Merge(m, src)
}
func (m *GetSignedMapRootByRevisionRequest) XXX_Size() int {
	return xxx_messageInfo_GetSignedMapRootByRevisionRequest.Size(m)
}
func (m *GetSignedMapRootByRevisionRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_GetSignedMapRootByRevisionRequest.DiscardUnknown(m)
}

var xxx_messageInfo_GetSignedMapRootByRevisionRequest proto.InternalMessageInfo

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
	MapRoot              *SignedMapRoot `protobuf:"bytes,2,opt,name=map_root,json=mapRoot,proto3" json:"map_root,omitempty"`
	XXX_NoUnkeyedLiteral struct{}       `json:"-"`
	XXX_unrecognized     []byte         `json:"-"`
	XXX_sizecache        int32          `json:"-"`
}

func (m *GetSignedMapRootResponse) Reset()         { *m = GetSignedMapRootResponse{} }
func (m *GetSignedMapRootResponse) String() string { return proto.CompactTextString(m) }
func (*GetSignedMapRootResponse) ProtoMessage()    {}
func (*GetSignedMapRootResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_28d34dfba22a7ce2, []int{9}
}

func (m *GetSignedMapRootResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_GetSignedMapRootResponse.Unmarshal(m, b)
}
func (m *GetSignedMapRootResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_GetSignedMapRootResponse.Marshal(b, m, deterministic)
}
func (m *GetSignedMapRootResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_GetSignedMapRootResponse.Merge(m, src)
}
func (m *GetSignedMapRootResponse) XXX_Size() int {
	return xxx_messageInfo_GetSignedMapRootResponse.Size(m)
}
func (m *GetSignedMapRootResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_GetSignedMapRootResponse.DiscardUnknown(m)
}

var xxx_messageInfo_GetSignedMapRootResponse proto.InternalMessageInfo

func (m *GetSignedMapRootResponse) GetMapRoot() *SignedMapRoot {
	if m != nil {
		return m.MapRoot
	}
	return nil
}

type InitMapRequest struct {
	MapId                int64    `protobuf:"varint,1,opt,name=map_id,json=mapId,proto3" json:"map_id,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *InitMapRequest) Reset()         { *m = InitMapRequest{} }
func (m *InitMapRequest) String() string { return proto.CompactTextString(m) }
func (*InitMapRequest) ProtoMessage()    {}
func (*InitMapRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_28d34dfba22a7ce2, []int{10}
}

func (m *InitMapRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_InitMapRequest.Unmarshal(m, b)
}
func (m *InitMapRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_InitMapRequest.Marshal(b, m, deterministic)
}
func (m *InitMapRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_InitMapRequest.Merge(m, src)
}
func (m *InitMapRequest) XXX_Size() int {
	return xxx_messageInfo_InitMapRequest.Size(m)
}
func (m *InitMapRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_InitMapRequest.DiscardUnknown(m)
}

var xxx_messageInfo_InitMapRequest proto.InternalMessageInfo

func (m *InitMapRequest) GetMapId() int64 {
	if m != nil {
		return m.MapId
	}
	return 0
}

type InitMapResponse struct {
	Created              *SignedMapRoot `protobuf:"bytes,1,opt,name=created,proto3" json:"created,omitempty"`
	XXX_NoUnkeyedLiteral struct{}       `json:"-"`
	XXX_unrecognized     []byte         `json:"-"`
	XXX_sizecache        int32          `json:"-"`
}

func (m *InitMapResponse) Reset()         { *m = InitMapResponse{} }
func (m *InitMapResponse) String() string { return proto.CompactTextString(m) }
func (*InitMapResponse) ProtoMessage()    {}
func (*InitMapResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_28d34dfba22a7ce2, []int{11}
}

func (m *InitMapResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_InitMapResponse.Unmarshal(m, b)
}
func (m *InitMapResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_InitMapResponse.Marshal(b, m, deterministic)
}
func (m *InitMapResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_InitMapResponse.Merge(m, src)
}
func (m *InitMapResponse) XXX_Size() int {
	return xxx_messageInfo_InitMapResponse.Size(m)
}
func (m *InitMapResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_InitMapResponse.DiscardUnknown(m)
}

var xxx_messageInfo_InitMapResponse proto.InternalMessageInfo

func (m *InitMapResponse) GetCreated() *SignedMapRoot {
	if m != nil {
		return m.Created
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
	proto.RegisterType((*InitMapRequest)(nil), "trillian.InitMapRequest")
	proto.RegisterType((*InitMapResponse)(nil), "trillian.InitMapResponse")
}

func init() { proto.RegisterFile("trillian_map_api.proto", fileDescriptor_28d34dfba22a7ce2) }

var fileDescriptor_28d34dfba22a7ce2 = []byte{
	// 699 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xa4, 0x55, 0xdb, 0x4e, 0xdb, 0x4a,
	0x14, 0x3d, 0xce, 0x3d, 0x3b, 0x47, 0x9c, 0x9c, 0x81, 0x73, 0x30, 0x86, 0x54, 0x60, 0x84, 0x28,
	0x42, 0x8a, 0x4b, 0xfa, 0xc6, 0x5b, 0x11, 0x12, 0x17, 0x01, 0x42, 0x4e, 0x45, 0xa5, 0xbe, 0xa4,
	0x43, 0x32, 0x24, 0x23, 0xd9, 0x1e, 0x37, 0x9e, 0x44, 0xb4, 0x88, 0x97, 0x3e, 0xf4, 0x07, 0xda,
	0xa7, 0x56, 0xea, 0x4f, 0xf5, 0x17, 0xfa, 0x21, 0xd5, 0x5c, 0x72, 0x71, 0xe2, 0xa4, 0x51, 0xfb,
	0xe6, 0x99, 0x7d, 0x59, 0x6b, 0xaf, 0xbd, 0x46, 0x86, 0xff, 0x79, 0x97, 0x7a, 0x1e, 0xc5, 0x41,
	0xc3, 0xc7, 0x61, 0x03, 0x87, 0xb4, 0x1a, 0x76, 0x19, 0x67, 0xa8, 0x30, 0xb8, 0xb7, 0x96, 0x06,
	0x5f, 0x2a, 0x62, 0x6d, 0xb4, 0x19, 0x6b, 0x7b, 0xc4, 0xc1, 0x21, 0x75, 0x70, 0x10, 0x30, 0x8e,
	0x39, 0x65, 0x41, 0xa4, 0xa2, 0xf6, 0x7b, 0xc8, 0x5f, 0xe2, 0xf0, 0x82, 0xe0, 0x3b, 0xb4, 0x02,
	0x59, 0x1a, 0xb4, 0xc8, 0xbd, 0x69, 0x6c, 0x1a, 0x4f, 0xff, 0x76, 0xd5, 0x01, 0xad, 0x43, 0xd1,
	0x23, 0xf8, 0xae, 0xd1, 0xc1, 0x51, 0xc7, 0x4c, 0xc9, 0x48, 0x41, 0x5c, 0x9c, 0xe2, 0xa8, 0x83,
	0x2a, 0x00, 0x32, 0xd8, 0xc7, 0x5e, 0x8f, 0x98, 0x69, 0x19, 0x95, 0xe9, 0x37, 0xe2, 0x42, 0x84,
	0xc9, 0x3d, 0xef, 0xe2, 0x46, 0x0b, 0x73, 0x6c, 0x66, 0x54, 0x58, 0xde, 0x1c, 0x63, 0x8e, 0xed,
	0x57, 0x50, 0xd6, 0xd8, 0x67, 0x41, 0xd3, 0xeb, 0x45, 0x94, 0x05, 0x68, 0x07, 0x32, 0xa2, 0x5e,
	0x72, 0x28, 0xd5, 0xfe, 0xad, 0x0e, 0x87, 0xd1, 0x99, 0xae, 0x0c, 0xa3, 0x0d, 0x28, 0xd2, 0x41,
	0x8d, 0x99, 0xda, 0x4c, 0x8b, 0xc6, 0xc3, 0x0b, 0xfb, 0x14, 0x96, 0x4f, 0x08, 0x57, 0x15, 0x7d,
	0x12, 0xb9, 0xe4, 0x6d, 0x8f, 0x44, 0x1c, 0xfd, 0x07, 0x39, 0x21, 0x1a, 0x6d, 0xc9, 0xee, 0x69,
	0x37, 0xeb, 0xe3, 0xf0, 0xac, 0x35, 0x9a, 0x5b, 0xf5, 0x51, 0x87, 0xf3, 0x4c, 0x21, 0x5d, 0xce,
	0xd8, 0x1d, 0xa8, 0x8c, 0x77, 0x3a, 0x7a, 0xe7, 0x92, 0x3e, 0x15, 0x18, 0xbf, 0xd3, 0x13, 0x59,
	0x50, 0xe8, 0xea, 0x7a, 0x29, 0x56, 0xda, 0x1d, 0x9e, 0xed, 0xcf, 0x06, 0xac, 0xc4, 0x49, 0x47,
	0x21, 0x0b, 0x22, 0x82, 0x4e, 0x01, 0x09, 0x04, 0xa9, 0x73, 0x7c, 0xe6, 0x52, 0xcd, 0x9a, 0xd2,
	0x67, 0xa8, 0xa4, 0x5b, 0xf6, 0x27, 0xb5, 0xad, 0x41, 0x41, 0x74, 0xea, 0x32, 0xc6, 0x25, 0x7c,
	0xa9, 0xb6, 0x3a, 0xaa, 0xaf, 0xd3, 0x76, 0x40, 0x5a, 0x97, 0x38, 0x74, 0x19, 0xe3, 0x6e, 0xde,
	0x57, 0x1f, 0xf6, 0x57, 0x03, 0x96, 0xeb, 0x8b, 0x6b, 0xb9, 0x07, 0x39, 0x4f, 0xe6, 0x69, 0x82,
	0x09, 0x0b, 0xd4, 0x09, 0x42, 0x0c, 0x9f, 0x70, 0x2c, 0xad, 0x91, 0x55, 0xbe, 0x1a, 0x9c, 0x63,
	0x42, 0xe5, 0xe2, 0x42, 0xa9, 0xc5, 0x9c, 0x67, 0x0a, 0x99, 0x72, 0xd6, 0x3e, 0x87, 0x95, 0x7a,
	0x92, 0x66, 0xe3, 0x93, 0xa6, 0x16, 0x9c, 0xf4, 0x19, 0xac, 0x9e, 0x10, 0x1e, 0x0f, 0xce, 0x1d,
	0xd6, 0xbe, 0x81, 0xad, 0xc9, 0x8a, 0x85, 0x0d, 0x32, 0x3e, 0x61, 0x6a, 0xc2, 0x0a, 0x57, 0x60,
	0x4e, 0x33, 0xf9, 0x83, 0xc9, 0x76, 0x61, 0xe9, 0x2c, 0xa0, 0x42, 0xa6, 0x5f, 0x0c, 0x74, 0x0c,
	0xff, 0x0c, 0x13, 0x35, 0xde, 0x01, 0xe4, 0x9b, 0x5d, 0x82, 0x39, 0x69, 0xe9, 0x27, 0x39, 0x1b,
	0x4e, 0xe7, 0xd5, 0xbe, 0x64, 0xa1, 0xf4, 0x52, 0xe7, 0x5c, 0xe2, 0x10, 0x5d, 0x40, 0xf1, 0x84,
	0x70, 0xb5, 0x21, 0x54, 0x19, 0x95, 0x27, 0x3c, 0x51, 0xeb, 0xc9, 0xac, 0xb0, 0xa2, 0x63, 0xff,
	0x85, 0xde, 0xc8, 0xb7, 0x3d, 0xf9, 0x1c, 0xd1, 0x6e, 0x72, 0xe1, 0xd4, 0x3e, 0x16, 0x40, 0xb8,
	0x80, 0x62, 0x3d, 0x89, 0x6f, 0x7d, 0x3e, 0xdf, 0x7a, 0x72, 0xb7, 0x8f, 0x06, 0x94, 0x27, 0xb7,
	0x89, 0xb6, 0x62, 0x24, 0x92, 0x3c, 0x67, 0xd9, 0xf3, 0x52, 0x74, 0xf7, 0xfd, 0x0f, 0xdf, 0x7f,
	0x7c, 0x4a, 0xed, 0xa0, 0x6d, 0xa7, 0x7f, 0x70, 0x4b, 0x38, 0x3e, 0x70, 0x7c, 0x1c, 0x46, 0xce,
	0x83, 0xda, 0xed, 0xa3, 0x23, 0x5c, 0x12, 0x1d, 0x7a, 0x98, 0x8b, 0x9d, 0x7f, 0x33, 0xc0, 0x9a,
	0x6d, 0x57, 0xb4, 0x3f, 0x1b, 0x6f, 0x5a, 0xc4, 0x45, 0xc8, 0x39, 0x92, 0xdc, 0x1e, 0xda, 0x9d,
	0x47, 0xce, 0x79, 0x18, 0xb8, 0xfe, 0x11, 0x35, 0x21, 0xaf, 0xdd, 0x87, 0xcc, 0x51, 0xff, 0xb8,
	0x73, 0xad, 0xb5, 0x84, 0x88, 0x06, 0xdc, 0x96, 0x80, 0x15, 0x7b, 0x3d, 0x19, 0xf0, 0x90, 0x06,
	0x94, 0x1f, 0x5d, 0xc1, 0x5a, 0x93, 0xf9, 0x55, 0xf5, 0x4f, 0xac, 0xc6, 0x7f, 0x95, 0x47, 0xcb,
	0x63, 0xb6, 0x7d, 0x11, 0xd2, 0x6b, 0x71, 0x79, 0x6d, 0xbc, 0xb6, 0xda, 0x94, 0x77, 0x7a, 0xb7,
	0xd5, 0x26, 0xf3, 0x1d, 0xfd, 0x33, 0x1d, 0x14, 0xde, 0xe6, 0x64, 0xe5, 0xf3, 0x9f, 0x01, 0x00,
	0x00, 0xff, 0xff, 0x67, 0x02, 0x59, 0xb8, 0x98, 0x07, 0x00, 0x00,
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion4

// TrillianMapClient is the client API for TrillianMap service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type TrillianMapClient interface {
	// GetLeaves returns an inclusion proof for each index requested.
	// For indexes that do not exist, the inclusion proof will use nil for the
	// empty leaf value.
	GetLeaves(ctx context.Context, in *GetMapLeavesRequest, opts ...grpc.CallOption) (*GetMapLeavesResponse, error)
	GetLeavesByRevision(ctx context.Context, in *GetMapLeavesByRevisionRequest, opts ...grpc.CallOption) (*GetMapLeavesResponse, error)
	// SetLeaves sets the values for the provided leaves, and returns the new map
	// root if successful. Note that if a SetLeaves request fails for a
	// server-side reason (i.e. not an invalid request), the API user is required
	// to retry the request before performing a different SetLeaves request.
	SetLeaves(ctx context.Context, in *SetMapLeavesRequest, opts ...grpc.CallOption) (*SetMapLeavesResponse, error)
	GetSignedMapRoot(ctx context.Context, in *GetSignedMapRootRequest, opts ...grpc.CallOption) (*GetSignedMapRootResponse, error)
	GetSignedMapRootByRevision(ctx context.Context, in *GetSignedMapRootByRevisionRequest, opts ...grpc.CallOption) (*GetSignedMapRootResponse, error)
	InitMap(ctx context.Context, in *InitMapRequest, opts ...grpc.CallOption) (*InitMapResponse, error)
}

type trillianMapClient struct {
	cc *grpc.ClientConn
}

func NewTrillianMapClient(cc *grpc.ClientConn) TrillianMapClient {
	return &trillianMapClient{cc}
}

func (c *trillianMapClient) GetLeaves(ctx context.Context, in *GetMapLeavesRequest, opts ...grpc.CallOption) (*GetMapLeavesResponse, error) {
	out := new(GetMapLeavesResponse)
	err := c.cc.Invoke(ctx, "/trillian.TrillianMap/GetLeaves", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *trillianMapClient) GetLeavesByRevision(ctx context.Context, in *GetMapLeavesByRevisionRequest, opts ...grpc.CallOption) (*GetMapLeavesResponse, error) {
	out := new(GetMapLeavesResponse)
	err := c.cc.Invoke(ctx, "/trillian.TrillianMap/GetLeavesByRevision", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *trillianMapClient) SetLeaves(ctx context.Context, in *SetMapLeavesRequest, opts ...grpc.CallOption) (*SetMapLeavesResponse, error) {
	out := new(SetMapLeavesResponse)
	err := c.cc.Invoke(ctx, "/trillian.TrillianMap/SetLeaves", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *trillianMapClient) GetSignedMapRoot(ctx context.Context, in *GetSignedMapRootRequest, opts ...grpc.CallOption) (*GetSignedMapRootResponse, error) {
	out := new(GetSignedMapRootResponse)
	err := c.cc.Invoke(ctx, "/trillian.TrillianMap/GetSignedMapRoot", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *trillianMapClient) GetSignedMapRootByRevision(ctx context.Context, in *GetSignedMapRootByRevisionRequest, opts ...grpc.CallOption) (*GetSignedMapRootResponse, error) {
	out := new(GetSignedMapRootResponse)
	err := c.cc.Invoke(ctx, "/trillian.TrillianMap/GetSignedMapRootByRevision", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *trillianMapClient) InitMap(ctx context.Context, in *InitMapRequest, opts ...grpc.CallOption) (*InitMapResponse, error) {
	out := new(InitMapResponse)
	err := c.cc.Invoke(ctx, "/trillian.TrillianMap/InitMap", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// TrillianMapServer is the server API for TrillianMap service.
type TrillianMapServer interface {
	// GetLeaves returns an inclusion proof for each index requested.
	// For indexes that do not exist, the inclusion proof will use nil for the
	// empty leaf value.
	GetLeaves(context.Context, *GetMapLeavesRequest) (*GetMapLeavesResponse, error)
	GetLeavesByRevision(context.Context, *GetMapLeavesByRevisionRequest) (*GetMapLeavesResponse, error)
	// SetLeaves sets the values for the provided leaves, and returns the new map
	// root if successful. Note that if a SetLeaves request fails for a
	// server-side reason (i.e. not an invalid request), the API user is required
	// to retry the request before performing a different SetLeaves request.
	SetLeaves(context.Context, *SetMapLeavesRequest) (*SetMapLeavesResponse, error)
	GetSignedMapRoot(context.Context, *GetSignedMapRootRequest) (*GetSignedMapRootResponse, error)
	GetSignedMapRootByRevision(context.Context, *GetSignedMapRootByRevisionRequest) (*GetSignedMapRootResponse, error)
	InitMap(context.Context, *InitMapRequest) (*InitMapResponse, error)
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

func _TrillianMap_InitMap_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(InitMapRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TrillianMapServer).InitMap(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/trillian.TrillianMap/InitMap",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TrillianMapServer).InitMap(ctx, req.(*InitMapRequest))
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
		{
			MethodName: "InitMap",
			Handler:    _TrillianMap_InitMap_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "trillian_map_api.proto",
}
