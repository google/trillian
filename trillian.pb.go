// Code generated by protoc-gen-go. DO NOT EDIT.
// source: trillian.proto

package trillian

import (
	fmt "fmt"
	proto "github.com/golang/protobuf/proto"
	any "github.com/golang/protobuf/ptypes/any"
	duration "github.com/golang/protobuf/ptypes/duration"
	timestamp "github.com/golang/protobuf/ptypes/timestamp"
	keyspb "github.com/google/trillian/crypto/keyspb"
	sigpb "github.com/google/trillian/crypto/sigpb"
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

func (LogRootFormat) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_364603a4e17a2a56, []int{0}
}

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

func (MapRootFormat) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_364603a4e17a2a56, []int{1}
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
	// The CONIKS sparse tree hasher with SHA512_256 as the hash algorithm.
	HashStrategy_CONIKS_SHA512_256 HashStrategy = 4
)

var HashStrategy_name = map[int32]string{
	0: "UNKNOWN_HASH_STRATEGY",
	1: "RFC6962_SHA256",
	2: "TEST_MAP_HASHER",
	4: "CONIKS_SHA512_256",
}

var HashStrategy_value = map[string]int32{
	"UNKNOWN_HASH_STRATEGY": 0,
	"RFC6962_SHA256":        1,
	"TEST_MAP_HASHER":       2,
	"CONIKS_SHA512_256":     4,
}

func (x HashStrategy) String() string {
	return proto.EnumName(HashStrategy_name, int32(x))
}

func (HashStrategy) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_364603a4e17a2a56, []int{2}
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
	TreeState_DEPRECATED_SOFT_DELETED TreeState = 3 // Deprecated: Do not use.
	// Deprecated: now tracked in Tree.deleted.
	TreeState_DEPRECATED_HARD_DELETED TreeState = 4 // Deprecated: Do not use.
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

func (TreeState) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_364603a4e17a2a56, []int{3}
}

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

