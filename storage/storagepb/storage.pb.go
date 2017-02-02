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

// NodeIDProto is the serialized form of NodeID. It's used only for persistence in storage.
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
	// This structure is usually used in RAM as a cache, the internal nodes of
	// the subtree are not generally stored. However internal nodes are stored for
	// partially filled log subtrees.
	InternalNodes map[string][]byte `protobuf:"bytes,5,rep,name=internal_nodes,json=internalNodes" json:"internal_nodes,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value,proto3"`
	// Used as a crosscheck on the internal node map by recording its expected size
	InternalNodeCount uint32 `protobuf:"varint,6,opt,name=internal_node_count,json=internalNodeCount" json:"internal_node_count,omitempty"`
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

func (m *SubtreeProto) GetInternalNodeCount() uint32 {
	if m != nil {
		return m.InternalNodeCount
	}
	return 0
}

func init() {
	proto.RegisterType((*NodeIDProto)(nil), "storagepb.NodeIDProto")
	proto.RegisterType((*SubtreeProto)(nil), "storagepb.SubtreeProto")
}

func init() { proto.RegisterFile("storage.proto", fileDescriptor0) }

var fileDescriptor0 = []byte{
	// 308 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x09, 0x6e, 0x88, 0x02, 0xff, 0x94, 0x91, 0x4d, 0x4b, 0xc3, 0x40,
	0x10, 0x86, 0x49, 0xd3, 0x06, 0x3b, 0x6d, 0xfc, 0x58, 0x45, 0x96, 0x7a, 0x09, 0x15, 0x24, 0x78,
	0xc8, 0x41, 0x2f, 0x7e, 0x5c, 0xc4, 0x0f, 0xb0, 0x50, 0x44, 0xd7, 0x1f, 0x10, 0x36, 0x66, 0x34,
	0xc1, 0xb0, 0x1b, 0x76, 0x37, 0xc5, 0xfe, 0x11, 0x7f, 0xaf, 0x64, 0xb3, 0x48, 0x44, 0x3c, 0x78,
	0x9b, 0x77, 0xe6, 0x9d, 0x27, 0x93, 0x77, 0x21, 0xd4, 0x46, 0x2a, 0xfe, 0x86, 0x49, 0xad, 0xa4,
	0x91, 0x64, 0xec, 0x64, 0x9d, 0xcd, 0x17, 0x30, 0x79, 0x90, 0x39, 0x2e, 0x6e, 0x1f, 0xed, 0x84,
	0xc0, 0xb0, 0xe6, 0xa6, 0xa0, 0x5e, 0xe4, 0xc5, 0x53, 0x66, 0x6b, 0x72, 0x04, 0x5b, 0xb5, 0xc2,
	0xd7, 0xf2, 0x23, 0xad, 0x50, 0xa4, 0x59, 0x69, 0x34, 0x1d, 0x44, 0x5e, 0x3c, 0x62, 0x61, 0xd7,
	0x5e, 0xa2, 0xb8, 0x2e, 0x8d, 0x9e, 0x7f, 0xfa, 0x30, 0x7d, 0x6e, 0x32, 0xa3, 0x10, 0x3b, 0xd8,
	0x3e, 0x04, 0x9d, 0xc3, 0xe1, 0x9c, 0x22, 0x7b, 0x30, 0xca, 0xb1, 0x36, 0x85, 0xc3, 0x74, 0x82,
	0x1c, 0xc0, 0x58, 0x49, 0x69, 0xd2, 0x82, 0xeb, 0x82, 0xfa, 0x76, 0x61, 0xa3, 0x6d, 0xdc, 0x73,
	0x5d, 0x90, 0x4b, 0x08, 0x2a, 0xe4, 0x2b, 0xd4, 0x74, 0x18, 0xf9, 0xf1, 0xe4, 0xe4, 0x30, 0xf9,
	0xfe, 0x85, 0xa4, 0xff, 0xcd, 0x64, 0x69, 0x5d, 0x77, 0xc2, 0xa8, 0x35, 0x73, 0x2b, 0xe4, 0x09,
	0x36, 0x4b, 0x61, 0x50, 0x09, 0x5e, 0xa5, 0x42, 0xe6, 0xa8, 0xe9, 0xc8, 0x42, 0x8e, 0xff, 0x82,
	0x2c, 0x9c, 0xbb, 0x4d, 0xc6, 0xb1, 0xc2, 0xb2, 0xdf, 0x23, 0x09, 0xec, 0xfe, 0x40, 0xa6, 0x2f,
	0xb2, 0x11, 0x86, 0x06, 0x91, 0x17, 0x87, 0x6c, 0xa7, 0xef, 0xbd, 0x69, 0x07, 0xb3, 0x73, 0x98,
	0xf4, 0x2e, 0x23, 0xdb, 0xe0, 0xbf, 0xe3, 0xda, 0xc6, 0x32, 0x66, 0x6d, 0xd9, 0x66, 0xb2, 0xe2,
	0x55, 0x83, 0x36, 0x93, 0x29, 0xeb, 0xc4, 0xc5, 0xe0, 0xcc, 0x9b, 0x5d, 0x01, 0xf9, 0x7d, 0xcf,
	0x7f, 0x08, 0x59, 0x60, 0x5f, 0xfd, 0xf4, 0x2b, 0x00, 0x00, 0xff, 0xff, 0xf4, 0xae, 0x38, 0x20,
	0x06, 0x02, 0x00, 0x00,
}
