// Code generated by protoc-gen-go. DO NOT EDIT.
// source: spanner.proto

package spannerpb // import "github.com/google/trillian/storage/cloudspanner/spannerpb"

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"
import any "github.com/golang/protobuf/ptypes/any"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion2 // please upgrade the proto package

// State of the Tree.
// Mirrors trillian.TreeState.
type TreeState int32

const (
	TreeState_UNKNOWN_TREE_STATE TreeState = 0
	TreeState_ACTIVE             TreeState = 1
	TreeState_FROZEN             TreeState = 2
)

var TreeState_name = map[int32]string{
	0: "UNKNOWN_TREE_STATE",
	1: "ACTIVE",
	2: "FROZEN",
}
var TreeState_value = map[string]int32{
	"UNKNOWN_TREE_STATE": 0,
	"ACTIVE":             1,
	"FROZEN":             2,
}

func (x TreeState) String() string {
	return proto.EnumName(TreeState_name, int32(x))
}
func (TreeState) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_spanner_9d8b0e1d0a3a47ae, []int{0}
}

// Type of the Tree.
// Mirrors trillian.TreeType.
type TreeType int32

const (
	TreeType_UNKNOWN TreeType = 0
	TreeType_LOG     TreeType = 1
	TreeType_MAP     TreeType = 2
)

var TreeType_name = map[int32]string{
	0: "UNKNOWN",
	1: "LOG",
	2: "MAP",
}
var TreeType_value = map[string]int32{
	"UNKNOWN": 0,
	"LOG":     1,
	"MAP":     2,
}

func (x TreeType) String() string {
	return proto.EnumName(TreeType_name, int32(x))
}
func (TreeType) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_spanner_9d8b0e1d0a3a47ae, []int{1}
}

// Defines the preimage protection used for tree leaves / nodes.
// Eg, RFC6962 dictates a 0x00 prefix for leaves and 0x01 for nodes.
// Mirrors trillian.HashStrategy.
type HashStrategy int32

const (
	HashStrategy_UNKNOWN_HASH_STRATEGY HashStrategy = 0
	HashStrategy_RFC_6962              HashStrategy = 1
	HashStrategy_TEST_MAP_HASHER       HashStrategy = 2
	HashStrategy_OBJECT_RFC6962_SHA256 HashStrategy = 3
	HashStrategy_CONIKS_SHA512_256     HashStrategy = 4
)

var HashStrategy_name = map[int32]string{
	0: "UNKNOWN_HASH_STRATEGY",
	1: "RFC_6962",
	2: "TEST_MAP_HASHER",
	3: "OBJECT_RFC6962_SHA256",
	4: "CONIKS_SHA512_256",
}
var HashStrategy_value = map[string]int32{
	"UNKNOWN_HASH_STRATEGY": 0,
	"RFC_6962":              1,
	"TEST_MAP_HASHER":       2,
	"OBJECT_RFC6962_SHA256": 3,
	"CONIKS_SHA512_256":     4,
}

func (x HashStrategy) String() string {
	return proto.EnumName(HashStrategy_name, int32(x))
}
func (HashStrategy) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_spanner_9d8b0e1d0a3a47ae, []int{2}
}

// Supported hash algorithms.
// The numbering space is the same as for TLS, given in RFC 5246 s7.4.1.4.1. See
// http://www.iana.org/assignments/tls-parameters/tls-parameters.xhtml#tls-parameters-18.
// Mirrors trillian.HashAlgorithm.
type HashAlgorithm int32

const (
	// No hash algorithm is used.
	HashAlgorithm_NONE HashAlgorithm = 0
	// SHA256 is used.
	HashAlgorithm_SHA256 HashAlgorithm = 4
)

var HashAlgorithm_name = map[int32]string{
	0: "NONE",
	4: "SHA256",
}
var HashAlgorithm_value = map[string]int32{
	"NONE":   0,
	"SHA256": 4,
}

func (x HashAlgorithm) String() string {
	return proto.EnumName(HashAlgorithm_name, int32(x))
}
func (HashAlgorithm) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_spanner_9d8b0e1d0a3a47ae, []int{3}
}

// Supported signature algorithms.
// The numbering space is the same as for TLS, given in RFC 5246 s7.4.1.4.1. See
// http://www.iana.org/assignments/tls-parameters/tls-parameters.xhtml#tls-parameters-16.
// Mirrors trillian.SignatureAlgorithm.
type SignatureAlgorithm int32

const (
	// Anonymous signature scheme.
	SignatureAlgorithm_ANONYMOUS SignatureAlgorithm = 0
	// RSA signature scheme.
	SignatureAlgorithm_RSA SignatureAlgorithm = 1
	// ECDSA signature scheme.
	SignatureAlgorithm_ECDSA SignatureAlgorithm = 3
)

