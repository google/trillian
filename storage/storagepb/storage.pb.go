// Code generated by protoc-gen-go.
// source: storage.proto
// DO NOT EDIT!

/*
Package storagepb is a generated protocol buffer package.

It is generated from these files:
	storage.proto

It has these top-level messages:
	NodeIDProto
	SubtreeProto
*/
package storagepb

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

// NodeIDProto is the serialised form of NodeID. It's used only for persistence in storage.
// As this is long-term we prefer not to use a Go specific format.
type NodeIDProto struct {
	Path          []byte `protobuf:"bytes,1,opt,name=path,proto3" json:"path,omitempty"`
	PrefixLenBits int32  `protobuf:"varint,2,opt,name=prefix_len_bits,json=prefixLenBits" json:"prefix_len_bits,omitempty"`
}

func (m *NodeIDProto) Reset()                    { *m = NodeIDProto{} }
func (m *NodeIDProto) String() string            { return proto.CompactTextString(m) }
func (*NodeIDProto) ProtoMessage()               {}
func (*NodeIDProto) Descriptor() ([]byte, []int) { return fileDescriptor0, []int{0} }

func (m *NodeIDProto) GetPath() []byte {
	if m != nil {
		return m.Path
	}
	return nil
}

func (m *NodeIDProto) GetPrefixLenBits() int32 {
	if m != nil {
		return m.PrefixLenBits
	}
	return 0
}

// SubtreeProto contains nodes of a subtree.
type SubtreeProto struct {
	// subtree's prefix (must be a multiple of 8 bits)
	Prefix []byte `protobuf:"bytes,1,opt,name=prefix,proto3" json:"prefix,omitempty"`
	// subtree's depth
	Depth    int32  `protobuf:"varint,2,opt,name=depth" json:"depth,omitempty"`
	RootHash []byte `protobuf:"bytes,3,opt,name=root_hash,json=rootHash,proto3" json:"root_hash,omitempty"`
	// map of suffix (within subtree) to subtree-leaf node hash
	Leaves map[string][]byte `protobuf:"bytes,4,rep,name=leaves" json:"leaves,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value,proto3"`
	// Map of suffix (within subtree) to subtree-internal node hash.
	// This structure is only used in RAM as a cache, the internal nodes of
	// the subtree are not generally stored.
	InternalNodes map[string][]byte `protobuf:"bytes,5,rep,name=internal_nodes,json=internalNodes" json:"internal_nodes,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value,proto3"`
}

func (m *SubtreeProto) Reset()                    { *m = SubtreeProto{} }
func (m *SubtreeProto) String() string            { return proto.CompactTextString(m) }
func (*SubtreeProto) ProtoMessage()               {}
func (*SubtreeProto) Descriptor() ([]byte, []int) { return fileDescriptor0, []int{1} }

func (m *SubtreeProto) GetPrefix() []byte {
	if m != nil {
		return m.Prefix
	}
	return nil
}

func (m *SubtreeProto) GetDepth() int32 {
	if m != nil {
		return m.Depth
	}
	return 0
}

func (m *SubtreeProto) GetRootHash() []byte {
	if m != nil {
		return m.RootHash
	}
	return nil
}

func (m *SubtreeProto) GetLeaves() map[string][]byte {
	if m != nil {
		return m.Leaves
	}
	return nil
}

func (m *SubtreeProto) GetInternalNodes() map[string][]byte {
	if m != nil {
		return m.InternalNodes
	}
	return nil
}

func init() {
	proto.RegisterType((*NodeIDProto)(nil), "storagepb.NodeIDProto")
	proto.RegisterType((*SubtreeProto)(nil), "storagepb.SubtreeProto")
}

func init() { proto.RegisterFile("storage.proto", fileDescriptor0) }

