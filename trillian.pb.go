// Code generated by protoc-gen-go.
// source: github.com/google/trillian/trillian.proto
// DO NOT EDIT!

package trillian

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This defines the way empty / node / leaf hashes are constructed incorporating
// preimage protection, which can be application specific.
type TreeHasherPreimageType int32

const (
	// For Certificate transparency leaf hash prefix = 0x00, node prefix = 0x01, empty hash
	// is digest([]byte{}) as defined in the specification
	TreeHasherPreimageType_RFC_6962_PREIMAGE TreeHasherPreimageType = 0
)

var TreeHasherPreimageType_name = map[int32]string{
	0: "RFC_6962_PREIMAGE",
}
var TreeHasherPreimageType_value = map[string]int32{
	"RFC_6962_PREIMAGE": 0,
}

func (x TreeHasherPreimageType) String() string {
	return proto.EnumName(TreeHasherPreimageType_name, int32(x))
}
func (TreeHasherPreimageType) EnumDescriptor() ([]byte, []int) { return fileDescriptor1, []int{0} }

type SignatureAlgorithm int32

const (
	SignatureAlgorithm_ECDSA SignatureAlgorithm = 0
	SignatureAlgorithm_RSA   SignatureAlgorithm = 1
)

var SignatureAlgorithm_name = map[int32]string{
	0: "ECDSA",
	1: "RSA",
}
var SignatureAlgorithm_value = map[string]int32{
	"ECDSA": 0,
	"RSA":   1,
}

func (x SignatureAlgorithm) String() string {
	return proto.EnumName(SignatureAlgorithm_name, int32(x))
}
func (SignatureAlgorithm) EnumDescriptor() ([]byte, []int) { return fileDescriptor1, []int{1} }

type HashAlgorithm int32

const (
	HashAlgorithm_SHA256 HashAlgorithm = 0
)

var HashAlgorithm_name = map[int32]string{
	0: "SHA256",
}
var HashAlgorithm_value = map[string]int32{
	"SHA256": 0,
}

func (x HashAlgorithm) String() string {
	return proto.EnumName(HashAlgorithm_name, int32(x))
}
func (HashAlgorithm) EnumDescriptor() ([]byte, []int) { return fileDescriptor1, []int{2} }

type DigitallySigned struct {
	SignatureAlgorithm SignatureAlgorithm `protobuf:"varint,1,opt,name=signature_algorithm,json=signatureAlgorithm,enum=trillian.SignatureAlgorithm" json:"signature_algorithm,omitempty"`
	HashAlgorithm      HashAlgorithm      `protobuf:"varint,2,opt,name=hash_algorithm,json=hashAlgorithm,enum=trillian.HashAlgorithm" json:"hash_algorithm,omitempty"`
	Signature          []byte             `protobuf:"bytes,3,opt,name=signature,proto3" json:"signature,omitempty"`
}

func (m *DigitallySigned) Reset()                    { *m = DigitallySigned{} }
func (m *DigitallySigned) String() string            { return proto.CompactTextString(m) }
func (*DigitallySigned) ProtoMessage()               {}
func (*DigitallySigned) Descriptor() ([]byte, []int) { return fileDescriptor1, []int{0} }

type SignedEntryTimestamp struct {
	TimestampNanos int64            `protobuf:"varint,1,opt,name=timestamp_nanos,json=timestampNanos" json:"timestamp_nanos,omitempty"`
	LogId          int64            `protobuf:"varint,2,opt,name=log_id,json=logId" json:"log_id,omitempty"`
	Signature      *DigitallySigned `protobuf:"bytes,3,opt,name=signature" json:"signature,omitempty"`
}

func (m *SignedEntryTimestamp) Reset()                    { *m = SignedEntryTimestamp{} }
func (m *SignedEntryTimestamp) String() string            { return proto.CompactTextString(m) }
func (*SignedEntryTimestamp) ProtoMessage()               {}
func (*SignedEntryTimestamp) Descriptor() ([]byte, []int) { return fileDescriptor1, []int{1} }

