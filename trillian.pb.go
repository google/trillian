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
// source: trillian.proto

package trillian

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	anypb "google.golang.org/protobuf/types/known/anypb"
	durationpb "google.golang.org/protobuf/types/known/durationpb"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
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

// LogRootFormat specifies the fields that are covered by the
// SignedLogRoot signature, as well as their ordering and formats.
type LogRootFormat int32

const (
	LogRootFormat_LOG_ROOT_FORMAT_UNKNOWN LogRootFormat = 0
	LogRootFormat_LOG_ROOT_FORMAT_V1      LogRootFormat = 1
)

// Enum value maps for LogRootFormat.
var (
	LogRootFormat_name = map[int32]string{
		0: "LOG_ROOT_FORMAT_UNKNOWN",
		1: "LOG_ROOT_FORMAT_V1",
	}
	LogRootFormat_value = map[string]int32{
		"LOG_ROOT_FORMAT_UNKNOWN": 0,
		"LOG_ROOT_FORMAT_V1":      1,
	}
)

func (x LogRootFormat) Enum() *LogRootFormat {
	p := new(LogRootFormat)
	*p = x
	return p
}

func (x LogRootFormat) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (LogRootFormat) Descriptor() protoreflect.EnumDescriptor {
	return file_trillian_proto_enumTypes[0].Descriptor()
}

func (LogRootFormat) Type() protoreflect.EnumType {
	return &file_trillian_proto_enumTypes[0]
}

func (x LogRootFormat) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use LogRootFormat.Descriptor instead.
func (LogRootFormat) EnumDescriptor() ([]byte, []int) {
	return file_trillian_proto_rawDescGZIP(), []int{0}
}

// Defines the way empty / node / leaf hashes are constructed incorporating
// preimage protection, which can be application specific.
type HashStrategy int32

const (
	// Hash strategy cannot be determined. Included to enable detection of
	// mismatched proto versions being used. Represents an invalid value.
	HashStrategy_UNKNOWN_HASH_STRATEGY HashStrategy = 0
	// Certificate Transparency strategy: leaf hash prefix = 0x00, node prefix =
	// 0x01, empty hash is digest([]byte{}), as defined in the specification.
	HashStrategy_RFC6962_SHA256 HashStrategy = 1
	// Sparse Merkle Tree strategy:  leaf hash prefix = 0x00, node prefix = 0x01,
	// empty branch is recursively computed from empty leaf nodes.
	// NOT secure in a multi tree environment. For testing only.
	HashStrategy_TEST_MAP_HASHER HashStrategy = 2
	// Append-only log strategy where leaf nodes are defined as the ObjectHash.
	// All other properties are equal to RFC6962_SHA256.
	HashStrategy_OBJECT_RFC6962_SHA256 HashStrategy = 3
	// The CONIKS sparse tree hasher with SHA512_256 as the hash algorithm.
	HashStrategy_CONIKS_SHA512_256 HashStrategy = 4
	// The CONIKS sparse tree hasher with SHA256 as the hash algorithm.
	HashStrategy_CONIKS_SHA256 HashStrategy = 5
)

// Enum value maps for HashStrategy.
var (
	HashStrategy_name = map[int32]string{
		0: "UNKNOWN_HASH_STRATEGY",
		1: "RFC6962_SHA256",
		2: "TEST_MAP_HASHER",
		3: "OBJECT_RFC6962_SHA256",
		4: "CONIKS_SHA512_256",
		5: "CONIKS_SHA256",
	}
	HashStrategy_value = map[string]int32{
		"UNKNOWN_HASH_STRATEGY": 0,
		"RFC6962_SHA256":        1,
		"TEST_MAP_HASHER":       2,
		"OBJECT_RFC6962_SHA256": 3,
		"CONIKS_SHA512_256":     4,
		"CONIKS_SHA256":         5,
	}
)

func (x HashStrategy) Enum() *HashStrategy {
	p := new(HashStrategy)
	*p = x
	return p
}

func (x HashStrategy) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (HashStrategy) Descriptor() protoreflect.EnumDescriptor {
	return file_trillian_proto_enumTypes[1].Descriptor()
}

