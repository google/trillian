// Code generated by protoc-gen-go.
// source: github.com/google/trillian/trillian.proto
// DO NOT EDIT!

/*
Package trillian is a generated protocol buffer package.

It is generated from these files:
	github.com/google/trillian/trillian.proto
	github.com/google/trillian/trillian_api.proto

It has these top-level messages:
	DigitallySigned
	SignedEntryTimestamp
	SignedLogRoot
	MapperMetadata
	SignedMapRoot
	TrillianApiStatus
	LogLeaf
	Node
	Proof
	QueueLeavesRequest
	QueueLeavesResponse
	GetInclusionProofRequest
	GetInclusionProofResponse
	GetInclusionProofByHashRequest
	GetInclusionProofByHashResponse
	GetConsistencyProofRequest
	GetConsistencyProofResponse
	GetLeavesByHashRequest
	GetLeavesByHashResponse
	GetLeavesByIndexRequest
	GetLeavesByIndexResponse
	GetSequencedLeafCountRequest
	GetSequencedLeafCountResponse
	GetLatestSignedLogRootRequest
	GetLatestSignedLogRootResponse
	GetEntryAndProofRequest
	GetEntryAndProofResponse
	MapLeaf
	KeyValue
	KeyValueInclusion
	GetMapLeavesRequest
	GetMapLeavesResponse
	SetMapLeavesRequest
	SetMapLeavesResponse
	GetSignedMapRootRequest
	GetSignedMapRootResponse
*/
package trillian

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
func (TreeHasherPreimageType) EnumDescriptor() ([]byte, []int) { return fileDescriptor0, []int{0} }

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
func (SignatureAlgorithm) EnumDescriptor() ([]byte, []int) { return fileDescriptor0, []int{1} }

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
func (HashAlgorithm) EnumDescriptor() ([]byte, []int) { return fileDescriptor0, []int{2} }

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
func (*DigitallySigned) Descriptor() ([]byte, []int) { return fileDescriptor0, []int{0} }

type SignedEntryTimestamp struct {
	TimestampNanos int64            `protobuf:"varint,1,opt,name=timestamp_nanos,json=timestampNanos" json:"timestamp_nanos,omitempty"`
	LogId          int64            `protobuf:"varint,2,opt,name=log_id,json=logId" json:"log_id,omitempty"`
	Signature      *DigitallySigned `protobuf:"bytes,3,opt,name=signature" json:"signature,omitempty"`
}

func (m *SignedEntryTimestamp) Reset()                    { *m = SignedEntryTimestamp{} }
func (m *SignedEntryTimestamp) String() string            { return proto.CompactTextString(m) }
func (*SignedEntryTimestamp) ProtoMessage()               {}
func (*SignedEntryTimestamp) Descriptor() ([]byte, []int) { return fileDescriptor0, []int{1} }

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
func (*SignedLogRoot) Descriptor() ([]byte, []int) { return fileDescriptor0, []int{2} }

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
func (*MapperMetadata) Descriptor() ([]byte, []int) { return fileDescriptor0, []int{3} }

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
func (*SignedMapRoot) Descriptor() ([]byte, []int) { return fileDescriptor0, []int{4} }

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

func init() { proto.RegisterFile("github.com/google/trillian/trillian.proto", fileDescriptor0) }