func (TreeType) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_364603a4e17a2a56, []int{4}
}

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
	TreeId int64 `protobuf:"varint,1,opt,name=tree_id,json=treeId,proto3" json:"tree_id,omitempty"`
	// State of the tree.
	// Trees are ACTIVE after creation. At any point the tree may transition
	// between ACTIVE, DRAINING and FROZEN states.
	TreeState TreeState `protobuf:"varint,2,opt,name=tree_state,json=treeState,proto3,enum=trillian.TreeState" json:"tree_state,omitempty"`
	// Type of the tree.
	// Readonly after Tree creation. Exception: Can be switched from
	// PREORDERED_LOG to LOG if the Tree is and remains in the FROZEN state.
	TreeType TreeType `protobuf:"varint,3,opt,name=tree_type,json=treeType,proto3,enum=trillian.TreeType" json:"tree_type,omitempty"`
	// Hash strategy to be used by the tree.
	// Readonly.
	HashStrategy HashStrategy `protobuf:"varint,4,opt,name=hash_strategy,json=hashStrategy,proto3,enum=trillian.HashStrategy" json:"hash_strategy,omitempty"`
	// Hash algorithm to be used by the tree.
	// Readonly.
	HashAlgorithm sigpb.DigitallySigned_HashAlgorithm `protobuf:"varint,5,opt,name=hash_algorithm,json=hashAlgorithm,proto3,enum=sigpb.DigitallySigned_HashAlgorithm" json:"hash_algorithm,omitempty"`
	// Signature algorithm to be used by the tree.
	// Readonly.
	SignatureAlgorithm sigpb.DigitallySigned_SignatureAlgorithm `protobuf:"varint,6,opt,name=signature_algorithm,json=signatureAlgorithm,proto3,enum=sigpb.DigitallySigned_SignatureAlgorithm" json:"signature_algorithm,omitempty"`
	// Display name of the tree.
	// Optional.
	DisplayName string `protobuf:"bytes,8,opt,name=display_name,json=displayName,proto3" json:"display_name,omitempty"`
	// Description of the tree,
	// Optional.
	Description string `protobuf:"bytes,9,opt,name=description,proto3" json:"description,omitempty"`
	// Identifies the private key used for signing tree heads and entry
	// timestamps.
	// This can be any type of message to accommodate different key management
	// systems, e.g. PEM files, HSMs, etc.
	// Private keys are write-only: they're never returned by RPCs.
	// The private_key message can be changed after a tree is created, but the
	// underlying key must remain the same - this is to enable migrating a key
	// from one provider to another.
	PrivateKey *any.Any `protobuf:"bytes,12,opt,name=private_key,json=privateKey,proto3" json:"private_key,omitempty"`
	// Storage-specific settings.
	// Varies according to the storage implementation backing Trillian.
	StorageSettings *any.Any `protobuf:"bytes,13,opt,name=storage_settings,json=storageSettings,proto3" json:"storage_settings,omitempty"`
	// The public key used for verifying tree heads and entry timestamps.
	// Readonly.
	PublicKey *keyspb.PublicKey `protobuf:"bytes,14,opt,name=public_key,json=publicKey,proto3" json:"public_key,omitempty"`
	// Interval after which a new signed root is produced even if there have been
	// no submission.  If zero, this behavior is disabled.
	MaxRootDuration *duration.Duration `protobuf:"bytes,15,opt,name=max_root_duration,json=maxRootDuration,proto3" json:"max_root_duration,omitempty"`
	// Time of tree creation.
	// Readonly.
	CreateTime *timestamp.Timestamp `protobuf:"bytes,16,opt,name=create_time,json=createTime,proto3" json:"create_time,omitempty"`
	// Time of last tree update.
	// Readonly (automatically assigned on updates).
	UpdateTime *timestamp.Timestamp `protobuf:"bytes,17,opt,name=update_time,json=updateTime,proto3" json:"update_time,omitempty"`
	// If true, the tree has been deleted.
	// Deleted trees may be undeleted during a certain time window, after which
	// they're permanently deleted (and unrecoverable).
	// Readonly.
	Deleted bool `protobuf:"varint,19,opt,name=deleted,proto3" json:"deleted,omitempty"`
	// Time of tree deletion, if any.
	// Readonly.
	DeleteTime           *timestamp.Timestamp `protobuf:"bytes,20,opt,name=delete_time,json=deleteTime,proto3" json:"delete_time,omitempty"`
	XXX_NoUnkeyedLiteral struct{}             `json:"-"`
	XXX_unrecognized     []byte               `json:"-"`
	XXX_sizecache        int32                `json:"-"`
}

func (m *Tree) Reset()         { *m = Tree{} }
func (m *Tree) String() string { return proto.CompactTextString(m) }
func (*Tree) ProtoMessage()    {}
func (*Tree) Descriptor() ([]byte, []int) {
	return fileDescriptor_364603a4e17a2a56, []int{0}
}

func (m *Tree) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Tree.Unmarshal(m, b)
}
func (m *Tree) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Tree.Marshal(b, m, deterministic)
}
func (m *Tree) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Tree.Merge(m, src)
}
func (m *Tree) XXX_Size() int {
	return xxx_messageInfo_Tree.Size(m)
}
func (m *Tree) XXX_DiscardUnknown() {
	xxx_messageInfo_Tree.DiscardUnknown(m)
}

var xxx_messageInfo_Tree proto.InternalMessageInfo

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

func (m *Tree) GetPrivateKey() *any.Any {
	if m != nil {
		return m.PrivateKey
	}
	return nil
}