func (HashStrategy) Type() protoreflect.EnumType {
	return &file_trillian_proto_enumTypes[1]
}

func (x HashStrategy) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use HashStrategy.Descriptor instead.
func (HashStrategy) EnumDescriptor() ([]byte, []int) {
	return file_trillian_proto_rawDescGZIP(), []int{1}
}

// State of the tree.
type TreeState int32

const (
	// Tree state cannot be determined. Included to enable detection of
	// mismatched proto versions being used. Represents an invalid value.
	TreeState_UNKNOWN_TREE_STATE TreeState = 0
	// Active trees are able to respond to both read and write requests.
	TreeState_ACTIVE TreeState = 1
	// Frozen trees are only able to respond to read requests, writing to a frozen
	// tree is forbidden. Trees should not be frozen when there are entries
	// in the queue that have not yet been integrated. See the DRAINING
	// state for this case.
	TreeState_FROZEN TreeState = 2
	// Deprecated: now tracked in Tree.deleted.
	//
	// Deprecated: Marked as deprecated in trillian.proto.
	TreeState_DEPRECATED_SOFT_DELETED TreeState = 3
	// Deprecated: now tracked in Tree.deleted.
	//
	// Deprecated: Marked as deprecated in trillian.proto.
	TreeState_DEPRECATED_HARD_DELETED TreeState = 4
	// A tree that is draining will continue to integrate queued entries.
	// No new entries should be accepted.
	TreeState_DRAINING TreeState = 5
)

// Enum value maps for TreeState.
var (
	TreeState_name = map[int32]string{
		0: "UNKNOWN_TREE_STATE",
		1: "ACTIVE",
		2: "FROZEN",
		3: "DEPRECATED_SOFT_DELETED",
		4: "DEPRECATED_HARD_DELETED",
		5: "DRAINING",
	}
	TreeState_value = map[string]int32{
		"UNKNOWN_TREE_STATE":      0,
		"ACTIVE":                  1,
		"FROZEN":                  2,
		"DEPRECATED_SOFT_DELETED": 3,
		"DEPRECATED_HARD_DELETED": 4,
		"DRAINING":                5,
	}
)

func (x TreeState) Enum() *TreeState {
	p := new(TreeState)
	*p = x
	return p
}

func (x TreeState) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (TreeState) Descriptor() protoreflect.EnumDescriptor {
	return file_trillian_proto_enumTypes[2].Descriptor()
}

func (TreeState) Type() protoreflect.EnumType {
	return &file_trillian_proto_enumTypes[2]
}

func (x TreeState) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use TreeState.Descriptor instead.
func (TreeState) EnumDescriptor() ([]byte, []int) {
	return file_trillian_proto_rawDescGZIP(), []int{2}
}

// Type of the tree.
type TreeType int32

const (
	// Tree type cannot be determined. Included to enable detection of mismatched
	// proto versions being used. Represents an invalid value.
	TreeType_UNKNOWN_TREE_TYPE TreeType = 0
	// Tree represents a verifiable log.
	TreeType_LOG TreeType = 1
	// Tree represents a verifiable pre-ordered log, i.e., a log whose entries are
	// placed according to sequence numbers assigned outside of Trillian.
	TreeType_PREORDERED_LOG TreeType = 3
)

// Enum value maps for TreeType.
var (
	TreeType_name = map[int32]string{
		0: "UNKNOWN_TREE_TYPE",
		1: "LOG",
		3: "PREORDERED_LOG",
	}
	TreeType_value = map[string]int32{
		"UNKNOWN_TREE_TYPE": 0,
		"LOG":               1,
		"PREORDERED_LOG":    3,
	}
)

func (x TreeType) Enum() *TreeType {
	p := new(TreeType)
	*p = x
	return p
}

func (x TreeType) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (TreeType) Descriptor() protoreflect.EnumDescriptor {
	return file_trillian_proto_enumTypes[3].Descriptor()
}

func (TreeType) Type() protoreflect.EnumType {
	return &file_trillian_proto_enumTypes[3]
}