var SignatureAlgorithm_name = map[int32]string{
	0: "ANONYMOUS",
	1: "RSA",
	3: "ECDSA",
}
var SignatureAlgorithm_value = map[string]int32{
	"ANONYMOUS": 0,
	"RSA":       1,
	"ECDSA":     3,
}

func (x SignatureAlgorithm) String() string {
	return proto.EnumName(SignatureAlgorithm_name, int32(x))
}
func (SignatureAlgorithm) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_spanner_9d8b0e1d0a3a47ae, []int{4}
}

// LogStorageConfig holds settings which tune the storage implementation for
// a given log tree.
type LogStorageConfig struct {
	// num_unseq_buckets defines the length of the unsequenced time ring buffer.
	// This value must *never* be reduced for any provisioned tree.
	//
	// This value should be >= 1, and there's probably not much benefit in
	// raising it past about 4.
	// TODO(al): test what the effects of various values are here.
	NumUnseqBuckets int64 `protobuf:"varint,1,opt,name=num_unseq_buckets,json=numUnseqBuckets" json:"num_unseq_buckets,omitempty"`
	// num_merkle_buckets defines the number of individual buckets below each
	// unsequenced ring bucket.
	// This value may be changed at any time (so long as you understand the
	// impact it'll have on integration performace!)
	//
	// This value must lie in the range [1..256]
	NumMerkleBuckets     int64    `protobuf:"varint,2,opt,name=num_merkle_buckets,json=numMerkleBuckets" json:"num_merkle_buckets,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *LogStorageConfig) Reset()         { *m = LogStorageConfig{} }
func (m *LogStorageConfig) String() string { return proto.CompactTextString(m) }
func (*LogStorageConfig) ProtoMessage()    {}
func (*LogStorageConfig) Descriptor() ([]byte, []int) {
	return fileDescriptor_spanner_9d8b0e1d0a3a47ae, []int{0}
}
func (m *LogStorageConfig) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_LogStorageConfig.Unmarshal(m, b)
}
func (m *LogStorageConfig) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_LogStorageConfig.Marshal(b, m, deterministic)
}
func (dst *LogStorageConfig) XXX_Merge(src proto.Message) {
	xxx_messageInfo_LogStorageConfig.Merge(dst, src)
}
func (m *LogStorageConfig) XXX_Size() int {
	return xxx_messageInfo_LogStorageConfig.Size(m)
}
func (m *LogStorageConfig) XXX_DiscardUnknown() {
	xxx_messageInfo_LogStorageConfig.DiscardUnknown(m)
}

var xxx_messageInfo_LogStorageConfig proto.InternalMessageInfo

func (m *LogStorageConfig) GetNumUnseqBuckets() int64 {
	if m != nil {
		return m.NumUnseqBuckets
	}
	return 0
}

func (m *LogStorageConfig) GetNumMerkleBuckets() int64 {
	if m != nil {
		return m.NumMerkleBuckets
	}
	return 0
}

// MapStorageConfig holds settings which tune the storage implementation for
// a given map tree.
type MapStorageConfig struct {
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *MapStorageConfig) Reset()         { *m = MapStorageConfig{} }
func (m *MapStorageConfig) String() string { return proto.CompactTextString(m) }
func (*MapStorageConfig) ProtoMessage()    {}
func (*MapStorageConfig) Descriptor() ([]byte, []int) {
	return fileDescriptor_spanner_9d8b0e1d0a3a47ae, []int{1}
}
func (m *MapStorageConfig) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_MapStorageConfig.Unmarshal(m, b)
}
func (m *MapStorageConfig) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_MapStorageConfig.Marshal(b, m, deterministic)
}
func (dst *MapStorageConfig) XXX_Merge(src proto.Message) {
	xxx_messageInfo_MapStorageConfig.Merge(dst, src)
}
func (m *MapStorageConfig) XXX_Size() int {
	return xxx_messageInfo_MapStorageConfig.Size(m)
}
func (m *MapStorageConfig) XXX_DiscardUnknown() {
	xxx_messageInfo_MapStorageConfig.DiscardUnknown(m)
}

var xxx_messageInfo_MapStorageConfig proto.InternalMessageInfo

// TreeInfo stores information about a Trillian tree.
type TreeInfo struct {
	// tree_id is the ID of the tree, and is used as a primary key.
	TreeId int64 `protobuf:"varint,1,opt,name=tree_id,json=treeId" json:"tree_id,omitempty"`
	// key_id identifies the private key associated with this tree.
	KeyId int64 `protobuf:"varint,2,opt,name=key_id,json=keyId" json:"key_id,omitempty"`
	// name is a short name for this tree.
	Name string `protobuf:"bytes,3,opt,name=name" json:"name,omitempty"`
	// description is a short free form text describing the tree.
	Description string `protobuf:"bytes,4,opt,name=description" json:"description,omitempty"`
	// tree_type identifies whether this is a Log or a Map tree.
	TreeType TreeType `protobuf:"varint,5,opt,name=tree_type,json=treeType,enum=spannerpb.TreeType" json:"tree_type,omitempty"`
	// tree_state is the state of the tree.
	TreeState TreeState `protobuf:"varint,8,opt,name=tree_state,json=treeState,enum=spannerpb.TreeState" json:"tree_state,omitempty"`
	// hash_strategy is the hashing strategy used by the tree.
	HashStrategy HashStrategy `protobuf:"varint,9,opt,name=hash_strategy,json=hashStrategy,enum=spannerpb.HashStrategy" json:"hash_strategy,omitempty"`
	// hash_algorithm is the hash algorithm used by the tree.
	HashAlgorithm HashAlgorithm `protobuf:"varint,10,opt,name=hash_algorithm,json=hashAlgorithm,enum=spannerpb.HashAlgorithm" json:"hash_algorithm,omitempty"`
	// signature_algorithm is the signature algorithm used by the tree.
	SignatureAlgorithm SignatureAlgorithm `protobuf:"varint,11,opt,name=signature_algorithm,json=signatureAlgorithm,enum=spannerpb.SignatureAlgorithm" json:"signature_algorithm,omitempty"`
	// create_time_nanos is the creation timestamp of the tree, in nanos since
	// epoch.
	CreateTimeNanos int64 `protobuf:"varint,13,opt,name=create_time_nanos,json=createTimeNanos" json:"create_time_nanos,omitempty"`
	// update_time_nanos is the last update time of the tree, in nanos since
	// epoch.
	UpdateTimeNanos int64 `protobuf:"varint,14,opt,name=update_time_nanos,json=updateTimeNanos" json:"update_time_nanos,omitempty"`
	// private_key should be used to generate signatures for this tree.
	PrivateKey *any.Any `protobuf:"bytes,15,opt,name=private_key,json=privateKey" json:"private_key,omitempty"`
	// public_key_der should be used to verify signatures produced by this tree.
	// It is the key in DER-encoded PKIX form.
	PublicKeyDer []byte `protobuf:"bytes,16,opt,name=public_key_der,json=publicKeyDer,proto3" json:"public_key_der,omitempty"`
	// config contains the log or map specific tree configuration.
	//
	// Types that are valid to be assigned to StorageConfig:
	//	*TreeInfo_LogStorageConfig
	//	*TreeInfo_MapStorageConfig
	StorageConfig isTreeInfo_StorageConfig `protobuf_oneof:"storage_config"`
	// max_root_duration_millis is the interval after which a new signed root is
	// produced even if there have been no submission.  If zero, this behavior is
	// disabled.
	MaxRootDurationMillis int64 `protobuf:"varint,17,opt,name=max_root_duration_millis,json=maxRootDurationMillis" json:"max_root_duration_millis,omitempty"`
	// If true the tree was soft deleted.
	Deleted bool `protobuf:"varint,18,opt,name=deleted" json:"deleted,omitempty"`
	// Time of tree deletion, if any.
	DeleteTimeNanos      int64    `protobuf:"varint,19,opt,name=delete_time_nanos,json=deleteTimeNanos" json:"delete_time_nanos,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *TreeInfo) Reset()         { *m = TreeInfo{} }
func (m *TreeInfo) String() string { return proto.CompactTextString(m) }
func (*TreeInfo) ProtoMessage()    {}
func (*TreeInfo) Descriptor() ([]byte, []int) {
	return fileDescriptor_spanner_9d8b0e1d0a3a47ae, []int{2}
}
func (m *TreeInfo) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_TreeInfo.Unmarshal(m, b)
}
func (m *TreeInfo) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_TreeInfo.Marshal(b, m, deterministic)
}
func (dst *TreeInfo) XXX_Merge(src proto.Message) {
	xxx_messageInfo_TreeInfo.Merge(dst, src)
}
func (m *TreeInfo) XXX_Size() int {
	return xxx_messageInfo_TreeInfo.Size(m)
}
func (m *TreeInfo) XXX_DiscardUnknown() {
	xxx_messageInfo_TreeInfo.DiscardUnknown(m)
}