func (m *Tree) GetStorageSettings() *any.Any {
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

func (m *Tree) GetMaxRootDuration() *duration.Duration {
	if m != nil {
		return m.MaxRootDuration
	}
	return nil
}

func (m *Tree) GetCreateTime() *timestamp.Timestamp {
	if m != nil {
		return m.CreateTime
	}
	return nil
}

func (m *Tree) GetUpdateTime() *timestamp.Timestamp {
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

func (m *Tree) GetDeleteTime() *timestamp.Timestamp {
	if m != nil {
		return m.DeleteTime
	}
	return nil
}

type SignedEntryTimestamp struct {
	TimestampNanos       int64                  `protobuf:"varint,1,opt,name=timestamp_nanos,json=timestampNanos,proto3" json:"timestamp_nanos,omitempty"`
	LogId                int64                  `protobuf:"varint,2,opt,name=log_id,json=logId,proto3" json:"log_id,omitempty"`
	Signature            *sigpb.DigitallySigned `protobuf:"bytes,3,opt,name=signature,proto3" json:"signature,omitempty"`
	XXX_NoUnkeyedLiteral struct{}               `json:"-"`
	XXX_unrecognized     []byte                 `json:"-"`
	XXX_sizecache        int32                  `json:"-"`
}

func (m *SignedEntryTimestamp) Reset()         { *m = SignedEntryTimestamp{} }
func (m *SignedEntryTimestamp) String() string { return proto.CompactTextString(m) }
func (*SignedEntryTimestamp) ProtoMessage()    {}
func (*SignedEntryTimestamp) Descriptor() ([]byte, []int) {
	return fileDescriptor_364603a4e17a2a56, []int{1}
}

func (m *SignedEntryTimestamp) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SignedEntryTimestamp.Unmarshal(m, b)
}
func (m *SignedEntryTimestamp) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SignedEntryTimestamp.Marshal(b, m, deterministic)
}
func (m *SignedEntryTimestamp) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SignedEntryTimestamp.Merge(m, src)
}
func (m *SignedEntryTimestamp) XXX_Size() int {
	return xxx_messageInfo_SignedEntryTimestamp.Size(m)
}
func (m *SignedEntryTimestamp) XXX_DiscardUnknown() {
	xxx_messageInfo_SignedEntryTimestamp.DiscardUnknown(m)
}

var xxx_messageInfo_SignedEntryTimestamp proto.InternalMessageInfo

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
	// key_hint is a hint to identify the public key for signature verification.
	// key_hint is not authenticated and may be incorrect or missing, in which
	// case all known public keys may be used to verify the signature.
	// When directly communicating with a Trillian gRPC server, the key_hint will
	// typically contain the LogID encoded as a big-endian 64-bit integer;
	// however, in other contexts the key_hint is likely to have different
	// contents (e.g. it could be a GUID, a URL + TreeID, or it could be
	// derived from the public key itself).
	KeyHint []byte `protobuf:"bytes,7,opt,name=key_hint,json=keyHint,proto3" json:"key_hint,omitempty"`
	// log_root holds the TLS-serialization of the following structure (described
	// in RFC5246 notation): Clients should validate log_root_signature with
	// VerifySignedLogRoot before deserializing log_root.
	// enum { v1(1), (65535)} Version;
	// struct {
	//   uint64 tree_size;
	//   opaque root_hash<0..128>;
	//   uint64 timestamp_nanos;
	//   uint64 revision;
	//   opaque metadata<0..65535>;
	// } LogRootV1;
	// struct {
	//   Version version;
	//   select(version) {
	//     case v1: LogRootV1;
	//   }
	// } LogRoot;
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
	LogRoot []byte `protobuf:"bytes,8,opt,name=log_root,json=logRoot,proto3" json:"log_root,omitempty"`
	// log_root_signature is the raw signature over log_root.
	LogRootSignature     []byte   `protobuf:"bytes,9,opt,name=log_root_signature,json=logRootSignature,proto3" json:"log_root_signature,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *SignedLogRoot) Reset()         { *m = SignedLogRoot{} }
func (m *SignedLogRoot) String() string { return proto.CompactTextString(m) }
func (*SignedLogRoot) ProtoMessage()    {}
func (*SignedLogRoot) Descriptor() ([]byte, []int) {
	return fileDescriptor_364603a4e17a2a56, []int{2}
}

func (m *SignedLogRoot) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SignedLogRoot.Unmarshal(m, b)
}
func (m *SignedLogRoot) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SignedLogRoot.Marshal(b, m, deterministic)
}
func (m *SignedLogRoot) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SignedLogRoot.Merge(m, src)
}
func (m *SignedLogRoot) XXX_Size() int {
	return xxx_messageInfo_SignedLogRoot.Size(m)
}
func (m *SignedLogRoot) XXX_DiscardUnknown() {
	xxx_messageInfo_SignedLogRoot.DiscardUnknown(m)
}

var xxx_messageInfo_SignedLogRoot proto.InternalMessageInfo

func (m *SignedLogRoot) GetKeyHint() []byte {
	if m != nil {
		return m.KeyHint
	}
	return nil
}

func (m *SignedLogRoot) GetLogRoot() []byte {
	if m != nil {
		return m.LogRoot
	}
	return nil
}

func (m *SignedLogRoot) GetLogRootSignature() []byte {
	if m != nil {
		return m.LogRootSignature
	}
	return nil
}

// SignedMapRoot represents a commitment by a Map to a particular tree.
type SignedMapRoot struct {
	// map_root holds the TLS-serialization of the following structure (described
	// in RFC5246 notation): Clients should validate signature with
	// VerifySignedMapRoot before deserializing map_root.
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
	// Signature is the raw signature over MapRoot.
	Signature            []byte   `protobuf:"bytes,4,opt,name=signature,proto3" json:"signature,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *SignedMapRoot) Reset()         { *m = SignedMapRoot{} }