func (x TreeType) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use TreeType.Descriptor instead.
func (TreeType) EnumDescriptor() ([]byte, []int) {
	return file_trillian_proto_rawDescGZIP(), []int{3}
}

// Represents a tree.
// Readonly attributes are assigned at tree creation, after which they may not
// be modified.
//
// Note: Many APIs within the rest of the code require these objects to
// be provided. For safety they should be obtained via Admin API calls and
// not created dynamically.
type Tree struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// ID of the tree.
	// Readonly.
	TreeId int64 `protobuf:"varint,1,opt,name=tree_id,json=treeId,proto3" json:"tree_id,omitempty"`
	// State of the tree.
	// Trees are ACTIVE after creation. At any point the tree may transition
	// between ACTIVE, DRAINING and FROZEN states.
	TreeState TreeState `protobuf:"varint,2,opt,name=tree_state,json=treeState,proto3,enum=trillian.TreeState" json:"tree_state,omitempty"`
	// Type of the tree.
	// Readonly after Tree creation. Exception: Can be switched from
	// PREORDERED_LOG to LOG if the Tree is and remains in the FROZEN state.
	TreeType TreeType `protobuf:"varint,3,opt,name=tree_type,json=treeType,proto3,enum=trillian.TreeType" json:"tree_type,omitempty"`
	// Display name of the tree.
	// Optional.
	DisplayName string `protobuf:"bytes,8,opt,name=display_name,json=displayName,proto3" json:"display_name,omitempty"`
	// Description of the tree,
	// Optional.
	Description string `protobuf:"bytes,9,opt,name=description,proto3" json:"description,omitempty"`
	// Storage-specific settings.
	// Varies according to the storage implementation backing Trillian.
	StorageSettings *anypb.Any `protobuf:"bytes,13,opt,name=storage_settings,json=storageSettings,proto3" json:"storage_settings,omitempty"`
	// Interval after which a new signed root is produced even if there have been
	// no submission.  If zero, this behavior is disabled.
	MaxRootDuration *durationpb.Duration `protobuf:"bytes,15,opt,name=max_root_duration,json=maxRootDuration,proto3" json:"max_root_duration,omitempty"`
	// Time of tree creation.
	// Readonly.
	CreateTime *timestamppb.Timestamp `protobuf:"bytes,16,opt,name=create_time,json=createTime,proto3" json:"create_time,omitempty"`
	// Time of last tree update.
	// Readonly (automatically assigned on updates).
	UpdateTime *timestamppb.Timestamp `protobuf:"bytes,17,opt,name=update_time,json=updateTime,proto3" json:"update_time,omitempty"`
	// If true, the tree has been deleted.
	// Deleted trees may be undeleted during a certain time window, after which
	// they're permanently deleted (and unrecoverable).
	// Readonly.
	Deleted bool `protobuf:"varint,19,opt,name=deleted,proto3" json:"deleted,omitempty"`
	// Time of tree deletion, if any.
	// Readonly.
	DeleteTime    *timestamppb.Timestamp `protobuf:"bytes,20,opt,name=delete_time,json=deleteTime,proto3" json:"delete_time,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Tree) Reset() {
	*x = Tree{}
	mi := &file_trillian_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Tree) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Tree) ProtoMessage() {}

func (x *Tree) ProtoReflect() protoreflect.Message {
	mi := &file_trillian_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Tree.ProtoReflect.Descriptor instead.
func (*Tree) Descriptor() ([]byte, []int) {
	return file_trillian_proto_rawDescGZIP(), []int{0}
}

func (x *Tree) GetTreeId() int64 {
	if x != nil {
		return x.TreeId
	}
	return 0
}

func (x *Tree) GetTreeState() TreeState {
	if x != nil {
		return x.TreeState
	}
	return TreeState_UNKNOWN_TREE_STATE
}

func (x *Tree) GetTreeType() TreeType {
	if x != nil {
		return x.TreeType
	}
	return TreeType_UNKNOWN_TREE_TYPE
}

func (x *Tree) GetDisplayName() string {
	if x != nil {
		return x.DisplayName
	}
	return ""
}

func (x *Tree) GetDescription() string {
	if x != nil {
		return x.Description
	}
	return ""
}

func (x *Tree) GetStorageSettings() *anypb.Any {
	if x != nil {
		return x.StorageSettings
	}
	return nil
}

func (x *Tree) GetMaxRootDuration() *durationpb.Duration {
	if x != nil {
		return x.MaxRootDuration
	}
	return nil
}

func (x *Tree) GetCreateTime() *timestamppb.Timestamp {
	if x != nil {
		return x.CreateTime
	}
	return nil
}

func (x *Tree) GetUpdateTime() *timestamppb.Timestamp {
	if x != nil {
		return x.UpdateTime
	}
	return nil
}

func (x *Tree) GetDeleted() bool {
	if x != nil {
		return x.Deleted
	}
	return false
}

func (x *Tree) GetDeleteTime() *timestamppb.Timestamp {
	if x != nil {
		return x.DeleteTime
	}
	return nil
}

// SignedLogRoot represents a commitment by a Log to a particular tree.
//
// Note that the signature itself is no-longer provided by Trillian since
// https://github.com/google/trillian/pull/2452 .
// This functionality was intended to support a niche-use case but added
// significant complexity and was prone to causing confusion and
// misunderstanding for personality authors.
type SignedLogRoot struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// log_root holds the TLS-serialization of the following structure (described
	// in RFC5246 notation):
	//
	// enum { v1(1), (65535)} Version;
	//
	//	struct {
	//	  uint64 tree_size;
	//	  opaque root_hash<0..128>;
	//	  uint64 timestamp_nanos;
	//	  uint64 revision;
	//	  opaque metadata<0..65535>;
	//	} LogRootV1;
	//
	//	struct {
	//	  Version version;
	//	  select(version) {
	//	    case v1: LogRootV1;
	//	  }
	//	} LogRoot;
	//
	// A serialized v1 log root will therefore be laid out as:
	//
	// +---+---+---+---+---+---+---+---+---+---+---+---+---+---+-....--+
	// | ver=1 |          tree_size            |len|    root_hash      |
	// +---+---+---+---+---+---+---+---+---+---+---+---+---+---+-....--+
	//
	// +---+---+---+---+---+---+---+---+---+---+---+---+---+---+---+---+
	// |        timestamp_nanos        |      revision                 |
	// +---+---+---+---+---+---+---+---+---+---+---+---+---+---+---+---+
	//
	// +---+---+---+---+---+-....---+
	// |  len  |    metadata        |
	// +---+---+---+---+---+-....---+
	//
	// (with all integers encoded big-endian).
	LogRoot       []byte `protobuf:"bytes,8,opt,name=log_root,json=logRoot,proto3" json:"log_root,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *SignedLogRoot) Reset() {
	*x = SignedLogRoot{}
	mi := &file_trillian_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SignedLogRoot) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SignedLogRoot) ProtoMessage() {}

