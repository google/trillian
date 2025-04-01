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

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.6
// 	protoc        v3.20.1
// source: storage.proto

package storagepb

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
	unsafe "unsafe"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// SubtreeProto contains nodes of a subtree.
type SubtreeProto struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// subtree's prefix (must be a multiple of 8 bits)
	Prefix []byte `protobuf:"bytes,1,opt,name=prefix,proto3" json:"prefix,omitempty"`
	// subtree's depth
	Depth int32 `protobuf:"varint,2,opt,name=depth,proto3" json:"depth,omitempty"`
	// map of suffix (within subtree) to subtree-leaf node hash
	Leaves map[string][]byte `protobuf:"bytes,4,rep,name=leaves,proto3" json:"leaves,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value"`
	// Map of suffix (within subtree) to subtree-internal node hash.
	// This structure is usually used in RAM as a cache, the internal nodes of
	// the subtree are not generally stored. However internal nodes are stored for
	// partially filled log subtrees.
	InternalNodes map[string][]byte `protobuf:"bytes,5,rep,name=internal_nodes,json=internalNodes,proto3" json:"internal_nodes,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value"`
	// Used as a crosscheck on the internal node map by recording its expected
	// size after loading and repopulation.
	InternalNodeCount uint32 `protobuf:"varint,6,opt,name=internal_node_count,json=internalNodeCount,proto3" json:"internal_node_count,omitempty"`
	unknownFields     protoimpl.UnknownFields
	sizeCache         protoimpl.SizeCache
}

func (x *SubtreeProto) Reset() {
	*x = SubtreeProto{}
	mi := &file_storage_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SubtreeProto) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SubtreeProto) ProtoMessage() {}

func (x *SubtreeProto) ProtoReflect() protoreflect.Message {
	mi := &file_storage_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SubtreeProto.ProtoReflect.Descriptor instead.
func (*SubtreeProto) Descriptor() ([]byte, []int) {
	return file_storage_proto_rawDescGZIP(), []int{0}
}

func (x *SubtreeProto) GetPrefix() []byte {
	if x != nil {
		return x.Prefix
	}
	return nil
}

func (x *SubtreeProto) GetDepth() int32 {
	if x != nil {
		return x.Depth
	}
	return 0
}

func (x *SubtreeProto) GetLeaves() map[string][]byte {
	if x != nil {
		return x.Leaves
	}
	return nil
}

func (x *SubtreeProto) GetInternalNodes() map[string][]byte {
	if x != nil {
		return x.InternalNodes
	}
	return nil
}

func (x *SubtreeProto) GetInternalNodeCount() uint32 {
	if x != nil {
		return x.InternalNodeCount
	}
	return 0
}

var File_storage_proto protoreflect.FileDescriptor

const file_storage_proto_rawDesc = "" +
	"\n" +
	"\rstorage.proto\x12\tstoragepb\"\xff\x02\n" +
	"\fSubtreeProto\x12\x16\n" +
	"\x06prefix\x18\x01 \x01(\fR\x06prefix\x12\x14\n" +
	"\x05depth\x18\x02 \x01(\x05R\x05depth\x12;\n" +
	"\x06leaves\x18\x04 \x03(\v2#.storagepb.SubtreeProto.LeavesEntryR\x06leaves\x12Q\n" +
	"\x0einternal_nodes\x18\x05 \x03(\v2*.storagepb.SubtreeProto.InternalNodesEntryR\rinternalNodes\x12.\n" +
	"\x13internal_node_count\x18\x06 \x01(\rR\x11internalNodeCount\x1a9\n" +
	"\vLeavesEntry\x12\x10\n" +
	"\x03key\x18\x01 \x01(\tR\x03key\x12\x14\n" +
	"\x05value\x18\x02 \x01(\fR\x05value:\x028\x01\x1a@\n" +
	"\x12InternalNodesEntry\x12\x10\n" +
	"\x03key\x18\x01 \x01(\tR\x03key\x12\x14\n" +
	"\x05value\x18\x02 \x01(\fR\x05value:\x028\x01J\x04\b\x03\x10\x04B.Z,github.com/google/trillian/storage/storagepbb\x06proto3"

var (
	file_storage_proto_rawDescOnce sync.Once
	file_storage_proto_rawDescData []byte
)

func file_storage_proto_rawDescGZIP() []byte {
	file_storage_proto_rawDescOnce.Do(func() {
		file_storage_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_storage_proto_rawDesc), len(file_storage_proto_rawDesc)))
	})
	return file_storage_proto_rawDescData
}

var file_storage_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_storage_proto_goTypes = []any{
	(*SubtreeProto)(nil), // 0: storagepb.SubtreeProto
	nil,                  // 1: storagepb.SubtreeProto.LeavesEntry
	nil,                  // 2: storagepb.SubtreeProto.InternalNodesEntry
}
var file_storage_proto_depIdxs = []int32{
	1, // 0: storagepb.SubtreeProto.leaves:type_name -> storagepb.SubtreeProto.LeavesEntry
	2, // 1: storagepb.SubtreeProto.internal_nodes:type_name -> storagepb.SubtreeProto.InternalNodesEntry
	2, // [2:2] is the sub-list for method output_type
	2, // [2:2] is the sub-list for method input_type
	2, // [2:2] is the sub-list for extension type_name
	2, // [2:2] is the sub-list for extension extendee
	0, // [0:2] is the sub-list for field type_name
}

func init() { file_storage_proto_init() }
func file_storage_proto_init() {
	if File_storage_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_storage_proto_rawDesc), len(file_storage_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   3,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_storage_proto_goTypes,
		DependencyIndexes: file_storage_proto_depIdxs,
		MessageInfos:      file_storage_proto_msgTypes,
	}.Build()
	File_storage_proto = out.File
	file_storage_proto_goTypes = nil
	file_storage_proto_depIdxs = nil
}