var xxx_messageInfo_TreeInfo proto.InternalMessageInfo

type isTreeInfo_StorageConfig interface {
	isTreeInfo_StorageConfig()
}

type TreeInfo_LogStorageConfig struct {
	LogStorageConfig *LogStorageConfig `protobuf:"bytes,6,opt,name=log_storage_config,json=logStorageConfig,oneof"`
}
type TreeInfo_MapStorageConfig struct {
	MapStorageConfig *MapStorageConfig `protobuf:"bytes,7,opt,name=map_storage_config,json=mapStorageConfig,oneof"`
}

func (*TreeInfo_LogStorageConfig) isTreeInfo_StorageConfig() {}
func (*TreeInfo_MapStorageConfig) isTreeInfo_StorageConfig() {}

func (m *TreeInfo) GetStorageConfig() isTreeInfo_StorageConfig {
	if m != nil {
		return m.StorageConfig
	}
	return nil
}

func (m *TreeInfo) GetTreeId() int64 {
	if m != nil {
		return m.TreeId
	}
	return 0
}

func (m *TreeInfo) GetKeyId() int64 {
	if m != nil {
		return m.KeyId
	}
	return 0
}

func (m *TreeInfo) GetName() string {
	if m != nil {
		return m.Name
	}
	return ""
}