func (x *SignedLogRoot) ProtoReflect() protoreflect.Message {
	mi := &file_trillian_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SignedLogRoot.ProtoReflect.Descriptor instead.
func (*SignedLogRoot) Descriptor() ([]byte, []int) {
	return file_trillian_proto_rawDescGZIP(), []int{1}
}

func (x *SignedLogRoot) GetLogRoot() []byte {
	if x != nil {
		return x.LogRoot
	}
	return nil
}

// Proof holds a consistency or inclusion proof for a Merkle tree, as returned
// by the API.
type Proof struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// leaf_index indicates the requested leaf index when this message is used for
	// a leaf inclusion proof.  This field is set to zero when this message is
	// used for a consistency proof.
	LeafIndex     int64    `protobuf:"varint,1,opt,name=leaf_index,json=leafIndex,proto3" json:"leaf_index,omitempty"`
	Hashes        [][]byte `protobuf:"bytes,3,rep,name=hashes,proto3" json:"hashes,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Proof) Reset() {
	*x = Proof{}
	mi := &file_trillian_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Proof) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Proof) ProtoMessage() {}

func (x *Proof) ProtoReflect() protoreflect.Message {
	mi := &file_trillian_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Proof.ProtoReflect.Descriptor instead.
func (*Proof) Descriptor() ([]byte, []int) {
	return file_trillian_proto_rawDescGZIP(), []int{2}
}

func (x *Proof) GetLeafIndex() int64 {
	if x != nil {
		return x.LeafIndex
	}
	return 0
}

func (x *Proof) GetHashes() [][]byte {
	if x != nil {
		return x.Hashes
	}
	return nil
}

var File_trillian_proto protoreflect.FileDescriptor

const file_trillian_proto_rawDesc = "" +
	"\n" +
	"\x0etrillian.proto\x12\btrillian\x1a\x19google/protobuf/any.proto\x1a\x1egoogle/protobuf/duration.proto\x1a\x1fgoogle/protobuf/timestamp.proto\"\xf1\x05\n" +
	"\x04Tree\x12\x17\n" +
	"\atree_id\x18\x01 \x01(\x03R\x06treeId\x122\n" +
	"\n" +
	"tree_state\x18\x02 \x01(\x0e2\x13.trillian.TreeStateR\ttreeState\x12/\n" +
	"\ttree_type\x18\x03 \x01(\x0e2\x12.trillian.TreeTypeR\btreeType\x12!\n" +
	"\fdisplay_name\x18\b \x01(\tR\vdisplayName\x12 \n" +
	"\vdescription\x18\t \x01(\tR\vdescription\x12?\n" +
	"\x10storage_settings\x18\r \x01(\v2\x14.google.protobuf.AnyR\x0fstorageSettings\x12E\n" +
	"\x11max_root_duration\x18\x0f \x01(\v2\x19.google.protobuf.DurationR\x0fmaxRootDuration\x12;\n" +
	"\vcreate_time\x18\x10 \x01(\v2\x1a.google.protobuf.TimestampR\n" +
	"createTime\x12;\n" +
	"\vupdate_time\x18\x11 \x01(\v2\x1a.google.protobuf.TimestampR\n" +
	"updateTime\x12\x18\n" +
	"\adeleted\x18\x13 \x01(\bR\adeleted\x12;\n" +
	"\vdelete_time\x18\x14 \x01(\v2\x1a.google.protobuf.TimestampR\n" +
	"deleteTimeJ\x04\b\x04\x10\bJ\x04\b\n" +
	"\x10\rJ\x04\b\x0e\x10\x0fJ\x04\b\x12\x10\x13R\x1ecreate_time_millis_since_epochR\x10duplicate_policyR\x0ehash_algorithmR\rhash_strategyR\vprivate_keyR\n" +
	"public_keyR\x13signature_algorithmR\x16signature_cipher_suiteR\x1eupdate_time_millis_since_epoch\"\x9d\x01\n" +
	"\rSignedLogRoot\x12\x19\n" +
	"\blog_root\x18\b \x01(\fR\alogRootJ\x04\b\x01\x10\bJ\x04\b\t\x10\n" +
	"R\bkey_hintR\x06log_idR\x12log_root_signatureR\troot_hashR\tsignatureR\x0ftimestamp_nanosR\rtree_revisionR\ttree_size\"P\n" +
	"\x05Proof\x12\x1d\n" +
	"\n" +
	"leaf_index\x18\x01 \x01(\x03R\tleafIndex\x12\x16\n" +
	"\x06hashes\x18\x03 \x03(\fR\x06hashesJ\x04\b\x02\x10\x03R\n" +
	"proof_node*D\n" +
	"\rLogRootFormat\x12\x1b\n" +
	"\x17LOG_ROOT_FORMAT_UNKNOWN\x10\x00\x12\x16\n" +
	"\x12LOG_ROOT_FORMAT_V1\x10\x01*\x97\x01\n" +
	"\fHashStrategy\x12\x19\n" +
	"\x15UNKNOWN_HASH_STRATEGY\x10\x00\x12\x12\n" +
	"\x0eRFC6962_SHA256\x10\x01\x12\x13\n" +
	"\x0fTEST_MAP_HASHER\x10\x02\x12\x19\n" +
	"\x15OBJECT_RFC6962_SHA256\x10\x03\x12\x15\n" +
	"\x11CONIKS_SHA512_256\x10\x04\x12\x11\n" +
	"\rCONIKS_SHA256\x10\x05*\x8b\x01\n" +
	"\tTreeState\x12\x16\n" +
	"\x12UNKNOWN_TREE_STATE\x10\x00\x12\n" +
	"\n" +
	"\x06ACTIVE\x10\x01\x12\n" +
	"\n" +
	"\x06FROZEN\x10\x02\x12\x1f\n" +
	"\x17DEPRECATED_SOFT_DELETED\x10\x03\x1a\x02\b\x01\x12\x1f\n" +
	"\x17DEPRECATED_HARD_DELETED\x10\x04\x1a\x02\b\x01\x12\f\n" +
	"\bDRAINING\x10\x05*I\n" +
	"\bTreeType\x12\x15\n" +
	"\x11UNKNOWN_TREE_TYPE\x10\x00\x12\a\n" +
	"\x03LOG\x10\x01\x12\x12\n" +
	"\x0ePREORDERED_LOG\x10\x03\"\x04\b\x02\x10\x02*\x03MAPBH\n" +
	"\x19com.google.trillian.protoB\rTrillianProtoP\x01Z\x1agithub.com/google/trillianb\x06proto3"

var (
	file_trillian_proto_rawDescOnce sync.Once
	file_trillian_proto_rawDescData []byte
)

func file_trillian_proto_rawDescGZIP() []byte {
	file_trillian_proto_rawDescOnce.Do(func() {
		file_trillian_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_trillian_proto_rawDesc), len(file_trillian_proto_rawDesc)))
	})
	return file_trillian_proto_rawDescData
}

var file_trillian_proto_enumTypes = make([]protoimpl.EnumInfo, 4)
var file_trillian_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_trillian_proto_goTypes = []any{
	(LogRootFormat)(0),            // 0: trillian.LogRootFormat
	(HashStrategy)(0),             // 1: trillian.HashStrategy
	(TreeState)(0),                // 2: trillian.TreeState
	(TreeType)(0),                 // 3: trillian.TreeType
	(*Tree)(nil),                  // 4: trillian.Tree
	(*SignedLogRoot)(nil),         // 5: trillian.SignedLogRoot
	(*Proof)(nil),                 // 6: trillian.Proof
	(*anypb.Any)(nil),             // 7: google.protobuf.Any
	(*durationpb.Duration)(nil),   // 8: google.protobuf.Duration
	(*timestamppb.Timestamp)(nil), // 9: google.protobuf.Timestamp
}
var file_trillian_proto_depIdxs = []int32{
	2, // 0: trillian.Tree.tree_state:type_name -> trillian.TreeState
	3, // 1: trillian.Tree.tree_type:type_name -> trillian.TreeType
	7, // 2: trillian.Tree.storage_settings:type_name -> google.protobuf.Any
	8, // 3: trillian.Tree.max_root_duration:type_name -> google.protobuf.Duration
	9, // 4: trillian.Tree.create_time:type_name -> google.protobuf.Timestamp
	9, // 5: trillian.Tree.update_time:type_name -> google.protobuf.Timestamp
	9, // 6: trillian.Tree.delete_time:type_name -> google.protobuf.Timestamp
	7, // [7:7] is the sub-list for method output_type
	7, // [7:7] is the sub-list for method input_type
	7, // [7:7] is the sub-list for extension type_name
	7, // [7:7] is the sub-list for extension extendee
	0, // [0:7] is the sub-list for field type_name
}

func init() { file_trillian_proto_init() }
func file_trillian_proto_init() {
	if File_trillian_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_trillian_proto_rawDesc), len(file_trillian_proto_rawDesc)),
			NumEnums:      4,
			NumMessages:   3,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_trillian_proto_goTypes,
		DependencyIndexes: file_trillian_proto_depIdxs,
		EnumInfos:         file_trillian_proto_enumTypes,
		MessageInfos:      file_trillian_proto_msgTypes,
	}.Build()
	File_trillian_proto = out.File
	file_trillian_proto_goTypes = nil
	file_trillian_proto_depIdxs = nil
}