var fileDescriptor0 = []byte{
	// 592 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x09, 0x6e, 0x88, 0x02, 0xff, 0xac, 0x54, 0xdd, 0x4e, 0xd4, 0x40,
	0x18, 0xa5, 0x16, 0xd6, 0xdd, 0x8f, 0xed, 0x52, 0xc7, 0xbf, 0x35, 0x90, 0x88, 0x4b, 0x88, 0xc8,
	0x05, 0x24, 0xab, 0x42, 0x8c, 0xd1, 0xa4, 0x42, 0x11, 0x12, 0xba, 0x90, 0x29, 0x5e, 0xe8, 0x4d,
	0x33, 0xd0, 0x71, 0x3a, 0x49, 0xdb, 0x29, 0xd3, 0x59, 0x93, 0xe5, 0x25, 0x7c, 0x1f, 0x2f, 0x7c,
	0x1d, 0xdf, 0xc2, 0x98, 0x76, 0xdb, 0x6e, 0x77, 0x37, 0x26, 0x24, 0x7a, 0x37, 0xdf, 0x99, 0xd3,
	0x33, 0xe7, 0xfb, 0x4e, 0x67, 0xe0, 0x05, 0xe3, 0x2a, 0x18, 0x5e, 0xee, 0x5c, 0x89, 0x68, 0x97,
	0x09, 0xc1, 0x42, 0xba, 0xab, 0x24, 0x0f, 0x43, 0x4e, 0xe2, 0x6a, 0xb1, 0x93, 0x48, 0xa1, 0x04,
	0x6a, 0x96, 0x75, 0xef, 0xa7, 0x06, 0x2b, 0x87, 0x9c, 0x71, 0x45, 0xc2, 0x70, 0xe4, 0x72, 0x16,
	0x53, 0x1f, 0x39, 0x70, 0x3f, 0xe5, 0x2c, 0x26, 0x6a, 0x28, 0xa9, 0x47, 0x42, 0x26, 0x24, 0x57,
	0x41, 0xd4, 0xd5, 0xd6, 0xb5, 0xad, 0x4e, 0x7f, 0x6d, 0xa7, 0xd2, 0x72, 0x4b, 0x92, 0x55, 0x72,
	0x30, 0x4a, 0xe7, 0x30, 0xf4, 0x1e, 0x3a, 0x01, 0x49, 0x83, 0x9a, 0xd2, 0x9d, 0x5c, 0xe9, 0xf1,
	0x44, 0xe9, 0x98, 0xa4, 0xc1, 0x44, 0xc4, 0x08, 0xea, 0x25, 0x5a, 0x83, 0x56, 0xa5, 0xda, 0xd5,
	0xd7, 0xb5, 0xad, 0x36, 0x9e, 0x00, 0xbd, 0xef, 0x1a, 0x3c, 0x18, 0xfb, 0xb6, 0x63, 0x25, 0x47,
	0x17, 0x3c, 0xa2, 0xa9, 0x22, 0x51, 0x82, 0x9e, 0xc3, 0x8a, 0x2a, 0x0b, 0x2f, 0x26, 0xb1, 0x48,
	0xf3, 0x0e, 0x74, 0xdc, 0xa9, 0xe0, 0x41, 0x86, 0xa2, 0x87, 0xd0, 0x08, 0x05, 0xf3, 0xb8, 0x9f,
	0xfb, 0xd2, 0xf1, 0x52, 0x28, 0xd8, 0x89, 0x8f, 0xf6, 0x67, 0x8f, 0x5d, 0xee, 0x3f, 0x99, 0x38,
	0x9e, 0x99, 0x59, 0xdd, 0xd1, 0x2f, 0x0d, 0x8c, 0x31, 0x7a, 0x2a, 0x18, 0x16, 0x42, 0xdd, 0xde,
	0xca, 0x2a, 0xb4, 0xa4, 0x10, 0xca, 0xcb, 0x06, 0x90, 0xbb, 0x69, 0xe3, 0x66, 0x06, 0x64, 0xf3,
	0xc9, 0x36, 0x95, 0xa4, 0xd4, 0x4b, 0xf9, 0xcd, 0xd8, 0x90, 0x8e, 0x9b, 0x19, 0xe0, 0xf2, 0x1b,
	0x3a, 0xed, 0x76, 0xf1, 0xf6, 0x6e, 0x6b, 0xdd, 0x2f, 0xd5, 0xbb, 0xdf, 0x00, 0x23, 0x3f, 0x4c,
	0xd2, 0x6f, 0x3c, 0xe5, 0x22, 0xee, 0x36, 0xf2, 0xdd, 0x76, 0x06, 0xe2, 0x02, 0xeb, 0xfd, 0xd0,
	0xa0, 0xe3, 0x90, 0x24, 0xa1, 0xd2, 0xa1, 0x8a, 0xf8, 0x44, 0x11, 0xd4, 0x03, 0x23, 0x15, 0x43,
	0x79, 0x45, 0xbd, 0x42, 0x55, 0xcb, 0xbb, 0x58, 0x1e, 0x83, 0xa7, 0xb9, 0xf6, 0x3b, 0x58, 0x0d,
	0x38, 0x0b, 0x68, 0xaa, 0xbc, 0xaf, 0xc3, 0x30, 0x1c, 0x79, 0x57, 0x22, 0x4a, 0x42, 0xaa, 0xa8,
	0xef, 0xa5, 0xf4, 0xba, 0x48, 0xa1, 0x5b, 0x50, 0x8e, 0x32, 0xc6, 0x41, 0x49, 0x70, 0xe9, 0x35,
	0xb2, 0xe1, 0x69, 0xf9, 0x79, 0x42, 0xa4, 0xe2, 0x64, 0x5e, 0x62, 0x3c, 0x9d, 0xb5, 0x82, 0x76,
	0x5e, 0xb2, 0xea, 0x32, 0xbd, 0xdf, 0x55, 0x4c, 0x0e, 0x49, 0xfe, 0x63, 0x4c, 0xaf, 0xa0, 0x19,
	0x15, 0xd3, 0x28, 0x7e, 0x9b, 0xee, 0x24, 0x88, 0xe9, 0x69, 0xe1, 0x8a, 0xf9, 0x4f, 0xf9, 0x45,
	0x24, 0xa9, 0xe5, 0x17, 0x91, 0xe4, 0xc4, 0x47, 0xcf, 0xa0, 0x9d, 0xc1, 0x33, 0xf1, 0x2d, 0x47,
	0x24, 0x29, 0xd3, 0xdb, 0xde, 0x85, 0x47, 0x17, 0x92, 0xd2, 0xcc, 0x34, 0x95, 0xe7, 0x92, 0xf2,
	0x88, 0x30, 0x7a, 0x31, 0x4a, 0x32, 0xcd, 0x7b, 0xf8, 0xe8, 0xc0, 0xdb, 0x7b, 0xb3, 0xd7, 0xf7,
	0xce, 0xb1, 0x7d, 0xe2, 0x58, 0x1f, 0x6d, 0x73, 0x61, 0x7b, 0x1f, 0xd0, 0xfc, 0x95, 0x47, 0x06,
	0xb4, 0xac, 0xc1, 0xd9, 0xe0, 0xb3, 0x73, 0xf6, 0xc9, 0x35, 0x17, 0xd0, 0x5d, 0xd0, 0xb1, 0x6b,
	0x99, 0x1a, 0x6a, 0xc1, 0x92, 0x7d, 0x70, 0xe8, 0x5a, 0xa6, 0xbe, 0xbd, 0x09, 0xc6, 0xd4, 0x0d,
	0x47, 0x4d, 0x58, 0x1c, 0x9c, 0x0d, 0x6c, 0x73, 0x01, 0x01, 0x34, 0xdc, 0x63, 0xab, 0xff, 0x7a,
	0xcf, 0x5c, 0xfc, 0xb0, 0xf9, 0x65, 0xe3, 0xef, 0x4f, 0xd8, 0xdb, 0x72, 0x71, 0xd9, 0xc8, 0xdf,
	0xb0, 0x97, 0x7f, 0x02, 0x00, 0x00, 0xff, 0xff, 0x77, 0xe6, 0x6f, 0xf0, 0xf0, 0x04, 0x00, 0x00,
}
