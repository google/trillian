// Code generated by protoc-gen-go. DO NOT EDIT.
// source: trillian.proto

package trillian

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"
import keyspb "github.com/google/trillian/crypto/keyspb"
import sigpb "github.com/google/trillian/crypto/sigpb"
import google_protobuf2 "github.com/golang/protobuf/ptypes/any"
import google_protobuf3 "github.com/golang/protobuf/ptypes/duration"
import google_protobuf1 "github.com/golang/protobuf/ptypes/timestamp"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// LogRootFormat specifies the fields that are covered by the
// SignedLogRoot signature, as well as their ordering and formats.
type LogRootFormat int32

const (
	LogRootFormat_LOG_ROOT_FORMAT_UNKNOWN LogRootFormat = 0
	LogRootFormat_LOG_ROOT_FORMAT_V1      LogRootFormat = 1
)

var LogRootFormat_name = map[int32]string{
	0: "LOG_ROOT_FORMAT_UNKNOWN",
	1: "LOG_ROOT_FORMAT_V1",
}
var LogRootFormat_value = map[string]int32{
	"LOG_ROOT_FORMAT_UNKNOWN": 0,
	"LOG_ROOT_FORMAT_V1":      1,
}

func (x LogRootFormat) String() string {
	return proto.EnumName(LogRootFormat_name, int32(x))
}
func (LogRootFormat) EnumDescriptor() ([]byte, []int) { return fileDescriptor3, []int{0} }

// MapRootFormat specifies the fields that are covered by the
// SignedMapRoot signature, as well as their ordering and formats.
type MapRootFormat int32

const (
	MapRootFormat_MAP_ROOT_FORMAT_UNKNOWN MapRootFormat = 0
	MapRootFormat_MAP_ROOT_FORMAT_V1      MapRootFormat = 1
)

var MapRootFormat_name = map[int32]string{
	0: "MAP_ROOT_FORMAT_UNKNOWN",
	1: "MAP_ROOT_FORMAT_V1",
}
var MapRootFormat_value = map[string]int32{
	"MAP_ROOT_FORMAT_UNKNOWN": 0,
	"MAP_ROOT_FORMAT_V1":      1,
}

func (x MapRootFormat) String() string {
	return proto.EnumName(MapRootFormat_name, int32(x))
}
func (MapRootFormat) EnumDescriptor() ([]byte, []int) { return fileDescriptor3, []int{1} }

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
)

var HashStrategy_name = map[int32]string{
	0: "UNKNOWN_HASH_STRATEGY",
	1: "RFC6962_SHA256",
	2: "TEST_MAP_HASHER",
	3: "OBJECT_RFC6962_SHA256",
	4: "CONIKS_SHA512_256",
}
var HashStrategy_value = map[string]int32{
	"UNKNOWN_HASH_STRATEGY": 0,
	"RFC6962_SHA256":        1,
	"TEST_MAP_HASHER":       2,
	"OBJECT_RFC6962_SHA256": 3,
	"CONIKS_SHA512_256":     4,
}

func (x HashStrategy) String() string {
	return proto.EnumName(HashStrategy_name, int32(x))
}
func (HashStrategy) EnumDescriptor() ([]byte, []int) { return fileDescriptor3, []int{2} }

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
	// Deprecated in favor of Tree.deleted.
	TreeState_DEPRECATED_SOFT_DELETED TreeState = 3
	// Deprecated in favor of Tree.deleted.
	TreeState_DEPRECATED_HARD_DELETED TreeState = 4
	// A tree that is draining will continue to integrate queued entries.
	// No new entries should be accepted.
	TreeState_DRAINING TreeState = 5
)

var TreeState_name = map[int32]string{
	0: "UNKNOWN_TREE_STATE",
	1: "ACTIVE",
	2: "FROZEN",
	3: "DEPRECATED_SOFT_DELETED",
	4: "DEPRECATED_HARD_DELETED",
	5: "DRAINING",
}
var TreeState_value = map[string]int32{
	"UNKNOWN_TREE_STATE":      0,
	"ACTIVE":                  1,
	"FROZEN":                  2,
	"DEPRECATED_SOFT_DELETED": 3,
	"DEPRECATED_HARD_DELETED": 4,
	"DRAINING":                5,
}