func (m *SignedEntryTimestamp) GetSignature() *DigitallySigned {
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
	TreeSize int64 `protobuf:"varint,3,opt,name=tree_size,json=treeSize" json:"tree_size,omitempty"`
	// TODO(al): define serialised format for the signature scheme.
	Signature    *DigitallySigned `protobuf:"bytes,4,opt,name=signature" json:"signature,omitempty"`
	LogId        int64            `protobuf:"varint,5,opt,name=log_id,json=logId" json:"log_id,omitempty"`
	TreeRevision int64            `protobuf:"varint,6,opt,name=tree_revision,json=treeRevision" json:"tree_revision,omitempty"`
}

func (m *SignedLogRoot) Reset()                    { *m = SignedLogRoot{} }
func (m *SignedLogRoot) String() string            { return proto.CompactTextString(m) }
func (*SignedLogRoot) ProtoMessage()               {}
func (*SignedLogRoot) Descriptor() ([]byte, []int) { return fileDescriptor1, []int{2} }

func (m *SignedLogRoot) GetSignature() *DigitallySigned {
	if m != nil {
		return m.Signature
	}
	return nil
}

type MapperMetadata struct {
	SourceLogId                  []byte `protobuf:"bytes,1,opt,name=source_log_id,json=sourceLogId,proto3" json:"source_log_id,omitempty"`
	HighestFullyCompletedSeq     int64  `protobuf:"varint,2,opt,name=highest_fully_completed_seq,json=highestFullyCompletedSeq" json:"highest_fully_completed_seq,omitempty"`
	HighestPartiallyCompletedSeq int64  `protobuf:"varint,3,opt,name=highest_partially_completed_seq,json=highestPartiallyCompletedSeq" json:"highest_partially_completed_seq,omitempty"`
}

func (m *MapperMetadata) Reset()                    { *m = MapperMetadata{} }
func (m *MapperMetadata) String() string            { return proto.CompactTextString(m) }
func (*MapperMetadata) ProtoMessage()               {}
func (*MapperMetadata) Descriptor() ([]byte, []int) { return fileDescriptor1, []int{3} }

// SignedMapRoot represents a commitment by a Map to a particular tree.
type SignedMapRoot struct {
	TimestampNanos int64           `protobuf:"varint,1,opt,name=timestamp_nanos,json=timestampNanos" json:"timestamp_nanos,omitempty"`
	RootHash       []byte          `protobuf:"bytes,2,opt,name=root_hash,json=rootHash,proto3" json:"root_hash,omitempty"`
	Metadata       *MapperMetadata `protobuf:"bytes,3,opt,name=metadata" json:"metadata,omitempty"`
	// TODO(al): define serialised format for the signature scheme.
	Signature   *DigitallySigned `protobuf:"bytes,4,opt,name=signature" json:"signature,omitempty"`
	MapId       int64            `protobuf:"varint,5,opt,name=map_id,json=mapId" json:"map_id,omitempty"`
	MapRevision int64            `protobuf:"varint,6,opt,name=map_revision,json=mapRevision" json:"map_revision,omitempty"`
}

func (m *SignedMapRoot) Reset()                    { *m = SignedMapRoot{} }
func (m *SignedMapRoot) String() string            { return proto.CompactTextString(m) }
func (*SignedMapRoot) ProtoMessage()               {}
func (*SignedMapRoot) Descriptor() ([]byte, []int) { return fileDescriptor1, []int{4} }

func (m *SignedMapRoot) GetMetadata() *MapperMetadata {
	if m != nil {
		return m.Metadata
	}
	return nil
}

func (m *SignedMapRoot) GetSignature() *DigitallySigned {
	if m != nil {
		return m.Signature
	}
	return nil
}

func init() {
	proto.RegisterType((*DigitallySigned)(nil), "trillian.DigitallySigned")
	proto.RegisterType((*SignedEntryTimestamp)(nil), "trillian.SignedEntryTimestamp")
	proto.RegisterType((*SignedLogRoot)(nil), "trillian.SignedLogRoot")
	proto.RegisterType((*MapperMetadata)(nil), "trillian.MapperMetadata")
	proto.RegisterType((*SignedMapRoot)(nil), "trillian.SignedMapRoot")
	proto.RegisterEnum("trillian.TreeHasherPreimageType", TreeHasherPreimageType_name, TreeHasherPreimageType_value)
	proto.RegisterEnum("trillian.SignatureAlgorithm", SignatureAlgorithm_name, SignatureAlgorithm_value)
	proto.RegisterEnum("trillian.HashAlgorithm", HashAlgorithm_name, HashAlgorithm_value)
}