func (m *TreeInfo) GetDescription() string {
	if m != nil {
		return m.Description
	}
	return ""
}

func (m *TreeInfo) GetTreeType() TreeType {
	if m != nil {
		return m.TreeType
	}
	return TreeType_UNKNOWN
}

func (m *TreeInfo) GetTreeState() TreeState {
	if m != nil {
		return m.TreeState
	}
	return TreeState_UNKNOWN_TREE_STATE
}

func (m *TreeInfo) GetHashStrategy() HashStrategy {
	if m != nil {
		return m.HashStrategy
	}
	return HashStrategy_UNKNOWN_HASH_STRATEGY
}

func (m *TreeInfo) GetHashAlgorithm() HashAlgorithm {
	if m != nil {
		return m.HashAlgorithm
	}
	return HashAlgorithm_NONE
}

func (m *TreeInfo) GetSignatureAlgorithm() SignatureAlgorithm {
	if m != nil {
		return m.SignatureAlgorithm
	}
	return SignatureAlgorithm_ANONYMOUS
}

func (m *TreeInfo) GetCreateTimeNanos() int64 {
	if m != nil {
		return m.CreateTimeNanos
	}
	return 0
}

func (m *TreeInfo) GetUpdateTimeNanos() int64 {
	if m != nil {
		return m.UpdateTimeNanos
	}
	return 0
}

func (m *TreeInfo) GetPrivateKey() *any.Any {
	if m != nil {
		return m.PrivateKey
	}
	return nil
}

func (m *TreeInfo) GetPublicKeyDer() []byte {
	if m != nil {
		return m.PublicKeyDer
	}
	return nil
}

func (m *TreeInfo) GetLogStorageConfig() *LogStorageConfig {
	if x, ok := m.GetStorageConfig().(*TreeInfo_LogStorageConfig); ok {
		return x.LogStorageConfig
	}
	return nil
}

func (m *TreeInfo) GetMapStorageConfig() *MapStorageConfig {
	if x, ok := m.GetStorageConfig().(*TreeInfo_MapStorageConfig); ok {
		return x.MapStorageConfig
	}
	return nil
}

func (m *TreeInfo) GetMaxRootDurationMillis() int64 {
	if m != nil {
		return m.MaxRootDurationMillis
	}
	return 0
}

func (m *TreeInfo) GetDeleted() bool {
	if m != nil {
		return m.Deleted
	}
	return false
}

func (m *TreeInfo) GetDeleteTimeNanos() int64 {
	if m != nil {
		return m.DeleteTimeNanos
	}
	return 0
}

// XXX_OneofFuncs is for the internal use of the proto package.
func (*TreeInfo) XXX_OneofFuncs() (func(msg proto.Message, b *proto.Buffer) error, func(msg proto.Message, tag, wire int, b *proto.Buffer) (bool, error), func(msg proto.Message) (n int), []interface{}) {
	return _TreeInfo_OneofMarshaler, _TreeInfo_OneofUnmarshaler, _TreeInfo_OneofSizer, []interface{}{
		(*TreeInfo_LogStorageConfig)(nil),
		(*TreeInfo_MapStorageConfig)(nil),
	}
}

func _TreeInfo_OneofMarshaler(msg proto.Message, b *proto.Buffer) error {
	m := msg.(*TreeInfo)
	// storage_config
	switch x := m.StorageConfig.(type) {
	case *TreeInfo_LogStorageConfig:
		b.EncodeVarint(6<<3 | proto.WireBytes)
		if err := b.EncodeMessage(x.LogStorageConfig); err != nil {
			return err
		}
	case *TreeInfo_MapStorageConfig:
		b.EncodeVarint(7<<3 | proto.WireBytes)
		if err := b.EncodeMessage(x.MapStorageConfig); err != nil {
			return err
		}
	case nil:
	default:
		return fmt.Errorf("TreeInfo.StorageConfig has unexpected type %T", x)
	}
	return nil
}