func (x TreeState) String() string {
	return proto.EnumName(TreeState_name, int32(x))
}
func (TreeState) EnumDescriptor() ([]byte, []int) { return fileDescriptor3, []int{3} }

// Type of the tree.
type TreeType int32

const (
	// Tree type cannot be determined. Included to enable detection of mismatched
	// proto versions being used. Represents an invalid value.
	TreeType_UNKNOWN_TREE_TYPE TreeType = 0
	// Tree represents a verifiable log.
	TreeType_LOG TreeType = 1
	// Tree represents a verifiable map.
	TreeType_MAP TreeType = 2
	// Tree represents a verifiable pre-ordered log, i.e., a log whose entries are
	// placed according to sequence numbers assigned outside of Trillian.
	// TODO(pavelkalinnikov): Support this type.
	TreeType_PREORDERED_LOG TreeType = 3
)

var TreeType_name = map[int32]string{
	0: "UNKNOWN_TREE_TYPE",
	1: "LOG",
	2: "MAP",
	3: "PREORDERED_LOG",
}
var TreeType_value = map[string]int32{
	"UNKNOWN_TREE_TYPE": 0,
	"LOG":               1,
	"MAP":               2,
	"PREORDERED_LOG":    3,
}

func (x TreeType) String() string {
	return proto.EnumName(TreeType_name, int32(x))
}
func (TreeType) EnumDescriptor() ([]byte, []int) { return fileDescriptor3, []int{4} }

// Represents a tree, which may be either a verifiable log or map.
// Readonly attributes are assigned at tree creation, after which they may not
// be modified.
//
// Note: Many APIs within the rest of the code require these objects to
// be provided. For safety they should be obtained via Admin API calls and
// not created dynamically.
type Tree struct {
	// ID of the tree.
	// Readonly.
	TreeId int64 `protobuf:"varint,1,opt,name=tree_id,json=treeId" json:"tree_id,omitempty"`
	// State of the tree.
	// Trees are active after creation. At any point the tree may transition
	// between ACTIVE and FROZEN.
	TreeState TreeState `protobuf:"varint,2,opt,name=tree_state,json=treeState,enum=trillian.TreeState" json:"tree_state,omitempty"`
	// Type of the tree.
	// Readonly.
	TreeType TreeType `protobuf:"varint,3,opt,name=tree_type,json=treeType,enum=trillian.TreeType" json:"tree_type,omitempty"`
	// Hash strategy to be used by the tree.
	// Readonly.
	HashStrategy HashStrategy `protobuf:"varint,4,opt,name=hash_strategy,json=hashStrategy,enum=trillian.HashStrategy" json:"hash_strategy,omitempty"`
	// Hash algorithm to be used by the tree.
	// Readonly.
	HashAlgorithm sigpb.DigitallySigned_HashAlgorithm `protobuf:"varint,5,opt,name=hash_algorithm,json=hashAlgorithm,enum=sigpb.DigitallySigned_HashAlgorithm" json:"hash_algorithm,omitempty"`
	// Signature algorithm to be used by the tree.
	// Readonly.
	SignatureAlgorithm sigpb.DigitallySigned_SignatureAlgorithm `protobuf:"varint,6,opt,name=signature_algorithm,json=signatureAlgorithm,enum=sigpb.DigitallySigned_SignatureAlgorithm" json:"signature_algorithm,omitempty"`
	// Display name of the tree.
	// Optional.
	DisplayName string `protobuf:"bytes,8,opt,name=display_name,json=displayName" json:"display_name,omitempty"`
	// Description of the tree,
	// Optional.
	Description string `protobuf:"bytes,9,opt,name=description" json:"description,omitempty"`
	// Identifies the private key used for signing tree heads and entry
	// timestamps.
	// This can be any type of message to accommodate different key management
	// systems, e.g. PEM files, HSMs, etc.
	// Private keys are write-only: they're never returned by RPCs.
	// The private_key message can be changed after a tree is created, but the
	// underlying key must remain the same - this is to enable migrating a key
	// from one provider to another.
	PrivateKey *google_protobuf2.Any `protobuf:"bytes,12,opt,name=private_key,json=privateKey" json:"private_key,omitempty"`
	// Storage-specific settings.
	// Varies according to the storage implementation backing Trillian.
	StorageSettings *google_protobuf2.Any `protobuf:"bytes,13,opt,name=storage_settings,json=storageSettings" json:"storage_settings,omitempty"`
	// The public key used for verifying tree heads and entry timestamps.
	// Readonly.
	PublicKey *keyspb.PublicKey `protobuf:"bytes,14,opt,name=public_key,json=publicKey" json:"public_key,omitempty"`
	// Interval after which a new signed root is produced even if there have been
	// no submission.  If zero, this behavior is disabled.
	MaxRootDuration *google_protobuf3.Duration `protobuf:"bytes,15,opt,name=max_root_duration,json=maxRootDuration" json:"max_root_duration,omitempty"`
	// Time of tree creation.
	// Readonly.
	CreateTime *google_protobuf1.Timestamp `protobuf:"bytes,16,opt,name=create_time,json=createTime" json:"create_time,omitempty"`
	// Time of last tree update.
	// Readonly (automatically assigned on updates).
	UpdateTime *google_protobuf1.Timestamp `protobuf:"bytes,17,opt,name=update_time,json=updateTime" json:"update_time,omitempty"`
	// If true, the tree has been deleted.
	// Deleted trees may be undeleted during a certain time window, after which
	// they're permanently deleted (and unrecoverable).
	// Readonly.
	Deleted bool `protobuf:"varint,19,opt,name=deleted" json:"deleted,omitempty"`
	// Time of tree deletion, if any.
	// Readonly.
	DeleteTime *google_protobuf1.Timestamp `protobuf:"bytes,20,opt,name=delete_time,json=deleteTime" json:"delete_time,omitempty"`
}