func init() { proto.RegisterFile("github.com/google/trillian/trillian.proto", fileDescriptor1) }

var fileDescriptor1 = []byte{
	// 568 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x09, 0x6e, 0x88, 0x02, 0xff, 0xac, 0x54, 0xdd, 0x8e, 0xd2, 0x40,
	0x18, 0xdd, 0x8a, 0x8b, 0xf0, 0xf1, 0xb3, 0x38, 0xfe, 0x61, 0x20, 0x71, 0x65, 0x63, 0x44, 0x2e,
	0x20, 0x41, 0xc5, 0x18, 0xa3, 0x09, 0xb2, 0xac, 0x4b, 0xb2, 0x18, 0x32, 0xe5, 0xca, 0x9b, 0x66,
	0x96, 0x8e, 0xd3, 0x49, 0xda, 0x4e, 0x77, 0x3a, 0x98, 0xb0, 0x2f, 0xe1, 0xfb, 0x78, 0xe1, 0xeb,
	0xf8, 0x16, 0xc6, 0x4c, 0x69, 0x4b, 0x81, 0x98, 0x6c, 0xa2, 0x77, 0xf3, 0x9d, 0x39, 0x3d, 0x73,
	0xbe, 0xef, 0x74, 0x06, 0x5e, 0x30, 0xae, 0x9c, 0xe5, 0x65, 0x77, 0x21, 0xbc, 0x1e, 0x13, 0x82,
	0xb9, 0xb4, 0xa7, 0x24, 0x77, 0x5d, 0x4e, 0xfc, 0x74, 0xd1, 0x0d, 0xa4, 0x50, 0x02, 0x15, 0x92,
	0xba, 0xf5, 0xd3, 0x80, 0xa3, 0x53, 0xce, 0xb8, 0x22, 0xae, 0xbb, 0x32, 0x39, 0xf3, 0xa9, 0x8d,
	0xa6, 0x70, 0x2f, 0xe4, 0xcc, 0x27, 0x6a, 0x29, 0xa9, 0x45, 0x5c, 0x26, 0x24, 0x57, 0x8e, 0x57,
	0x37, 0x8e, 0x8d, 0x76, 0xb5, 0xdf, 0xec, 0xa6, 0x5a, 0x66, 0x42, 0x1a, 0x26, 0x1c, 0x8c, 0xc2,
	0x3d, 0x0c, 0x7d, 0x80, 0xaa, 0x43, 0x42, 0x27, 0xa3, 0x74, 0x2b, 0x52, 0x7a, 0xb4, 0x51, 0x3a,
	0x27, 0xa1, 0xb3, 0x11, 0xa9, 0x38, 0xd9, 0x12, 0x35, 0xa1, 0x98, 0xaa, 0xd6, 0x73, 0xc7, 0x46,
	0xbb, 0x8c, 0x37, 0x40, 0xeb, 0xbb, 0x01, 0xf7, 0xd7, 0xbe, 0xc7, 0xbe, 0x92, 0xab, 0x39, 0xf7,
	0x68, 0xa8, 0x88, 0x17, 0xa0, 0xe7, 0x70, 0xa4, 0x92, 0xc2, 0xf2, 0x89, 0x2f, 0xc2, 0xa8, 0x83,
	0x1c, 0xae, 0xa6, 0xf0, 0x67, 0x8d, 0xa2, 0x07, 0x90, 0x77, 0x05, 0xb3, 0xb8, 0x1d, 0xf9, 0xca,
	0xe1, 0x43, 0x57, 0xb0, 0x89, 0x8d, 0xde, 0xec, 0x1e, 0x5b, 0xea, 0x3f, 0xde, 0x38, 0xde, 0x99,
	0x59, 0xd6, 0xd1, 0x2f, 0x03, 0x2a, 0x6b, 0xf4, 0x42, 0x30, 0x2c, 0x84, 0xba, 0xb9, 0x95, 0x06,
	0x14, 0xa5, 0x10, 0xca, 0xd2, 0x03, 0x88, 0xdc, 0x94, 0x71, 0x41, 0x03, 0x7a, 0x3e, 0x7a, 0x53,
	0x49, 0x4a, 0xad, 0x90, 0x5f, 0xaf, 0x0d, 0xe5, 0x70, 0x41, 0x03, 0x26, 0xbf, 0xa6, 0xdb, 0x6e,
	0x6f, 0xdf, 0xdc, 0x6d, 0xa6, 0xfb, 0xc3, 0x6c, 0xf7, 0x27, 0x50, 0x89, 0x0e, 0x93, 0xf4, 0x1b,
	0x0f, 0xb9, 0xf0, 0xeb, 0xf9, 0x68, 0xb7, 0xac, 0x41, 0x1c, 0x63, 0xad, 0x1f, 0x06, 0x54, 0xa7,
	0x24, 0x08, 0xa8, 0x9c, 0x52, 0x45, 0x6c, 0xa2, 0x08, 0x6a, 0x41, 0x25, 0x14, 0x4b, 0xb9, 0xa0,
	0x56, 0xac, 0x6a, 0x44, 0x5d, 0x94, 0xd6, 0xe0, 0x45, 0xa4, 0xfd, 0x1e, 0x1a, 0x0e, 0x67, 0x0e,
	0x0d, 0x95, 0xf5, 0x75, 0xe9, 0xba, 0x2b, 0x6b, 0x21, 0xbc, 0xc0, 0xa5, 0x8a, 0xda, 0x56, 0x48,
	0xaf, 0xe2, 0x14, 0xea, 0x31, 0xe5, 0x4c, 0x33, 0x46, 0x09, 0xc1, 0xa4, 0x57, 0x68, 0x0c, 0x4f,
	0x92, 0xcf, 0x03, 0x22, 0x15, 0x27, 0xfb, 0x12, 0xeb, 0xe9, 0x34, 0x63, 0xda, 0x2c, 0x61, 0x65,
	0x65, 0x5a, 0xbf, 0xd3, 0x98, 0xa6, 0x24, 0xf8, 0x8f, 0x31, 0xbd, 0x82, 0x82, 0x17, 0x4f, 0x23,
	0xfe, 0x6d, 0xea, 0x9b, 0x20, 0xb6, 0xa7, 0x85, 0x53, 0xe6, 0x3f, 0xe5, 0xe7, 0x91, 0x20, 0x93,
	0x9f, 0x47, 0x82, 0x89, 0x8d, 0x9e, 0x42, 0x59, 0xc3, 0x3b, 0xf1, 0x95, 0x3c, 0x12, 0x24, 0xe9,
	0x75, 0x7a, 0xf0, 0x70, 0x2e, 0x29, 0xd5, 0xa6, 0xa9, 0x9c, 0x49, 0xca, 0x3d, 0xc2, 0xe8, 0x7c,
	0x15, 0x68, 0xcd, 0xbb, 0xf8, 0x6c, 0x64, 0x0d, 0xde, 0x0e, 0xfa, 0xd6, 0x0c, 0x8f, 0x27, 0xd3,
	0xe1, 0xa7, 0x71, 0xed, 0xa0, 0xd3, 0x06, 0xb4, 0x7f, 0xe5, 0x51, 0x11, 0x0e, 0xc7, 0xa3, 0x53,
	0x73, 0x58, 0x3b, 0x40, 0x77, 0x20, 0x87, 0xcd, 0x61, 0xcd, 0xe8, 0x34, 0xa0, 0xb2, 0x75, 0xa5,
	0x11, 0x40, 0xde, 0x3c, 0x1f, 0xf6, 0x5f, 0x0f, 0x6a, 0x07, 0x1f, 0x9f, 0x7d, 0x39, 0xf9, 0xfb,
	0x4b, 0xf5, 0x2e, 0x59, 0x5c, 0xe6, 0xa3, 0xa7, 0xea, 0xe5, 0x9f, 0x00, 0x00, 0x00, 0xff, 0xff,
	0xd7, 0x4d, 0x2f, 0xcc, 0xd7, 0x04, 0x00, 0x00,
}