func _TreeInfo_OneofUnmarshaler(msg proto.Message, tag, wire int, b *proto.Buffer) (bool, error) {
	m := msg.(*TreeInfo)
	switch tag {
	case 6: // storage_config.log_storage_config
		if wire != proto.WireBytes {
			return true, proto.ErrInternalBadWireType
		}
		msg := new(LogStorageConfig)
		err := b.DecodeMessage(msg)
		m.StorageConfig = &TreeInfo_LogStorageConfig{msg}
		return true, err
	case 7: // storage_config.map_storage_config
		if wire != proto.WireBytes {
			return true, proto.ErrInternalBadWireType
		}
		msg := new(MapStorageConfig)
		err := b.DecodeMessage(msg)
		m.StorageConfig = &TreeInfo_MapStorageConfig{msg}
		return true, err
	default:
		return false, nil
	}
}

func _TreeInfo_OneofSizer(msg proto.Message) (n int) {
	m := msg.(*TreeInfo)
	// storage_config
	switch x := m.StorageConfig.(type) {
	case *TreeInfo_LogStorageConfig:
		s := proto.Size(x.LogStorageConfig)
		n += 1 // tag and wire
		n += proto.SizeVarint(uint64(s))
		n += s
	case *TreeInfo_MapStorageConfig:
		s := proto.Size(x.MapStorageConfig)
		n += 1 // tag and wire
		n += proto.SizeVarint(uint64(s))
		n += s
	case nil:
	default:
		panic(fmt.Sprintf("proto: unexpected type %T in oneof", x))
	}
	return n
}