func (m *Tree) Reset()                    { *m = Tree{} }
func (m *Tree) String() string            { return proto.CompactTextString(m) }
func (*Tree) ProtoMessage()               {}
func (*Tree) Descriptor() ([]byte, []int) { return fileDescriptor3, []int{0} }

func (m *Tree) GetTreeId() int64 {
	if m != nil {
		return m.TreeId
	}
	return 0
}

func (m *Tree) GetTreeState() TreeState {
	if m != nil {
		return m.TreeState
	}
	return TreeState_UNKNOWN_TREE_STATE
}

func (m *Tree) GetTreeType() TreeType {
	if m != nil {
		return m.TreeType
	}
	return TreeType_UNKNOWN_TREE_TYPE
}

func (m *Tree) GetHashStrategy() HashStrategy {
	if m != nil {
		return m.HashStrategy
	}
	return HashStrategy_UNKNOWN_HASH_STRATEGY
}

func (m *Tree) GetHashAlgorithm() sigpb.DigitallySigned_HashAlgorithm {
	if m != nil {
		return m.HashAlgorithm
	}
	return sigpb.DigitallySigned_NONE
}

func (m *Tree) GetSignatureAlgorithm() sigpb.DigitallySigned_SignatureAlgorithm {
	if m != nil {
		return m.SignatureAlgorithm
	}
	return sigpb.DigitallySigned_ANONYMOUS
}

func (m *Tree) GetDisplayName() string {
	if m != nil {
		return m.DisplayName
	}
	return ""
}

func (m *Tree) GetDescription() string {
	if m != nil {
		return m.Description
	}
	return ""
}

func (m *Tree) GetPrivateKey() *google_protobuf2.Any {
	if m != nil {
		return m.PrivateKey
	}
	return nil
}

func (m *Tree) GetStorageSettings() *google_protobuf2.Any {
	if m != nil {
		return m.StorageSettings
	}
	return nil
}

func (m *Tree) GetPublicKey() *keyspb.PublicKey {
	if m != nil {
		return m.PublicKey
	}
	return nil
}

func (m *Tree) GetMaxRootDuration() *google_protobuf3.Duration {
	if m != nil {
		return m.MaxRootDuration
	}
	return nil
}

func (m *Tree) GetCreateTime() *google_protobuf1.Timestamp {
	if m != nil {
		return m.CreateTime
	}
	return nil
}

func (m *Tree) GetUpdateTime() *google_protobuf1.Timestamp {
	if m != nil {
		return m.UpdateTime
	}
	return nil
}

func (m *Tree) GetDeleted() bool {
	if m != nil {
		return m.Deleted
	}
	return false
}

func (m *Tree) GetDeleteTime() *google_protobuf1.Timestamp {
	if m != nil {
		return m.DeleteTime
	}
	return nil
}

