// Code generated by protoc-gen-go.
// source: trillian.proto
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

// Supported signature algorithms.  The numbering space is the same as for TLS,
// given in RFC 5246 s7.4.1.4.1 and at:
// http://www.iana.org/assignments/tls-parameters/tls-parameters.xhtml#tls-parameters-16
type SignatureAlgorithm int32

const (
	SignatureAlgorithm_ANONYMOUS SignatureAlgorithm = 0
	SignatureAlgorithm_RSA       SignatureAlgorithm = 1
	SignatureAlgorithm_ECDSA     SignatureAlgorithm = 3
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
func (SignatureAlgorithm) EnumDescriptor() ([]byte, []int) { return fileDescriptor1, []int{1} }

// Supported hash algorithms.  The numbering space is the same as for TLS,
// given in RFC 5246 s7.4.1.4.1 and at:
// http://www.iana.org/assignments/tls-parameters/tls-parameters.xhtml#tls-parameters-18
type HashAlgorithm int32

const (
	HashAlgorithm_NONE   HashAlgorithm = 0
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
func (HashAlgorithm) EnumDescriptor() ([]byte, []int) { return fileDescriptor1, []int{2} }

// Protocol buffer encoding of the TLS DigitallySigned type, from
// RFC 5246 s4.7.
type DigitallySigned struct {
	SignatureAlgorithm SignatureAlgorithm `protobuf:"varint,1,opt,name=signature_algorithm,json=signatureAlgorithm,enum=trillian.SignatureAlgorithm" json:"signature_algorithm,omitempty"`
	HashAlgorithm      HashAlgorithm      `protobuf:"varint,2,opt,name=hash_algorithm,json=hashAlgorithm,enum=trillian.HashAlgorithm" json:"hash_algorithm,omitempty"`
	Signature          []byte             `protobuf:"bytes,3,opt,name=signature,proto3" json:"signature,omitempty"`
}

func (m *DigitallySigned) Reset()                    { *m = DigitallySigned{} }
func (m *DigitallySigned) String() string            { return proto.CompactTextString(m) }
func (*DigitallySigned) ProtoMessage()               {}
func (*DigitallySigned) Descriptor() ([]byte, []int) { return fileDescriptor1, []int{0} }

func (m *DigitallySigned) GetSignatureAlgorithm() SignatureAlgorithm {
	if m != nil {
		return m.SignatureAlgorithm
	}
	return SignatureAlgorithm_ANONYMOUS
}

func (m *DigitallySigned) GetHashAlgorithm() HashAlgorithm {
	if m != nil {
		return m.HashAlgorithm
	}
	return HashAlgorithm_NONE
}

func (m *DigitallySigned) GetSignature() []byte {
	if m != nil {
		return m.Signature
	}
	return nil
}

type SignedEntryTimestamp struct {
	TimestampNanos int64            `protobuf:"varint,1,opt,name=timestamp_nanos,json=timestampNanos" json:"timestamp_nanos,omitempty"`
	LogId          int64            `protobuf:"varint,2,opt,name=log_id,json=logId" json:"log_id,omitempty"`
	Signature      *DigitallySigned `protobuf:"bytes,3,opt,name=signature" json:"signature,omitempty"`
}

func (m *SignedEntryTimestamp) Reset()                    { *m = SignedEntryTimestamp{} }
func (m *SignedEntryTimestamp) String() string            { return proto.CompactTextString(m) }
func (*SignedEntryTimestamp) ProtoMessage()               {}
func (*SignedEntryTimestamp) Descriptor() ([]byte, []int) { return fileDescriptor1, []int{1} }

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
	// TODO(al): define serialized format for the signature scheme.
	Signature    *DigitallySigned `protobuf:"bytes,4,opt,name=signature" json:"signature,omitempty"`
	LogId        int64            `protobuf:"varint,5,opt,name=log_id,json=logId" json:"log_id,omitempty"`
	TreeRevision int64            `protobuf:"varint,6,opt,name=tree_revision,json=treeRevision" json:"tree_revision,omitempty"`
}

func (m *SignedLogRoot) Reset()                    { *m = SignedLogRoot{} }
func (m *SignedLogRoot) String() string            { return proto.CompactTextString(m) }
func (*SignedLogRoot) ProtoMessage()               {}
func (*SignedLogRoot) Descriptor() ([]byte, []int) { return fileDescriptor1, []int{2} }

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

func (m *SignedLogRoot) GetSignature() *DigitallySigned {
	if m != nil {
		return m.Signature
	}
	return nil
}

func (m *SignedLogRoot) GetLogId() int64 {
	if m != nil {
		return m.LogId
	}
	return 0
}

func (m *SignedLogRoot) GetTreeRevision() int64 {
	if m != nil {
		return m.TreeRevision
	}
	return 0
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

func (m *MapperMetadata) GetSourceLogId() []byte {
	if m != nil {
		return m.SourceLogId
	}
	return nil
}

func (m *MapperMetadata) GetHighestFullyCompletedSeq() int64 {
	if m != nil {
		return m.HighestFullyCompletedSeq
	}
	return 0
}

func (m *MapperMetadata) GetHighestPartiallyCompletedSeq() int64 {
	if m != nil {
		return m.HighestPartiallyCompletedSeq
	}
	return 0
}

// SignedMapRoot represents a commitment by a Map to a particular tree.
type SignedMapRoot struct {
	TimestampNanos int64           `protobuf:"varint,1,opt,name=timestamp_nanos,json=timestampNanos" json:"timestamp_nanos,omitempty"`
	RootHash       []byte          `protobuf:"bytes,2,opt,name=root_hash,json=rootHash,proto3" json:"root_hash,omitempty"`
	Metadata       *MapperMetadata `protobuf:"bytes,3,opt,name=metadata" json:"metadata,omitempty"`
	// TODO(al): define serialized format for the signature scheme.
	Signature   *DigitallySigned `protobuf:"bytes,4,opt,name=signature" json:"signature,omitempty"`
	MapId       int64            `protobuf:"varint,5,opt,name=map_id,json=mapId" json:"map_id,omitempty"`
	MapRevision int64            `protobuf:"varint,6,opt,name=map_revision,json=mapRevision" json:"map_revision,omitempty"`
}

func (m *SignedMapRoot) Reset()                    { *m = SignedMapRoot{} }
func (m *SignedMapRoot) String() string            { return proto.CompactTextString(m) }
func (*SignedMapRoot) ProtoMessage()               {}
func (*SignedMapRoot) Descriptor() ([]byte, []int) { return fileDescriptor1, []int{4} }

func (m *SignedMapRoot) GetTimestampNanos() int64 {
	if m != nil {
		return m.TimestampNanos
	}
	return 0
}

func (m *SignedMapRoot) GetRootHash() []byte {
	if m != nil {
		return m.RootHash
	}
	return nil
}

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

func (m *SignedMapRoot) GetMapId() int64 {
	if m != nil {
		return m.MapId
	}
	return 0
}

func (m *SignedMapRoot) GetMapRevision() int64 {
	if m != nil {
		return m.MapRevision
	}
	return 0
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

func init() { proto.RegisterFile("trillian.proto", fileDescriptor1) }

var fileDescriptor1 = []byte{
	// 564 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x09, 0x6e, 0x88, 0x02, 0xff, 0xac, 0x54, 0xdf, 0x8e, 0xd2, 0x4e,
	0x18, 0xdd, 0xfe, 0x0a, 0xfc, 0xe0, 0x83, 0xb2, 0x75, 0xfc, 0x87, 0x59, 0x12, 0x57, 0x8c, 0x71,
	0xc3, 0xc5, 0x9a, 0xa0, 0xb2, 0xf1, 0x42, 0x93, 0x86, 0xed, 0xba, 0x24, 0x5b, 0x20, 0x53, 0xbc,
	0xf0, 0xaa, 0x19, 0xb7, 0x63, 0x3b, 0x49, 0xdb, 0xe9, 0x4e, 0x07, 0x13, 0xf6, 0x25, 0x7c, 0x1f,
	0x2f, 0x7c, 0x1d, 0xdf, 0xc2, 0x98, 0x96, 0x16, 0x0a, 0xdc, 0x6c, 0xa2, 0x97, 0xdf, 0x99, 0xc3,
	0x99, 0xf3, 0x9d, 0xc3, 0x14, 0xda, 0x52, 0xb0, 0x20, 0x60, 0x24, 0x3a, 0x8d, 0x05, 0x97, 0x1c,
	0xd5, 0x8b, 0xb9, 0xf7, 0x53, 0x81, 0xc3, 0x73, 0xe6, 0x31, 0x49, 0x82, 0x60, 0x69, 0x33, 0x2f,
	0xa2, 0x2e, 0xb2, 0xe0, 0x7e, 0xc2, 0xbc, 0x88, 0xc8, 0x85, 0xa0, 0x0e, 0x09, 0x3c, 0x2e, 0x98,
	0xf4, 0xc3, 0x8e, 0x72, 0xac, 0x9c, 0xb4, 0x07, 0xdd, 0xd3, 0xb5, 0x96, 0x5d, 0x90, 0x8c, 0x82,
	0x83, 0x51, 0xb2, 0x87, 0xa1, 0x0f, 0xd0, 0xf6, 0x49, 0xe2, 0x97, 0x94, 0xfe, 0xcb, 0x94, 0x1e,
	0x6f, 0x94, 0x2e, 0x49, 0xe2, 0x6f, 0x44, 0x34, 0xbf, 0x3c, 0xa2, 0x2e, 0x34, 0xd6, 0xaa, 0x1d,
	0xf5, 0x58, 0x39, 0x69, 0xe1, 0x0d, 0xd0, 0xfb, 0xae, 0xc0, 0x83, 0x95, 0x6f, 0x33, 0x92, 0x62,
	0x39, 0x67, 0x21, 0x4d, 0x24, 0x09, 0x63, 0xf4, 0x12, 0x0e, 0x65, 0x31, 0x38, 0x11, 0x89, 0x78,
	0x92, 0x6d, 0xa0, 0xe2, 0xf6, 0x1a, 0x9e, 0xa4, 0x28, 0x7a, 0x08, 0xb5, 0x80, 0x7b, 0x0e, 0x73,
	0x33, 0x5f, 0x2a, 0xae, 0x06, 0xdc, 0x1b, 0xbb, 0xe8, 0x6c, 0xf7, 0xda, 0xe6, 0xe0, 0xc9, 0xc6,
	0xf1, 0x4e, 0x66, 0x65, 0x47, 0xbf, 0x14, 0xd0, 0x56, 0xe8, 0x15, 0xf7, 0x30, 0xe7, 0xf2, 0xee,
	0x56, 0x8e, 0xa0, 0x21, 0x38, 0x97, 0x4e, 0x1a, 0x40, 0xe6, 0xa6, 0x85, 0xeb, 0x29, 0x90, 0xe6,
	0x93, 0x1e, 0x4a, 0x41, 0xa9, 0x93, 0xb0, 0xdb, 0x95, 0x21, 0x15, 0xd7, 0x53, 0xc0, 0x66, 0xb7,
	0x74, 0xdb, 0x6d, 0xe5, 0xee, 0x6e, 0x4b, 0xdb, 0x57, 0xcb, 0xdb, 0x3f, 0x07, 0x2d, 0xbb, 0x4c,
	0xd0, 0x6f, 0x2c, 0x61, 0x3c, 0xea, 0xd4, 0xb2, 0xd3, 0x56, 0x0a, 0xe2, 0x1c, 0xeb, 0xfd, 0x50,
	0xa0, 0x6d, 0x91, 0x38, 0xa6, 0xc2, 0xa2, 0x92, 0xb8, 0x44, 0x12, 0xd4, 0x03, 0x2d, 0xe1, 0x0b,
	0x71, 0x4d, 0x9d, 0x5c, 0x55, 0xc9, 0xb6, 0x68, 0xae, 0xc0, 0xab, 0x4c, 0xfb, 0x3d, 0x1c, 0xf9,
	0xcc, 0xf3, 0x69, 0x22, 0x9d, 0xaf, 0x8b, 0x20, 0x58, 0x3a, 0xd7, 0x3c, 0x8c, 0x03, 0x2a, 0xa9,
	0xeb, 0x24, 0xf4, 0x26, 0x6f, 0xa1, 0x93, 0x53, 0x2e, 0x52, 0xc6, 0xa8, 0x20, 0xd8, 0xf4, 0x06,
	0x99, 0xf0, 0xb4, 0xf8, 0x79, 0x4c, 0x84, 0x64, 0x64, 0x5f, 0x62, 0x95, 0x4e, 0x37, 0xa7, 0xcd,
	0x0a, 0x56, 0x59, 0xa6, 0xf7, 0x7b, 0x5d, 0x93, 0x45, 0xe2, 0x7f, 0x58, 0xd3, 0x1b, 0xa8, 0x87,
	0x79, 0x1a, 0xf9, 0xdf, 0xa6, 0xb3, 0x29, 0x62, 0x3b, 0x2d, 0xbc, 0x66, 0xfe, 0x55, 0x7f, 0x21,
	0x89, 0x4b, 0xfd, 0x85, 0x24, 0x1e, 0xbb, 0xe8, 0x19, 0xb4, 0x52, 0x78, 0xa7, 0xbe, 0x66, 0x48,
	0xe2, 0xa2, 0xbd, 0xfe, 0x2b, 0x78, 0x34, 0x17, 0x94, 0xa6, 0xa6, 0xa9, 0x98, 0x09, 0xca, 0x42,
	0xe2, 0xd1, 0xf9, 0x32, 0x4e, 0x35, 0xef, 0xe1, 0x8b, 0x91, 0x33, 0x7c, 0x37, 0x1c, 0x38, 0x33,
	0x6c, 0x8e, 0x2d, 0xe3, 0xa3, 0xa9, 0x1f, 0xf4, 0xcf, 0x00, 0xed, 0x3f, 0x79, 0xa4, 0x41, 0xc3,
	0x98, 0x4c, 0x27, 0x9f, 0xad, 0xe9, 0x27, 0x5b, 0x3f, 0x40, 0xff, 0x83, 0x8a, 0x6d, 0x43, 0x57,
	0x50, 0x03, 0xaa, 0xe6, 0xe8, 0xdc, 0x36, 0x74, 0xb5, 0xff, 0x02, 0xb4, 0xad, 0x17, 0x8e, 0xea,
	0x50, 0x99, 0x4c, 0x27, 0xa6, 0x7e, 0x80, 0x00, 0x6a, 0xf6, 0xa5, 0x31, 0x78, 0x3b, 0xd4, 0x2b,
	0x5f, 0x6a, 0xd9, 0xc7, 0xe9, 0xf5, 0x9f, 0x00, 0x00, 0x00, 0xff, 0xff, 0x34, 0x61, 0xcc, 0x1e,
	0xae, 0x04, 0x00, 0x00,
}