var fileDescriptor0 = []byte{
	// 286 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x09, 0x6e, 0x88, 0x02, 0xff, 0x94, 0x91, 0x4f, 0x4b, 0xb5, 0x40,
	0x14, 0xc6, 0x51, 0xaf, 0xf2, 0x7a, 0xd4, 0xb7, 0x18, 0x22, 0xe4, 0xb6, 0x91, 0x1b, 0x84, 0xb4,
	0x70, 0x51, 0x9b, 0xfe, 0x6c, 0x22, 0x0a, 0x12, 0x2e, 0x51, 0xf6, 0x01, 0x64, 0xc4, 0x53, 0x4a,
	0x32, 0x23, 0x33, 0x73, 0x2f, 0xdd, 0x2f, 0xdc, 0xe7, 0x08, 0x67, 0x86, 0x10, 0xa2, 0x45, 0xbb,
	0xf3, 0x3c, 0x3e, 0xe7, 0x77, 0x78, 0x1c, 0x48, 0xa4, 0xe2, 0x82, 0xbe, 0x61, 0x31, 0x0a, 0xae,
	0x38, 0x09, 0xad, 0x1c, 0x9b, 0x55, 0x09, 0xd1, 0x23, 0x6f, 0xb1, 0xbc, 0x7b, 0xd2, 0x5f, 0x08,
	0x2c, 0x46, 0xaa, 0xba, 0xd4, 0xc9, 0x9c, 0x3c, 0xae, 0xf4, 0x4c, 0x4e, 0x60, 0x6f, 0x14, 0xf8,
	0xda, 0x7f, 0xd4, 0x03, 0xb2, 0xba, 0xe9, 0x95, 0x4c, 0xdd, 0xcc, 0xc9, 0xfd, 0x2a, 0x31, 0xf6,
	0x1a, 0xd9, 0x6d, 0xaf, 0xe4, 0xea, 0xd3, 0x85, 0xf8, 0x65, 0xd3, 0x28, 0x81, 0x68, 0x60, 0x87,
	0x10, 0x98, 0x84, 0xc5, 0x59, 0x45, 0x0e, 0xc0, 0x6f, 0x71, 0x54, 0x9d, 0xc5, 0x18, 0x41, 0x8e,
	0x20, 0x14, 0x9c, 0xab, 0xba, 0xa3, 0xb2, 0x4b, 0x3d, 0xbd, 0xf0, 0x6f, 0x32, 0x1e, 0xa8, 0xec,
	0xc8, 0x35, 0x04, 0x03, 0xd2, 0x2d, 0xca, 0x74, 0x91, 0x79, 0x79, 0x74, 0x76, 0x5c, 0x7c, 0x57,
	0x28, 0xe6, 0x37, 0x8b, 0xb5, 0x4e, 0xdd, 0x33, 0x25, 0x76, 0x95, 0x5d, 0x21, 0xcf, 0xf0, 0xbf,
	0x67, 0x0a, 0x05, 0xa3, 0x43, 0xcd, 0x78, 0x8b, 0x32, 0xf5, 0x35, 0xe4, 0xf4, 0x37, 0x48, 0x69,
	0xd3, 0xd3, 0x9f, 0xb1, 0xac, 0xa4, 0x9f, 0x7b, 0xcb, 0x4b, 0x88, 0x66, 0x97, 0xc8, 0x3e, 0x78,
	0xef, 0xb8, 0xd3, 0x35, 0xc3, 0x6a, 0x1a, 0xa7, 0x8e, 0x5b, 0x3a, 0x6c, 0x50, 0x77, 0x8c, 0x2b,
	0x23, 0xae, 0xdc, 0x0b, 0x67, 0x79, 0x03, 0xe4, 0x27, 0xff, 0x2f, 0x84, 0x26, 0xd0, 0xaf, 0x78,
	0xfe, 0x15, 0x00, 0x00, 0xff, 0xff, 0x1b, 0x68, 0x1d, 0xaa, 0xd6, 0x01, 0x00, 0x00,
}