type SignedEntryTimestamp struct {
	TimestampNanos int64                  `protobuf:"varint,1,opt,name=timestamp_nanos,json=timestampNanos" json:"timestamp_nanos,omitempty"`
	LogId          int64                  `protobuf:"varint,2,opt,name=log_id,json=logId" json:"log_id,omitempty"`
	Signature      *sigpb.DigitallySigned `protobuf:"bytes,3,opt,name=signature" json:"signature,omitempty"`
}

func (m *SignedEntryTimestamp) Reset()                    { *m = SignedEntryTimestamp{} }
func (m *SignedEntryTimestamp) String() string            { return proto.CompactTextString(m) }
func (*SignedEntryTimestamp) ProtoMessage()               {}
func (*SignedEntryTimestamp) Descriptor() ([]byte, []int) { return fileDescriptor3, []int{1} }

func (m *SignedEntryTimestamp) GetTimestampNanos() int64 {
	if m != nil {
		return m.TimestampNanos
	}
	return 0
}

func (m *SignedEntryTimestamp) GetLogId() int64 {
	if m != nil {
		return m.LogId
	}
	return 0
}

func (m *SignedEntryTimestamp) GetSignature() *sigpb.DigitallySigned {
	if m != nil {
		return m.Signature
	}
	return nil
}

// SignedLogRoot represents a commitment by a Log to a particular tree.
type SignedLogRoot struct {
	// epoch nanoseconds, good until 2500ish
	TimestampNanos int64  `protobuf:"varint,1,opt,name=timestamp_nanos,json=timestampNanos" json:"timestamp_nanos,omitempty"`
	RootHash       []byte `protobuf:"bytes,2,opt,name=root_hash,json=rootHash,proto3" json:"root_hash,omitempty"`
	// TreeSize is the number of entries in the tree.
	TreeSize     int64                  `protobuf:"varint,3,opt,name=tree_size,json=treeSize" json:"tree_size,omitempty"`
	Signature    *sigpb.DigitallySigned `protobuf:"bytes,4,opt,name=signature" json:"signature,omitempty"`
	TreeRevision int64                  `protobuf:"varint,6,opt,name=tree_revision,json=treeRevision" json:"tree_revision,omitempty"`
	// key_hint is a hint to identify the public key for signature verification.
	// key_hint is not authenticated and may be incorrect or missing, in which
	// case all known public keys may be used to verify the signature.
	// When directly communicating with a Trillian gRPC server, the key_hint will
	// typically contain the LogID encoded as a big-endian 64-bit integer;
	// however, in other contexts the key_hint is likely to have different
	// contents (e.g. it could be a GUID, a URL + TreeID, or it could be
	// derived from the public key itself).
	KeyHint []byte `protobuf:"bytes,7,opt,name=key_hint,json=keyHint,proto3" json:"key_hint,omitempty"`
}

func (m *SignedLogRoot) Reset()                    { *m = SignedLogRoot{} }
func (m *SignedLogRoot) String() string            { return proto.CompactTextString(m) }
func (*SignedLogRoot) ProtoMessage()               {}
func (*SignedLogRoot) Descriptor() ([]byte, []int) { return fileDescriptor3, []int{2} }

func (m *SignedLogRoot) GetTimestampNanos() int64 {
	if m != nil {
		return m.TimestampNanos
	}
	return 0
}

func (m *SignedLogRoot) GetRootHash() []byte {
	if m != nil {
		return m.RootHash
	}
	return nil
}

func (m *SignedLogRoot) GetTreeSize() int64 {
	if m != nil {
		return m.TreeSize
	}
	return 0
}

func (m *SignedLogRoot) GetSignature() *sigpb.DigitallySigned {
	if m != nil {
		return m.Signature
	}
	return nil
}

func (m *SignedLogRoot) GetTreeRevision() int64 {
	if m != nil {
		return m.TreeRevision
	}
	return 0
}

func (m *SignedLogRoot) GetKeyHint() []byte {
	if m != nil {
		return m.KeyHint
	}
	return nil
}