func (m *SignedMapRoot) String() string { return proto.CompactTextString(m) }
func (*SignedMapRoot) ProtoMessage()    {}
func (*SignedMapRoot) Descriptor() ([]byte, []int) {
	return fileDescriptor_364603a4e17a2a56, []int{3}
}

func (m *SignedMapRoot) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_SignedMapRoot.Unmarshal(m, b)
}
func (m *SignedMapRoot) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_SignedMapRoot.Marshal(b, m, deterministic)
}
func (m *SignedMapRoot) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SignedMapRoot.Merge(m, src)
}
func (m *SignedMapRoot) XXX_Size() int {
	return xxx_messageInfo_SignedMapRoot.Size(m)
}
func (m *SignedMapRoot) XXX_DiscardUnknown() {
	xxx_messageInfo_SignedMapRoot.DiscardUnknown(m)
}

var xxx_messageInfo_SignedMapRoot proto.InternalMessageInfo

func (m *SignedMapRoot) GetMapRoot() []byte {
	if m != nil {
		return m.MapRoot
	}
	return nil
}

func (m *SignedMapRoot) GetSignature() []byte {
	if m != nil {
		return m.Signature
	}
	return nil
}

// Proof holds a consistency or inclusion proof for a Merkle tree, as returned
// by the API.
type Proof struct {
	// leaf_index indicates the requested leaf index when this message is used for
	// a leaf inclusion proof.  This field is set to zero when this message is
	// used for a consistency proof.
	LeafIndex            int64    `protobuf:"varint,1,opt,name=leaf_index,json=leafIndex,proto3" json:"leaf_index,omitempty"`
	Hashes               [][]byte `protobuf:"bytes,3,rep,name=hashes,proto3" json:"hashes,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *Proof) Reset()         { *m = Proof{} }
func (m *Proof) String() string { return proto.CompactTextString(m) }
func (*Proof) ProtoMessage()    {}
func (*Proof) Descriptor() ([]byte, []int) {
	return fileDescriptor_364603a4e17a2a56, []int{4}
}

func (m *Proof) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Proof.Unmarshal(m, b)
}
func (m *Proof) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Proof.Marshal(b, m, deterministic)
}
func (m *Proof) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Proof.Merge(m, src)
}
func (m *Proof) XXX_Size() int {
	return xxx_messageInfo_Proof.Size(m)
}
func (m *Proof) XXX_DiscardUnknown() {
	xxx_messageInfo_Proof.DiscardUnknown(m)
}

var xxx_messageInfo_Proof proto.InternalMessageInfo

func (m *Proof) GetLeafIndex() int64 {
	if m != nil {
		return m.LeafIndex
	}
	return 0
}

func (m *Proof) GetHashes() [][]byte {
	if m != nil {
		return m.Hashes
	}
	return nil
}

func init() {
	proto.RegisterEnum("trillian.LogRootFormat", LogRootFormat_name, LogRootFormat_value)
	proto.RegisterEnum("trillian.MapRootFormat", MapRootFormat_name, MapRootFormat_value)
	proto.RegisterEnum("trillian.HashStrategy", HashStrategy_name, HashStrategy_value)
	proto.RegisterEnum("trillian.TreeState", TreeState_name, TreeState_value)
	proto.RegisterEnum("trillian.TreeType", TreeType_name, TreeType_value)
	proto.RegisterType((*Tree)(nil), "trillian.Tree")
	proto.RegisterType((*SignedEntryTimestamp)(nil), "trillian.SignedEntryTimestamp")
	proto.RegisterType((*SignedLogRoot)(nil), "trillian.SignedLogRoot")
	proto.RegisterType((*SignedMapRoot)(nil), "trillian.SignedMapRoot")
	proto.RegisterType((*Proof)(nil), "trillian.Proof")
}

func init() { proto.RegisterFile("trillian.proto", fileDescriptor_364603a4e17a2a56) }

var fileDescriptor_364603a4e17a2a56 = []byte{
	// 1086 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x84, 0x56, 0xef, 0x6f, 0xda, 0xc6,
	0x1b, 0xaf, 0xc1, 0x80, 0x79, 0x20, 0xc9, 0xe5, 0xd2, 0xa6, 0x0e, 0xdf, 0xef, 0x56, 0x86, 0x26,
	0x8d, 0x55, 0x13, 0x59, 0xd9, 0x5a, 0x69, 0xea, 0x8b, 0xc9, 0x0d, 0x4e, 0x80, 0x24, 0x80, 0x0e,
	0xaf, 0x53, 0xfb, 0xe6, 0xe4, 0xc0, 0xd5, 0x58, 0x31, 0xb6, 0x65, 0x1f, 0x53, 0xfd, 0x27, 0x4c,
	0xdb, 0xfb, 0xfd, 0xbb, 0xd3, 0x9d, 0xcf, 0x90, 0xa6, 0xed, 0xfa, 0x26, 0xb9, 0xe7, 0xf9, 0xfc,
	0x78, 0xce, 0xf7, 0x3c, 0x67, 0x03, 0xfb, 0x3c, 0xf1, 0x83, 0xc0, 0x77, 0xc3, 0x5e, 0x9c, 0x44,
	0x3c, 0xc2, 0x46, 0x11, 0xb7, 0x5a, 0x8b, 0x24, 0x8b, 0x79, 0x74, 0x7a, 0xcb, 0xb2, 0x34, 0xbe,
	0x51, 0xff, 0x72, 0x56, 0xcb, 0x54, 0x58, 0xea, 0x7b, 0xf1, 0x4d, 0xfe, 0x57, 0x21, 0x27, 0x5e,
	0x14, 0x79, 0x01, 0x3b, 0x95, 0xd1, 0xcd, 0xe6, 0xdd, 0xa9, 0x1b, 0x66, 0x0a, 0xfa, 0xfa, 0x3e,
	0xb4, 0xdc, 0x24, 0x2e, 0xf7, 0x23, 0x55, 0xba, 0xf5, 0xe4, 0x3e, 0xce, 0xfd, 0x35, 0x4b, 0xb9,
	0xbb, 0x8e, 0x73, 0x42, 0xe7, 0xcf, 0x1a, 0xe8, 0x4e, 0xc2, 0x18, 0x7e, 0x0c, 0x35, 0x9e, 0x30,
	0x46, 0xfd, 0xa5, 0xa9, 0xb5, 0xb5, 0x6e, 0x99, 0x54, 0x45, 0x38, 0x5a, 0xe2, 0x3e, 0x80, 0x04,
	0x52, 0xee, 0x72, 0x66, 0x96, 0xda, 0x5a, 0x77, 0xbf, 0x7f, 0xd4, 0xdb, 0x3e, 0xa2, 0x10, 0xcf,
	0x05, 0x44, 0xea, 0xbc, 0x58, 0xe2, 0x53, 0x90, 0x01, 0xe5, 0x59, 0xcc, 0xcc, 0xb2, 0x94, 0xe0,
	0x0f, 0x25, 0x4e, 0x16, 0x33, 0x62, 0x70, 0xb5, 0xc2, 0x2f, 0x61, 0x6f, 0xe5, 0xa6, 0x2b, 0x9a,
	0xf2, 0xc4, 0xe5, 0xcc, 0xcb, 0x4c, 0x5d, 0x8a, 0x8e, 0x77, 0xa2, 0xa1, 0x9b, 0xae, 0xe6, 0x0a,
	0x25, 0xcd, 0xd5, 0x9d, 0x08, 0x5f, 0xc2, 0xbe, 0x14, 0xbb, 0x81, 0x17, 0x25, 0x3e, 0x5f, 0xad,
	0xcd, 0x8a, 0x54, 0x7f, 0xdb, 0xcb, 0x4f, 0x71, 0xe0, 0x7b, 0x3e, 0x77, 0x83, 0x20, 0x9b, 0xfb,
	0x5e, 0xc8, 0x96, 0xd2, 0xca, 0x2a, 0xb8, 0x44, 0x16, 0xde, 0x86, 0xf8, 0x2d, 0x1c, 0xa5, 0xbe,
	0x17, 0xba, 0x7c, 0x93, 0xb0, 0x3b, 0x8e, 0x55, 0xe9, 0xf8, 0xfd, 0x67, 0x1c, 0xe7, 0x85, 0x62,
	0x67, 0x8b, 0xd3, 0x8f, 0x72, 0xf8, 0x1b, 0x68, 0x2e, 0xfd, 0x34, 0x0e, 0xdc, 0x8c, 0x86, 0xee,
	0x9a, 0x99, 0x46, 0x5b, 0xeb, 0xd6, 0x49, 0x43, 0xe5, 0x26, 0xee, 0x9a, 0xe1, 0x36, 0x34, 0x96,
	0x2c, 0x5d, 0x24, 0x7e, 0x2c, 0xba, 0x68, 0xd6, 0x15, 0x63, 0x97, 0xc2, 0xcf, 0xa1, 0x11, 0x27,
	0xfe, 0x1f, 0x2e, 0x67, 0xf4, 0x96, 0x65, 0x66, 0xb3, 0xad, 0x75, 0x1b, 0xfd, 0x87, 0xbd, 0xbc,
	0xd1, 0xbd, 0xa2, 0xd1, 0x3d, 0x2b, 0xcc, 0x08, 0x28, 0xe2, 0x25, 0xcb, 0xf0, 0xaf, 0x80, 0x52,
	0x1e, 0x25, 0xae, 0xc7, 0x68, 0xca, 0x38, 0xf7, 0x43, 0x2f, 0x35, 0xf7, 0xfe, 0x43, 0x7b, 0xa0,
	0xd8, 0x73, 0x45, 0xc6, 0x3f, 0x02, 0xc4, 0x9b, 0x9b, 0xc0, 0x5f, 0xc8, 0xb2, 0xfb, 0x52, 0x7a,
	0xd8, 0x53, 0x23, 0x3c, 0x93, 0xc8, 0x25, 0xcb, 0x48, 0x3d, 0x2e, 0x96, 0xd8, 0x86, 0xc3, 0xb5,
	0xfb, 0x9e, 0x26, 0x51, 0xc4, 0x69, 0x31, 0x97, 0xe6, 0x81, 0x14, 0x9e, 0x7c, 0x54, 0x73, 0xa0,
	0x08, 0xe4, 0x60, 0xed, 0xbe, 0x27, 0x51, 0xc4, 0x8b, 0x04, 0x7e, 0x09, 0x8d, 0x45, 0xc2, 0xc4,
	0xf3, 0x8a, 0xe1, 0x35, 0x91, 0x34, 0x68, 0x7d, 0x64, 0xe0, 0x14, 0x93, 0x4d, 0x20, 0xa7, 0x8b,
	0x84, 0x10, 0x6f, 0xe2, 0xe5, 0x56, 0x7c, 0xf8, 0x65, 0x71, 0x4e, 0x97, 0x62, 0x13, 0x6a, 0x4b,
	0x16, 0x30, 0xce, 0x96, 0xe6, 0x51, 0x5b, 0xeb, 0x1a, 0xa4, 0x08, 0x85, 0x6d, 0xbe, 0xcc, 0x6d,
	0x1f, 0x7e, 0xd9, 0x36, 0xa7, 0x8b, 0xc4, 0x58, 0x37, 0x30, 0x3a, 0x1a, 0xeb, 0x46, 0x0d, 0x19,
	0x63, 0xdd, 0x00, 0xd4, 0x18, 0xeb, 0x46, 0x03, 0x35, 0x3b, 0x7f, 0x6b, 0xf0, 0x30, 0x1f, 0x28,
	0x3b, 0xe4, 0x49, 0xb6, 0x15, 0xe3, 0xef, 0xe0, 0x60, 0x7b, 0x6f, 0x69, 0xe8, 0x86, 0x51, 0xaa,
	0xee, 0xe8, 0xfe, 0x36, 0x3d, 0x11, 0x59, 0xfc, 0x08, 0xaa, 0x41, 0xe4, 0x89, 0x3b, 0x5c, 0x92,
	0x78, 0x25, 0x88, 0xbc, 0xd1, 0x12, 0xff, 0x0c, 0xf5, 0xed, 0x34, 0xca, 0xeb, 0xd8, 0xe8, 0x1f,
	0x7f, 0x7a, 0x92, 0xc9, 0x8e, 0xd8, 0xf9, 0x47, 0x83, 0xbd, 0x3c, 0x7b, 0x15, 0x79, 0xa2, 0x23,
	0xf8, 0x04, 0x8c, 0x5b, 0x96, 0xd1, 0x95, 0x1f, 0x72, 0xb3, 0xd6, 0xd6, 0xba, 0x4d, 0x52, 0xbb,
	0x65, 0xd9, 0xd0, 0x0f, 0x25, 0x24, 0x2a, 0x8b, 0x5e, 0xcb, 0xb1, 0x6e, 0x92, 0x5a, 0xa0, 0x54,
	0x3f, 0x00, 0x2e, 0x20, 0xba, 0xdb, 0x46, 0x5d, 0x92, 0x90, 0x22, 0x6d, 0x2f, 0xd0, 0x58, 0x37,
	0x34, 0x54, 0x1a, 0xeb, 0x46, 0x09, 0x95, 0xc7, 0xba, 0x51, 0x46, 0xfa, 0x58, 0x37, 0x74, 0x54,
	0x19, 0xeb, 0x46, 0x05, 0x55, 0xc7, 0xba, 0x51, 0x45, 0xb5, 0x4e, 0x52, 0x6c, 0xec, 0xda, 0x8d,
	0x8b, 0x8d, 0xad, 0xdd, 0x38, 0xaf, 0x9e, 0x1b, 0xd7, 0xd6, 0x0a, 0xfa, 0xff, 0xdd, 0x67, 0xd7,
	0x25, 0xb6, 0x4b, 0x7c, 0xb2, 0xda, 0xb6, 0xce, 0xb6, 0x45, 0x06, 0xaa, 0x77, 0x06, 0x50, 0x99,
	0x25, 0x51, 0xf4, 0x0e, 0x7f, 0x05, 0x10, 0x30, 0xf7, 0x1d, 0xf5, 0xc3, 0x25, 0x7b, 0xaf, 0xfa,
	0x50, 0x17, 0x99, 0x91, 0x48, 0xe0, 0x63, 0xa8, 0x8a, 0x17, 0x0a, 0x4b, 0xcd, 0x72, 0xbb, 0xdc,
	0x6d, 0x12, 0x15, 0xe5, 0x35, 0x9e, 0x0e, 0x60, 0x4f, 0x1d, 0xe6, 0x79, 0x94, 0xac, 0x5d, 0x8e,
	0xff, 0x07, 0x8f, 0xaf, 0xa6, 0x17, 0x94, 0x4c, 0xa7, 0x0e, 0x3d, 0x9f, 0x92, 0x6b, 0xcb, 0xa1,
	0xbf, 0x4d, 0x2e, 0x27, 0xd3, 0xdf, 0x27, 0xe8, 0x01, 0x3e, 0x06, 0x7c, 0x1f, 0x7c, 0xfd, 0x0c,
	0x69, 0xc2, 0x45, 0x3d, 0xf9, 0xce, 0xe5, 0xda, 0x9a, 0x7d, 0xde, 0xe5, 0x3e, 0x28, 0x5d, 0x36,
	0xd0, 0xbc, 0xfb, 0x52, 0xc5, 0x27, 0xf0, 0x48, 0x89, 0xe8, 0xd0, 0x9a, 0x0f, 0xe9, 0xdc, 0x21,
	0x96, 0x63, 0x5f, 0xbc, 0x41, 0x0f, 0x30, 0x86, 0x7d, 0x72, 0x7e, 0xf6, 0xe2, 0x97, 0x17, 0x7d,
	0x3a, 0x1f, 0x5a, 0xfd, 0xe7, 0x2f, 0x90, 0x86, 0x8f, 0xe0, 0xc0, 0xb1, 0xe7, 0x0e, 0x15, 0xde,
	0x82, 0x6f, 0x13, 0x54, 0xc2, 0x8f, 0xe0, 0xf0, 0x6c, 0x3a, 0x19, 0x5d, 0xce, 0x05, 0xef, 0xf9,
	0xb3, 0x3e, 0x15, 0x5c, 0xbd, 0x23, 0x8e, 0xb6, 0xdc, 0x11, 0x47, 0x5b, 0x79, 0xfa, 0x97, 0x06,
	0xf5, 0xed, 0x47, 0x43, 0x6c, 0xae, 0x28, 0xea, 0x10, 0xdb, 0xa6, 0x73, 0xc7, 0x72, 0x6c, 0xf4,
	0x00, 0x03, 0x54, 0xad, 0x33, 0x67, 0xf4, 0xda, 0x46, 0x9a, 0x58, 0x9f, 0x93, 0xe9, 0x5b, 0x7b,
	0x82, 0x4a, 0xf8, 0x09, 0x3c, 0x1e, 0xd8, 0x33, 0x62, 0x9f, 0x59, 0x8e, 0x3d, 0xa0, 0xf3, 0xe9,
	0xb9, 0x43, 0x07, 0xf6, 0x95, 0xed, 0xd8, 0x03, 0x54, 0x6e, 0x95, 0x0c, 0xed, 0x1e, 0x61, 0x68,
	0x91, 0xc1, 0x96, 0xa0, 0x4b, 0x42, 0x13, 0x8c, 0x01, 0xb1, 0x46, 0x93, 0xd1, 0xe4, 0x02, 0x55,
	0x9e, 0x5e, 0x80, 0x51, 0x7c, 0x8e, 0xc4, 0xe6, 0x3f, 0xd8, 0x8b, 0xf3, 0x66, 0x26, 0xb6, 0x52,
	0x83, 0xf2, 0xd5, 0xf4, 0x02, 0x69, 0x62, 0x71, 0x6d, 0xcd, 0x50, 0x49, 0x1c, 0xc7, 0x8c, 0xd8,
	0x53, 0x32, 0xb0, 0x89, 0x3d, 0xa0, 0x02, 0x2c, 0xbf, 0x1a, 0xc2, 0xc9, 0x22, 0x5a, 0x17, 0x6f,
	0x80, 0x0f, 0x7f, 0x01, 0xbc, 0xda, 0x73, 0x54, 0x3c, 0x13, 0xe1, 0x4c, 0x7b, 0xdb, 0xf2, 0x7c,
	0xbe, 0xda, 0xdc, 0xf4, 0x16, 0xd1, 0xfa, 0x54, 0x7d, 0xa2, 0x0b, 0xc9, 0x4d, 0x55, 0x6a, 0x7e,
	0xfa, 0x37, 0x00, 0x00, 0xff, 0xff, 0x1a, 0xb8, 0x72, 0x37, 0x47, 0x08, 0x00, 0x00,
}