// TreeHead is the storage format for Trillian's commitment to a particular
// tree state.
type TreeHead struct {
	// tree_id identifies the tree this TreeHead is built from.
	TreeId int64 `protobuf:"varint,1,opt,name=tree_id,json=treeId" json:"tree_id,omitempty"`
	// ts_nanos is the nanosecond resolution timestamp at which the
	// TreeHead was created.
	TsNanos int64 `protobuf:"varint,2,opt,name=ts_nanos,json=tsNanos" json:"ts_nanos,omitempty"`
	// tree_size is the number of entries in the tree.
	TreeSize int64 `protobuf:"varint,3,opt,name=tree_size,json=treeSize" json:"tree_size,omitempty"`
	// root_hash is the root of the tree.
	RootHash []byte `protobuf:"bytes,4,opt,name=root_hash,json=rootHash,proto3" json:"root_hash,omitempty"`
	// signature holds the raw digital signature across the serialized log_root
	// (not present) represented by the data in this TreeHead.
	Signature []byte `protobuf:"bytes,10,opt,name=signature,proto3" json:"signature,omitempty"`
	// tree_revision identifies the revision at which the TreeHead was created.
	TreeRevision         int64    `protobuf:"varint,6,opt,name=tree_revision,json=treeRevision" json:"tree_revision,omitempty"`
	Metadata             []byte   `protobuf:"bytes,9,opt,name=metadata,proto3" json:"metadata,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *TreeHead) Reset()         { *m = TreeHead{} }
func (m *TreeHead) String() string { return proto.CompactTextString(m) }
func (*TreeHead) ProtoMessage()    {}
func (*TreeHead) Descriptor() ([]byte, []int) {
	return fileDescriptor_spanner_9d8b0e1d0a3a47ae, []int{3}
}
func (m *TreeHead) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_TreeHead.Unmarshal(m, b)
}
func (m *TreeHead) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_TreeHead.Marshal(b, m, deterministic)
}
func (dst *TreeHead) XXX_Merge(src proto.Message) {
	xxx_messageInfo_TreeHead.Merge(dst, src)
}
func (m *TreeHead) XXX_Size() int {
	return xxx_messageInfo_TreeHead.Size(m)
}
func (m *TreeHead) XXX_DiscardUnknown() {
	xxx_messageInfo_TreeHead.DiscardUnknown(m)
}

var xxx_messageInfo_TreeHead proto.InternalMessageInfo

func (m *TreeHead) GetTreeId() int64 {
	if m != nil {
		return m.TreeId
	}
	return 0
}

func (m *TreeHead) GetTsNanos() int64 {
	if m != nil {
		return m.TsNanos
	}
	return 0
}

func (m *TreeHead) GetTreeSize() int64 {
	if m != nil {
		return m.TreeSize
	}
	return 0
}

func (m *TreeHead) GetRootHash() []byte {
	if m != nil {
		return m.RootHash
	}
	return nil
}

func (m *TreeHead) GetSignature() []byte {
	if m != nil {
		return m.Signature
	}
	return nil
}

func (m *TreeHead) GetTreeRevision() int64 {
	if m != nil {
		return m.TreeRevision
	}
	return 0
}

func (m *TreeHead) GetMetadata() []byte {
	if m != nil {
		return m.Metadata
	}
	return nil
}

func init() {
	proto.RegisterType((*LogStorageConfig)(nil), "spannerpb.LogStorageConfig")
	proto.RegisterType((*MapStorageConfig)(nil), "spannerpb.MapStorageConfig")
	proto.RegisterType((*TreeInfo)(nil), "spannerpb.TreeInfo")
	proto.RegisterType((*TreeHead)(nil), "spannerpb.TreeHead")
	proto.RegisterEnum("spannerpb.TreeState", TreeState_name, TreeState_value)
	proto.RegisterEnum("spannerpb.TreeType", TreeType_name, TreeType_value)
	proto.RegisterEnum("spannerpb.HashStrategy", HashStrategy_name, HashStrategy_value)
	proto.RegisterEnum("spannerpb.HashAlgorithm", HashAlgorithm_name, HashAlgorithm_value)
	proto.RegisterEnum("spannerpb.SignatureAlgorithm", SignatureAlgorithm_name, SignatureAlgorithm_value)
}

func init() { proto.RegisterFile("spanner.proto", fileDescriptor_spanner_9d8b0e1d0a3a47ae) }

var fileDescriptor_spanner_9d8b0e1d0a3a47ae = []byte{
	// 950 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x74, 0x55, 0xed, 0x6e, 0x1a, 0x47,
	0x14, 0xf5, 0x1a, 0x0c, 0xcb, 0x35, 0xd8, 0xe3, 0x71, 0xdc, 0xac, 0x93, 0x56, 0x42, 0x6e, 0x2b,
	0x51, 0x54, 0x41, 0xeb, 0xc8, 0x8e, 0xa2, 0x54, 0xaa, 0xd6, 0x18, 0x07, 0x9b, 0xb0, 0x44, 0xb3,
	0xeb, 0x56, 0xc9, 0x9f, 0xd5, 0xc0, 0x8e, 0x61, 0xe5, 0xfd, 0xea, 0xee, 0x6c, 0x14, 0xf2, 0xa3,
	0x4f, 0xd0, 0x17, 0xed, 0x5b, 0x54, 0x33, 0xb3, 0x60, 0x8c, 0xd5, 0x7f, 0x33, 0xe7, 0x9e, 0x73,
	0xaf, 0xe7, 0xfa, 0x9c, 0x05, 0x1a, 0x59, 0x42, 0xa3, 0x88, 0xa5, 0x9d, 0x24, 0x8d, 0x79, 0x8c,
	0x6b, 0xc5, 0x35, 0x99, 0xbc, 0x38, 0x9e, 0xc5, 0xf1, 0x2c, 0x60, 0x5d, 0x59, 0x98, 0xe4, 0x77,
	0x5d, 0x1a, 0x2d, 0x14, 0xeb, 0x24, 0x00, 0xf4, 0x3e, 0x9e, 0xd9, 0x3c, 0x4e, 0xe9, 0x8c, 0xf5,
	0xe2, 0xe8, 0xce, 0x9f, 0xe1, 0x36, 0x1c, 0x44, 0x79, 0xe8, 0xe6, 0x51, 0xc6, 0xfe, 0x72, 0x27,
	0xf9, 0xf4, 0x9e, 0xf1, 0xcc, 0xd0, 0x9a, 0x5a, 0xab, 0x44, 0xf6, 0xa3, 0x3c, 0xbc, 0x15, 0xf8,
	0x85, 0x82, 0xf1, 0xcf, 0x80, 0x05, 0x37, 0x64, 0xe9, 0x7d, 0xc0, 0x56, 0xe4, 0x6d, 0x49, 0x46,
	0x51, 0x1e, 0x8e, 0x64, 0xa1, 0x60, 0x9f, 0x60, 0x40, 0x23, 0x9a, 0x3c, 0x9a, 0x76, 0xf2, 0x4f,
	0x15, 0x74, 0x27, 0x65, 0xec, 0x3a, 0xba, 0x8b, 0xf1, 0x73, 0xa8, 0xf2, 0x94, 0x31, 0xd7, 0xf7,
	0x8a, 0x81, 0x15, 0x71, 0xbd, 0xf6, 0xf0, 0x11, 0x54, 0xee, 0xd9, 0x42, 0xe0, 0xaa, 0xf7, 0xce,
	0x3d, 0x5b, 0x5c, 0x7b, 0x18, 0x43, 0x39, 0xa2, 0x21, 0x33, 0x4a, 0x4d, 0xad, 0x55, 0x23, 0xf2,
	0x8c, 0x9b, 0xb0, 0xeb, 0xb1, 0x6c, 0x9a, 0xfa, 0x09, 0xf7, 0xe3, 0xc8, 0x28, 0xcb, 0xd2, 0x3a,
	0x84, 0x7f, 0x81, 0x9a, 0x9c, 0xc2, 0x17, 0x09, 0x33, 0x76, 0x9a, 0x5a, 0x6b, 0xef, 0xf4, 0xb0,
	0xb3, 0x5a, 0x57, 0x47, 0xfc, 0x35, 0xce, 0x22, 0x61, 0x44, 0xe7, 0xc5, 0x09, 0xbf, 0x02, 0x90,
	0x8a, 0x8c, 0x53, 0xce, 0x0c, 0x5d, 0x4a, 0x9e, 0x6d, 0x48, 0x6c, 0x51, 0x23, 0xb2, 0xb3, 0x3c,
	0xe2, 0xdf, 0xa0, 0x31, 0xa7, 0xd9, 0xdc, 0xcd, 0x78, 0x4a, 0x39, 0x9b, 0x2d, 0x8c, 0x9a, 0xd4,
	0x3d, 0x5f, 0xd3, 0x0d, 0x68, 0x36, 0xb7, 0x8b, 0x32, 0xa9, 0xcf, 0xd7, 0x6e, 0xf8, 0x77, 0xd8,
	0x93, 0x6a, 0x1a, 0xcc, 0xe2, 0xd4, 0xe7, 0xf3, 0xd0, 0x00, 0x29, 0x37, 0x36, 0xe4, 0xe6, 0xb2,
	0x4e, 0xe4, 0xb4, 0xd5, 0x15, 0x5b, 0x70, 0x98, 0xf9, 0xb3, 0x88, 0xf2, 0x3c, 0x65, 0x6b, 0x5d,
	0x76, 0x65, 0x97, 0xef, 0xd6, 0xba, 0xd8, 0x4b, 0xd6, 0x43, 0x2b, 0x9c, 0x3d, 0xc1, 0x84, 0x2d,
	0xa6, 0x29, 0xa3, 0x9c, 0xb9, 0xdc, 0x0f, 0x99, 0x1b, 0xd1, 0x28, 0xce, 0x8c, 0x86, 0xb2, 0x85,
	0x2a, 0x38, 0x7e, 0xc8, 0x2c, 0x01, 0x0b, 0x6e, 0x9e, 0x78, 0x1b, 0xdc, 0x3d, 0xc5, 0x55, 0x85,
	0x07, 0xee, 0x19, 0xec, 0x26, 0xa9, 0xff, 0x59, 0x90, 0xef, 0xd9, 0xc2, 0xd8, 0x6f, 0x6a, 0xad,
	0xdd, 0xd3, 0x67, 0x1d, 0xe5, 0xd9, 0xce, 0xd2, 0xb3, 0x1d, 0x33, 0x5a, 0x10, 0x28, 0x88, 0x43,
	0xb6, 0xc0, 0x3f, 0xc0, 0x5e, 0x92, 0x4f, 0x02, 0x7f, 0x2a, 0x54, 0xae, 0xc7, 0x52, 0x03, 0x35,
	0xb5, 0x56, 0x9d, 0xd4, 0x15, 0x3a, 0x64, 0x8b, 0x4b, 0x96, 0xe2, 0x21, 0xe0, 0x20, 0x9e, 0xb9,
	0x99, 0xb2, 0x9c, 0x3b, 0x95, 0x9e, 0x33, 0x2a, 0x72, 0xc6, 0xcb, 0xb5, 0x1d, 0x6c, 0x86, 0x60,
	0xb0, 0x45, 0x50, 0xb0, 0x19, 0x8c, 0x21, 0xe0, 0x90, 0x26, 0x9b, 0xcd, 0xaa, 0x4f, 0x9a, 0x6d,
	0x7a, 0x5c, 0x34, 0x0b, 0x37, 0x30, 0xfc, 0x1a, 0x8c, 0x90, 0x7e, 0x71, 0xd3, 0x38, 0xe6, 0xae,
	0x97, 0xa7, 0x54, 0x38, 0xd3, 0x0d, 0xfd, 0x20, 0xf0, 0x33, 0xe3, 0x40, 0x6e, 0xea, 0x28, 0xa4,
	0x5f, 0x48, 0x1c, 0xf3, 0xcb, 0xa2, 0x3a, 0x92, 0x45, 0x6c, 0x40, 0xd5, 0x63, 0x01, 0xe3, 0xcc,
	0x33, 0x70, 0x53, 0x6b, 0xe9, 0x64, 0x79, 0x15, 0x5b, 0x57, 0xc7, 0xf5, 0xad, 0x1f, 0xaa, 0xad,
	0xab, 0xc2, 0x6a, 0xeb, 0x17, 0x08, 0xf6, 0x1e, 0xbf, 0xe3, 0xa6, 0xac, 0xd7, 0x51, 0xe3, 0xe4,
	0x5f, 0x4d, 0xc5, 0x71, 0xc0, 0xa8, 0xf7, 0xff, 0x71, 0x3c, 0x06, 0x9d, 0x67, 0xc5, 0x00, 0x15,
	0xc8, 0x2a, 0xcf, 0xd4, 0xbf, 0xf3, 0x65, 0x11, 0xae, 0xcc, 0xff, 0xaa, 0x72, 0x59, 0x52, 0x39,
	0xb2, 0xfd, 0xaf, 0x4c, 0x14, 0xe5, 0x83, 0x85, 0x53, 0x65, 0x32, 0xeb, 0x44, 0x17, 0x80, 0x30,
	0x32, 0xfe, 0x16, 0x6a, 0x2b, 0xdb, 0x49, 0xb3, 0xd7, 0xc9, 0x03, 0x80, 0xbf, 0x87, 0x86, 0xec,
	0x9b, 0xb2, 0xcf, 0x7e, 0x26, 0x82, 0x5d, 0x91, 0xbd, 0xeb, 0x02, 0x24, 0x05, 0x86, 0x5f, 0x80,
	0x1e, 0x32, 0x4e, 0x3d, 0xca, 0xa9, 0x4c, 0x5b, 0x9d, 0xac, 0xee, 0x37, 0x65, 0x7d, 0x07, 0x55,
	0x6e, 0xca, 0xba, 0x8e, 0x6a, 0x37, 0x65, 0xbd, 0x8a, 0xf4, 0xf6, 0x5b, 0xa8, 0xad, 0x82, 0x8b,
	0xbf, 0x01, 0x7c, 0x6b, 0x0d, 0xad, 0xf1, 0x9f, 0x96, 0xeb, 0x90, 0x7e, 0xdf, 0xb5, 0x1d, 0xd3,
	0xe9, 0xa3, 0x2d, 0x0c, 0x50, 0x31, 0x7b, 0xce, 0xf5, 0x1f, 0x7d, 0xa4, 0x89, 0xf3, 0x15, 0x19,
	0x7f, 0xea, 0x5b, 0x68, 0xbb, 0xfd, 0x93, 0xda, 0x93, 0xfc, 0x3c, 0xec, 0x42, 0xb5, 0xd0, 0xa2,
	0x2d, 0x5c, 0x85, 0xd2, 0xfb, 0xf1, 0x3b, 0xa4, 0x89, 0xc3, 0xc8, 0xfc, 0x80, 0xb6, 0xdb, 0x7f,
	0x43, 0x7d, 0x3d, 0xe8, 0xf8, 0x18, 0x8e, 0x96, 0xa3, 0x06, 0xa6, 0x3d, 0x70, 0x6d, 0x87, 0x98,
	0x4e, 0xff, 0xdd, 0x47, 0xb4, 0x85, 0xeb, 0xa0, 0x93, 0xab, 0x9e, 0x7b, 0xfe, 0xe6, 0xfc, 0x14,
	0x69, 0xf8, 0x10, 0xf6, 0x9d, 0xbe, 0xed, 0xb8, 0x23, 0xf3, 0x83, 0x64, 0xf6, 0x09, 0xda, 0x16,
	0xea, 0xf1, 0xc5, 0x4d, 0xbf, 0xe7, 0xb8, 0xe4, 0xaa, 0x27, 0x88, 0xae, 0x3d, 0x30, 0x4f, 0xcf,
	0xce, 0x51, 0x09, 0x1f, 0xc1, 0x41, 0x6f, 0x6c, 0x5d, 0x0f, 0x6d, 0x01, 0x9d, 0xfd, 0x7a, 0xea,
	0x0a, 0xb8, 0xdc, 0xfe, 0x11, 0x1a, 0x8f, 0xbe, 0x14, 0x58, 0x87, 0xb2, 0x35, 0xb6, 0x8a, 0xd7,
	0x15, 0xea, 0x72, 0xfb, 0x35, 0xe0, 0xa7, 0x9f, 0x02, 0xdc, 0x80, 0x9a, 0x69, 0x8d, 0xad, 0x8f,
	0xa3, 0xf1, 0xad, 0xad, 0x5e, 0x47, 0x6c, 0x13, 0x69, 0xb8, 0x06, 0x3b, 0xfd, 0xde, 0xa5, 0x6d,
	0xa2, 0xd2, 0xc5, 0xdb, 0x4f, 0x6f, 0x66, 0x3e, 0x9f, 0xe7, 0x93, 0xce, 0x34, 0x0e, 0xbb, 0xc5,
	0x8f, 0x0d, 0x4f, 0x85, 0x5d, 0x69, 0xd4, 0x2d, 0x6c, 0xd6, 0x9d, 0x06, 0x71, 0xee, 0x15, 0x21,
	0xe9, 0xae, 0xc2, 0x32, 0xa9, 0xc8, 0x84, 0xbf, 0xfa, 0x2f, 0x00, 0x00, 0xff, 0xff, 0x4d, 0xcf,
	0xfb, 0xcd, 0xbf, 0x06, 0x00, 0x00,
}