// SignedMapRoot represents a commitment by a Map to a particular tree.
type SignedMapRoot struct {
	// map_root holds the TLS-serialization of the following
	// structure (described in RFC5246 notation):
	// enum { v1(1), (65535)} Version;
	// struct {
	//   opaque root_hash<0..128>;
	//   uint64 timestamp_nanos;
	//   uint64 revision;
	//   opaque metadata<0..65535>;
	// } MapRootV1;
	// struct {
	//   Version version;
	//   select(version) {
	//     case v1: MapRootV1;
	//   }
	// } MapRoot;
	MapRoot []byte `protobuf:"bytes,9,opt,name=map_root,json=mapRoot,proto3" json:"map_root,omitempty"`
	// TODO(al): define serialized format for the signature scheme.
	Signature *sigpb.DigitallySigned `protobuf:"bytes,4,opt,name=signature" json:"signature,omitempty"`
}

func (m *SignedMapRoot) Reset()                    { *m = SignedMapRoot{} }
func (m *SignedMapRoot) String() string            { return proto.CompactTextString(m) }
func (*SignedMapRoot) ProtoMessage()               {}
func (*SignedMapRoot) Descriptor() ([]byte, []int) { return fileDescriptor3, []int{3} }

func (m *SignedMapRoot) GetMapRoot() []byte {
	if m != nil {
		return m.MapRoot
	}
	return nil
}

func (m *SignedMapRoot) GetSignature() *sigpb.DigitallySigned {
	if m != nil {
		return m.Signature
	}
	return nil
}

func init() {
	proto.RegisterType((*Tree)(nil), "trillian.Tree")
	proto.RegisterType((*SignedEntryTimestamp)(nil), "trillian.SignedEntryTimestamp")
	proto.RegisterType((*SignedLogRoot)(nil), "trillian.SignedLogRoot")
	proto.RegisterType((*SignedMapRoot)(nil), "trillian.SignedMapRoot")
	proto.RegisterEnum("trillian.LogRootFormat", LogRootFormat_name, LogRootFormat_value)
	proto.RegisterEnum("trillian.MapRootFormat", MapRootFormat_name, MapRootFormat_value)
	proto.RegisterEnum("trillian.HashStrategy", HashStrategy_name, HashStrategy_value)
	proto.RegisterEnum("trillian.TreeState", TreeState_name, TreeState_value)
	proto.RegisterEnum("trillian.TreeType", TreeType_name, TreeType_value)
}

func init() { proto.RegisterFile("trillian.proto", fileDescriptor3) }

var fileDescriptor3 = []byte{
	// 1072 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x9c, 0x56, 0x4b, 0x6f, 0xdb, 0x46,
	0x10, 0x0e, 0x25, 0x5a, 0xa2, 0x46, 0x0f, 0xaf, 0xd7, 0x49, 0x4c, 0x29, 0x40, 0xa3, 0xba, 0x05,
	0xea, 0xfa, 0x20, 0x37, 0x6a, 0x6d, 0xa0, 0xc8, 0xa1, 0xa0, 0x2d, 0xda, 0x92, 0x6c, 0x4b, 0xc2,
	0x92, 0x4d, 0x11, 0x5f, 0x08, 0x4a, 0xda, 0x52, 0x84, 0xc5, 0x07, 0xc8, 0x55, 0x10, 0xe6, 0xdc,
	0x4b, 0x1f, 0xbf, 0xb5, 0xbf, 0xa1, 0xd8, 0x25, 0x29, 0xdb, 0x72, 0xd3, 0x04, 0xbd, 0xd8, 0x3b,
	0xf3, 0x3d, 0x76, 0x96, 0x3b, 0x43, 0x0a, 0x1a, 0x2c, 0x72, 0x97, 0x4b, 0xd7, 0xf6, 0x3b, 0x61,
	0x14, 0xb0, 0x00, 0x2b, 0x79, 0xdc, 0x6a, 0xcd, 0xa2, 0x24, 0x64, 0xc1, 0xd1, 0x2d, 0x4d, 0xe2,
	0x70, 0x9a, 0xfd, 0x4b, 0x59, 0x2d, 0x35, 0xc3, 0x62, 0xd7, 0x09, 0xa7, 0xe9, 0xdf, 0x0c, 0x69,
	0x3a, 0x41, 0xe0, 0x2c, 0xe9, 0x91, 0x88, 0xa6, 0xab, 0x5f, 0x8f, 0x6c, 0x3f, 0xc9, 0xa0, 0x2f,
	0x36, 0xa1, 0xf9, 0x2a, 0xb2, 0x99, 0x1b, 0x64, 0x5b, 0xb7, 0x5e, 0x6e, 0xe2, 0xcc, 0xf5, 0x68,
	0xcc, 0x6c, 0x2f, 0x4c, 0x09, 0xfb, 0xbf, 0x97, 0x41, 0x36, 0x23, 0x4a, 0xf1, 0x1e, 0x94, 0x59,
	0x44, 0xa9, 0xe5, 0xce, 0x55, 0xa9, 0x2d, 0x1d, 0x14, 0x49, 0x89, 0x87, 0x83, 0x39, 0xee, 0x02,
	0x08, 0x20, 0x66, 0x36, 0xa3, 0x6a, 0xa1, 0x2d, 0x1d, 0x34, 0xba, 0xbb, 0x9d, 0xf5, 0x11, 0xb9,
	0xd8, 0xe0, 0x10, 0xa9, 0xb0, 0x7c, 0x89, 0x8f, 0x40, 0x04, 0x16, 0x4b, 0x42, 0xaa, 0x16, 0x85,
	0x04, 0x3f, 0x94, 0x98, 0x49, 0x48, 0x89, 0xc2, 0xb2, 0x15, 0x7e, 0x0d, 0xf5, 0x85, 0x1d, 0x2f,
	0xac, 0x98, 0x45, 0x36, 0xa3, 0x4e, 0xa2, 0xca, 0x42, 0xf4, 0xfc, 0x4e, 0xd4, 0xb7, 0xe3, 0x85,
	0x91, 0xa1, 0xa4, 0xb6, 0xb8, 0x17, 0xe1, 0x4b, 0x68, 0x08, 0xb1, 0xbd, 0x74, 0x82, 0xc8, 0x65,
	0x0b, 0x4f, 0xdd, 0x12, 0xea, 0xaf, 0x3b, 0xe9, 0x53, 0xec, 0xb9, 0x8e, 0xcb, 0xec, 0xe5, 0x32,
	0x31, 0x5c, 0xc7, 0xa7, 0x73, 0x61, 0xa5, 0xe5, 0x5c, 0x22, 0x36, 0x5e, 0x87, 0xf8, 0x06, 0x76,
	0x63, 0xd7, 0xf1, 0x6d, 0xb6, 0x8a, 0xe8, 0x3d, 0xc7, 0x92, 0x70, 0xfc, 0xf6, 0x23, 0x8e, 0x46,
	0xae, 0xb8, 0xb3, 0xc5, 0xf1, 0xa3, 0x1c, 0xfe, 0x12, 0x6a, 0x73, 0x37, 0x0e, 0x97, 0x76, 0x62,
	0xf9, 0xb6, 0x47, 0x55, 0xa5, 0x2d, 0x1d, 0x54, 0x48, 0x35, 0xcb, 0x8d, 0x6c, 0x8f, 0xe2, 0x36,
	0x54, 0xe7, 0x34, 0x9e, 0x45, 0x6e, 0xc8, 0x6f, 0x51, 0xad, 0x64, 0x8c, 0xbb, 0x14, 0x3e, 0x86,
	0x6a, 0x18, 0xb9, 0xef, 0x6c, 0x46, 0xad, 0x5b, 0x9a, 0xa8, 0xb5, 0xb6, 0x74, 0x50, 0xed, 0x3e,
	0xed, 0xa4, 0x17, 0xdd, 0xc9, 0x2f, 0xba, 0xa3, 0xf9, 0x09, 0x81, 0x8c, 0x78, 0x49, 0x13, 0xfc,
	0x13, 0xa0, 0x98, 0x05, 0x91, 0xed, 0x50, 0x2b, 0xa6, 0x8c, 0xb9, 0xbe, 0x13, 0xab, 0xf5, 0xff,
	0xd0, 0x6e, 0x67, 0x6c, 0x23, 0x23, 0xe3, 0xef, 0x00, 0xc2, 0xd5, 0x74, 0xe9, 0xce, 0xc4, 0xb6,
	0x0d, 0x21, 0xdd, 0xe9, 0x64, 0x2d, 0x3c, 0x11, 0xc8, 0x25, 0x4d, 0x48, 0x25, 0xcc, 0x97, 0x58,
	0x87, 0x1d, 0xcf, 0x7e, 0x6f, 0x45, 0x41, 0xc0, 0xac, 0xbc, 0x2f, 0xd5, 0x6d, 0x21, 0x6c, 0x3e,
	0xda, 0xb3, 0x97, 0x11, 0xc8, 0xb6, 0x67, 0xbf, 0x27, 0x41, 0xc0, 0xf2, 0x04, 0x7e, 0x0d, 0xd5,
	0x59, 0x44, 0xf9, 0x79, 0x79, 0xf3, 0xaa, 0x48, 0x18, 0xb4, 0x1e, 0x19, 0x98, 0x79, 0x67, 0x13,
	0x48, 0xe9, 0x3c, 0xc1, 0xc5, 0xab, 0x70, 0xbe, 0x16, 0xef, 0x7c, 0x5a, 0x9c, 0xd2, 0x85, 0x58,
	0x85, 0xf2, 0x9c, 0x2e, 0x29, 0xa3, 0x73, 0x75, 0xb7, 0x2d, 0x1d, 0x28, 0x24, 0x0f, 0xb9, 0x6d,
	0xba, 0x4c, 0x6d, 0x9f, 0x7e, 0xda, 0x36, 0xa5, 0xf3, 0xc4, 0x50, 0x56, 0x30, 0xda, 0x1d, 0xca,
	0x4a, 0x19, 0x29, 0x43, 0x59, 0x01, 0x54, 0x1d, 0xca, 0x4a, 0x15, 0xd5, 0xf6, 0xff, 0x92, 0xe0,
	0x69, 0xda, 0x50, 0xba, 0xcf, 0xa2, 0x64, 0x2d, 0xc6, 0xdf, 0xc0, 0xf6, 0x7a, 0x6e, 0x2d, 0xdf,
	0xf6, 0x83, 0x38, 0x9b, 0xd1, 0xc6, 0x3a, 0x3d, 0xe2, 0x59, 0xfc, 0x0c, 0x4a, 0xcb, 0xc0, 0xe1,
	0x33, 0x5c, 0x10, 0xf8, 0xd6, 0x32, 0x70, 0x06, 0x73, 0xfc, 0x03, 0x54, 0xd6, 0xdd, 0x28, 0xc6,
	0xb1, 0xda, 0x7d, 0xfe, 0xef, 0x9d, 0x4c, 0xee, 0x88, 0xfb, 0x7f, 0x4b, 0x50, 0x4f, 0xb3, 0x57,
	0x81, 0xc3, 0x6f, 0xe4, 0xf3, 0xeb, 0x78, 0x01, 0x15, 0x71, 0xeb, 0x7c, 0xb4, 0x44, 0x29, 0x35,
	0xa2, 0xf0, 0x04, 0x9f, 0x3c, 0x0e, 0xa6, 0x2f, 0x14, 0xf7, 0x43, 0x5a, 0x4d, 0x31, 0x7d, 0x11,
	0x18, 0xee, 0x07, 0xfa, 0xb0, 0x54, 0xf9, 0x33, 0x4b, 0xc5, 0x5f, 0x41, 0x5d, 0x58, 0x46, 0xf4,
	0x9d, 0x1b, 0xf3, 0x2e, 0x2b, 0x09, 0xdb, 0x1a, 0x4f, 0x92, 0x2c, 0x87, 0x9b, 0xa0, 0xdc, 0xd2,
	0xc4, 0x5a, 0xb8, 0x3e, 0x53, 0xcb, 0xa2, 0xa6, 0xf2, 0x2d, 0x4d, 0xfa, 0xae, 0xcf, 0x86, 0xb2,
	0xb2, 0x85, 0x4a, 0xfb, 0x7f, 0xac, 0x0f, 0x7c, 0x6d, 0x87, 0xe2, 0xc0, 0x4d, 0x50, 0x3c, 0x3b,
	0x14, 0x1d, 0x2c, 0x46, 0xb1, 0x46, 0xca, 0x5e, 0x06, 0xfd, 0xaf, 0x42, 0x87, 0xb2, 0x22, 0xa1,
	0xc2, 0x50, 0x56, 0x0a, 0xa8, 0x38, 0x94, 0x95, 0x22, 0x92, 0xd3, 0xad, 0x87, 0xb2, 0x52, 0x42,
	0xe5, 0x75, 0x4b, 0x28, 0xa8, 0x72, 0xd8, 0x83, 0x7a, 0xf6, 0xd8, 0xcf, 0x83, 0xc8, 0xb3, 0x19,
	0x7e, 0x01, 0x7b, 0x57, 0xe3, 0x0b, 0x8b, 0x8c, 0xc7, 0xa6, 0x75, 0x3e, 0x26, 0xd7, 0x9a, 0x69,
	0xfd, 0x3c, 0xba, 0x1c, 0x8d, 0x7f, 0x19, 0xa1, 0x27, 0xf8, 0x39, 0xe0, 0x4d, 0xf0, 0xcd, 0x2b,
	0x24, 0x71, 0x97, 0xec, 0x2c, 0x77, 0x2e, 0xd7, 0xda, 0xe4, 0xe3, 0x2e, 0x9b, 0xa0, 0x70, 0xf9,
	0x4d, 0x82, 0xda, 0xfd, 0xf7, 0x2f, 0x6e, 0xc2, 0xb3, 0x4c, 0x65, 0xf5, 0x35, 0xa3, 0x6f, 0x19,
	0x26, 0xd1, 0x4c, 0xfd, 0xe2, 0x2d, 0x7a, 0x82, 0x31, 0x34, 0xc8, 0xf9, 0xd9, 0xc9, 0x8f, 0x27,
	0x5d, 0xcb, 0xe8, 0x6b, 0xdd, 0xe3, 0x13, 0x24, 0xe1, 0x5d, 0xd8, 0x36, 0x75, 0xc3, 0xb4, 0xb8,
	0x39, 0xe7, 0xeb, 0x04, 0x15, 0xb8, 0xc7, 0xf8, 0x74, 0xa8, 0x9f, 0x99, 0xd6, 0x06, 0xbf, 0x88,
	0x9f, 0xc1, 0xce, 0xd9, 0x78, 0x34, 0xb8, 0x34, 0x78, 0xea, 0xf8, 0x55, 0xd7, 0xe2, 0x69, 0xf9,
	0xf0, 0x4f, 0x09, 0x2a, 0xeb, 0xcf, 0x0d, 0x2f, 0x36, 0xaf, 0xc1, 0x24, 0xba, 0x6e, 0x19, 0xa6,
	0x66, 0xea, 0xe8, 0x09, 0x06, 0x28, 0x69, 0x67, 0xe6, 0xe0, 0x8d, 0x8e, 0x24, 0xbe, 0x3e, 0x27,
	0xe3, 0x1b, 0x7d, 0x84, 0x0a, 0xf8, 0x25, 0xec, 0xf5, 0xf4, 0x09, 0xd1, 0xcf, 0x34, 0x53, 0xef,
	0x59, 0xc6, 0xf8, 0xdc, 0xb4, 0x7a, 0xfa, 0x95, 0x6e, 0xea, 0x3d, 0x54, 0x6c, 0x15, 0x14, 0x69,
	0x83, 0xd0, 0xd7, 0x48, 0x6f, 0x4d, 0x90, 0x05, 0xa1, 0x06, 0x4a, 0x8f, 0x68, 0x83, 0xd1, 0x60,
	0x74, 0x81, 0xb6, 0x0e, 0x2f, 0x40, 0xc9, 0x3f, 0x64, 0xbc, 0xe0, 0x07, 0xb5, 0x98, 0x6f, 0x27,
	0xbc, 0x94, 0x32, 0x14, 0xaf, 0xc6, 0x17, 0x48, 0xe2, 0x8b, 0x6b, 0x6d, 0x82, 0x0a, 0xfc, 0xe9,
	0x4c, 0x88, 0x3e, 0x26, 0x3d, 0x9d, 0xe8, 0x3d, 0x8b, 0x83, 0xc5, 0xd3, 0x3e, 0x34, 0x67, 0x81,
	0x97, 0xbf, 0x3b, 0x1e, 0xfe, 0x76, 0x38, 0xad, 0x9b, 0x59, 0x3c, 0xe1, 0xe1, 0x44, 0xba, 0x69,
	0x39, 0x2e, 0x5b, 0xac, 0xa6, 0x9d, 0x59, 0xe0, 0x1d, 0x65, 0x1f, 0xf7, 0x5c, 0x32, 0x2d, 0x09,
	0xcd, 0xf7, 0xff, 0x04, 0x00, 0x00, 0xff, 0xff, 0x2b, 0xfe, 0x59, 0x59, 0x81, 0x08, 0x00, 0x00,
}
