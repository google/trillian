// Copyright 2017 Google LLC. All Rights Reserved.
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
// 	protoc-gen-go v1.36.0
// 	protoc        v3.20.1
// source: crypto/keyspb/keyspb.proto

package keyspb

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// The supported elliptic curves.
type Specification_ECDSA_Curve int32

const (
	Specification_ECDSA_DEFAULT_CURVE Specification_ECDSA_Curve = 0 // Curve will be chosen by Trillian.
	Specification_ECDSA_P256          Specification_ECDSA_Curve = 1
	Specification_ECDSA_P384          Specification_ECDSA_Curve = 2
	Specification_ECDSA_P521          Specification_ECDSA_Curve = 3
)

// Enum value maps for Specification_ECDSA_Curve.
var (
	Specification_ECDSA_Curve_name = map[int32]string{
		0: "DEFAULT_CURVE",
		1: "P256",
		2: "P384",
		3: "P521",
	}
	Specification_ECDSA_Curve_value = map[string]int32{
		"DEFAULT_CURVE": 0,
		"P256":          1,
		"P384":          2,
		"P521":          3,
	}
)

func (x Specification_ECDSA_Curve) Enum() *Specification_ECDSA_Curve {
	p := new(Specification_ECDSA_Curve)
	*p = x
	return p
}

func (x Specification_ECDSA_Curve) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (Specification_ECDSA_Curve) Descriptor() protoreflect.EnumDescriptor {
	return file_crypto_keyspb_keyspb_proto_enumTypes[0].Descriptor()
}

func (Specification_ECDSA_Curve) Type() protoreflect.EnumType {
	return &file_crypto_keyspb_keyspb_proto_enumTypes[0]
}

func (x Specification_ECDSA_Curve) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Specification_ECDSA_Curve.Descriptor instead.
func (Specification_ECDSA_Curve) EnumDescriptor() ([]byte, []int) {
	return file_crypto_keyspb_keyspb_proto_rawDescGZIP(), []int{0, 0, 0}
}

// Specification for a private key.
type Specification struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The type of parameters provided determines the algorithm used for the key.
	//
	// Types that are valid to be assigned to Params:
	//
	//	*Specification_EcdsaParams
	//	*Specification_RsaParams
	//	*Specification_Ed25519Params
	Params        isSpecification_Params `protobuf_oneof:"params"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Specification) Reset() {
	*x = Specification{}
	mi := &file_crypto_keyspb_keyspb_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Specification) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Specification) ProtoMessage() {}

func (x *Specification) ProtoReflect() protoreflect.Message {
	mi := &file_crypto_keyspb_keyspb_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Specification.ProtoReflect.Descriptor instead.
func (*Specification) Descriptor() ([]byte, []int) {
	return file_crypto_keyspb_keyspb_proto_rawDescGZIP(), []int{0}
}

func (x *Specification) GetParams() isSpecification_Params {
	if x != nil {
		return x.Params
	}
	return nil
}

func (x *Specification) GetEcdsaParams() *Specification_ECDSA {
	if x != nil {
		if x, ok := x.Params.(*Specification_EcdsaParams); ok {
			return x.EcdsaParams
		}
	}
	return nil
}

func (x *Specification) GetRsaParams() *Specification_RSA {
	if x != nil {
		if x, ok := x.Params.(*Specification_RsaParams); ok {
			return x.RsaParams
		}
	}
	return nil
}

func (x *Specification) GetEd25519Params() *Specification_Ed25519 {
	if x != nil {
		if x, ok := x.Params.(*Specification_Ed25519Params); ok {
			return x.Ed25519Params
		}
	}
	return nil
}

type isSpecification_Params interface {
	isSpecification_Params()
}

type Specification_EcdsaParams struct {
	// The parameters for an ECDSA key.
	EcdsaParams *Specification_ECDSA `protobuf:"bytes,1,opt,name=ecdsa_params,json=ecdsaParams,proto3,oneof"`
}

type Specification_RsaParams struct {
	// The parameters for an RSA key.
	RsaParams *Specification_RSA `protobuf:"bytes,2,opt,name=rsa_params,json=rsaParams,proto3,oneof"`
}

type Specification_Ed25519Params struct {
	// The parameters for an Ed25519 key.
	Ed25519Params *Specification_Ed25519 `protobuf:"bytes,3,opt,name=ed25519_params,json=ed25519Params,proto3,oneof"`
}

func (*Specification_EcdsaParams) isSpecification_Params() {}

func (*Specification_RsaParams) isSpecification_Params() {}

func (*Specification_Ed25519Params) isSpecification_Params() {}

// PEMKeyFile identifies a private key stored in a PEM-encoded file.
type PEMKeyFile struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// File path of the private key.
	Path string `protobuf:"bytes,1,opt,name=path,proto3" json:"path,omitempty"`
	// Password for decrypting the private key.
	// If empty, indicates that the private key is not encrypted.
	Password      string `protobuf:"bytes,2,opt,name=password,proto3" json:"password,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *PEMKeyFile) Reset() {
	*x = PEMKeyFile{}
	mi := &file_crypto_keyspb_keyspb_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *PEMKeyFile) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PEMKeyFile) ProtoMessage() {}

func (x *PEMKeyFile) ProtoReflect() protoreflect.Message {
	mi := &file_crypto_keyspb_keyspb_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PEMKeyFile.ProtoReflect.Descriptor instead.
func (*PEMKeyFile) Descriptor() ([]byte, []int) {
	return file_crypto_keyspb_keyspb_proto_rawDescGZIP(), []int{1}
}

func (x *PEMKeyFile) GetPath() string {
	if x != nil {
		return x.Path
	}
	return ""
}

func (x *PEMKeyFile) GetPassword() string {
	if x != nil {
		return x.Password
	}
	return ""
}

// PrivateKey is a private key, used for generating signatures.
type PrivateKey struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The key in DER-encoded form.
	// The specific format (e.g. PKCS8) is not specified.
	Der           []byte `protobuf:"bytes,1,opt,name=der,proto3" json:"der,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *PrivateKey) Reset() {
	*x = PrivateKey{}
	mi := &file_crypto_keyspb_keyspb_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *PrivateKey) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PrivateKey) ProtoMessage() {}

func (x *PrivateKey) ProtoReflect() protoreflect.Message {
	mi := &file_crypto_keyspb_keyspb_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PrivateKey.ProtoReflect.Descriptor instead.
func (*PrivateKey) Descriptor() ([]byte, []int) {
	return file_crypto_keyspb_keyspb_proto_rawDescGZIP(), []int{2}
}

func (x *PrivateKey) GetDer() []byte {
	if x != nil {
		return x.Der
	}
	return nil
}

// PublicKey is a public key, used for verifying signatures.
type PublicKey struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The key in DER-encoded PKIX form.
	Der           []byte `protobuf:"bytes,1,opt,name=der,proto3" json:"der,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *PublicKey) Reset() {
	*x = PublicKey{}
	mi := &file_crypto_keyspb_keyspb_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *PublicKey) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PublicKey) ProtoMessage() {}

func (x *PublicKey) ProtoReflect() protoreflect.Message {
	mi := &file_crypto_keyspb_keyspb_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PublicKey.ProtoReflect.Descriptor instead.
func (*PublicKey) Descriptor() ([]byte, []int) {
	return file_crypto_keyspb_keyspb_proto_rawDescGZIP(), []int{3}
}

func (x *PublicKey) GetDer() []byte {
	if x != nil {
		return x.Der
	}
	return nil
}

// PKCS11Config identifies a private key accessed using PKCS #11.
type PKCS11Config struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The label of the PKCS#11 token.
	TokenLabel string `protobuf:"bytes,1,opt,name=token_label,json=tokenLabel,proto3" json:"token_label,omitempty"`
	// The PIN for the specific token.
	Pin string `protobuf:"bytes,2,opt,name=pin,proto3" json:"pin,omitempty"`
	// The PEM public key associated with the private key to be used.
	PublicKey     string `protobuf:"bytes,3,opt,name=public_key,json=publicKey,proto3" json:"public_key,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *PKCS11Config) Reset() {
	*x = PKCS11Config{}
	mi := &file_crypto_keyspb_keyspb_proto_msgTypes[4]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *PKCS11Config) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PKCS11Config) ProtoMessage() {}

func (x *PKCS11Config) ProtoReflect() protoreflect.Message {
	mi := &file_crypto_keyspb_keyspb_proto_msgTypes[4]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PKCS11Config.ProtoReflect.Descriptor instead.
func (*PKCS11Config) Descriptor() ([]byte, []int) {
	return file_crypto_keyspb_keyspb_proto_rawDescGZIP(), []int{4}
}

func (x *PKCS11Config) GetTokenLabel() string {
	if x != nil {
		return x.TokenLabel
	}
	return ""
}

func (x *PKCS11Config) GetPin() string {
	if x != nil {
		return x.Pin
	}
	return ""
}

func (x *PKCS11Config) GetPublicKey() string {
	if x != nil {
		return x.PublicKey
	}
	return ""
}

// / ECDSA defines parameters for an ECDSA key.
type Specification_ECDSA struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The elliptic curve to use.
	// Optional. If not set, the default curve will be used.
	Curve         Specification_ECDSA_Curve `protobuf:"varint,1,opt,name=curve,proto3,enum=keyspb.Specification_ECDSA_Curve" json:"curve,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Specification_ECDSA) Reset() {
	*x = Specification_ECDSA{}
	mi := &file_crypto_keyspb_keyspb_proto_msgTypes[5]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Specification_ECDSA) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Specification_ECDSA) ProtoMessage() {}

func (x *Specification_ECDSA) ProtoReflect() protoreflect.Message {
	mi := &file_crypto_keyspb_keyspb_proto_msgTypes[5]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Specification_ECDSA.ProtoReflect.Descriptor instead.
func (*Specification_ECDSA) Descriptor() ([]byte, []int) {
	return file_crypto_keyspb_keyspb_proto_rawDescGZIP(), []int{0, 0}
}

func (x *Specification_ECDSA) GetCurve() Specification_ECDSA_Curve {
	if x != nil {
		return x.Curve
	}
	return Specification_ECDSA_DEFAULT_CURVE
}

// RSA defines parameters for an RSA key.
type Specification_RSA struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Size of the keys in bits. Must be sufficiently large to allow two primes
	// to be generated.
	// Optional. If not set, the key size will be chosen by Trillian.
	Bits          int32 `protobuf:"varint,1,opt,name=bits,proto3" json:"bits,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Specification_RSA) Reset() {
	*x = Specification_RSA{}
	mi := &file_crypto_keyspb_keyspb_proto_msgTypes[6]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Specification_RSA) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Specification_RSA) ProtoMessage() {}

func (x *Specification_RSA) ProtoReflect() protoreflect.Message {
	mi := &file_crypto_keyspb_keyspb_proto_msgTypes[6]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Specification_RSA.ProtoReflect.Descriptor instead.
func (*Specification_RSA) Descriptor() ([]byte, []int) {
	return file_crypto_keyspb_keyspb_proto_rawDescGZIP(), []int{0, 1}
}

func (x *Specification_RSA) GetBits() int32 {
	if x != nil {
		return x.Bits
	}
	return 0
}

// Ed25519 defines (empty) parameters for an Ed25519 private key.
type Specification_Ed25519 struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Specification_Ed25519) Reset() {
	*x = Specification_Ed25519{}
	mi := &file_crypto_keyspb_keyspb_proto_msgTypes[7]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Specification_Ed25519) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Specification_Ed25519) ProtoMessage() {}

func (x *Specification_Ed25519) ProtoReflect() protoreflect.Message {
	mi := &file_crypto_keyspb_keyspb_proto_msgTypes[7]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Specification_Ed25519.ProtoReflect.Descriptor instead.
func (*Specification_Ed25519) Descriptor() ([]byte, []int) {
	return file_crypto_keyspb_keyspb_proto_rawDescGZIP(), []int{0, 2}
}

var File_crypto_keyspb_keyspb_proto protoreflect.FileDescriptor

var file_crypto_keyspb_keyspb_proto_rawDesc = []byte{
	0x0a, 0x1a, 0x63, 0x72, 0x79, 0x70, 0x74, 0x6f, 0x2f, 0x6b, 0x65, 0x79, 0x73, 0x70, 0x62, 0x2f,
	0x6b, 0x65, 0x79, 0x73, 0x70, 0x62, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x06, 0x6b, 0x65,
	0x79, 0x73, 0x70, 0x62, 0x22, 0x81, 0x03, 0x0a, 0x0d, 0x53, 0x70, 0x65, 0x63, 0x69, 0x66, 0x69,
	0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x40, 0x0a, 0x0c, 0x65, 0x63, 0x64, 0x73, 0x61, 0x5f,
	0x70, 0x61, 0x72, 0x61, 0x6d, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1b, 0x2e, 0x6b,
	0x65, 0x79, 0x73, 0x70, 0x62, 0x2e, 0x53, 0x70, 0x65, 0x63, 0x69, 0x66, 0x69, 0x63, 0x61, 0x74,
	0x69, 0x6f, 0x6e, 0x2e, 0x45, 0x43, 0x44, 0x53, 0x41, 0x48, 0x00, 0x52, 0x0b, 0x65, 0x63, 0x64,
	0x73, 0x61, 0x50, 0x61, 0x72, 0x61, 0x6d, 0x73, 0x12, 0x3a, 0x0a, 0x0a, 0x72, 0x73, 0x61, 0x5f,
	0x70, 0x61, 0x72, 0x61, 0x6d, 0x73, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x6b,
	0x65, 0x79, 0x73, 0x70, 0x62, 0x2e, 0x53, 0x70, 0x65, 0x63, 0x69, 0x66, 0x69, 0x63, 0x61, 0x74,
	0x69, 0x6f, 0x6e, 0x2e, 0x52, 0x53, 0x41, 0x48, 0x00, 0x52, 0x09, 0x72, 0x73, 0x61, 0x50, 0x61,
	0x72, 0x61, 0x6d, 0x73, 0x12, 0x46, 0x0a, 0x0e, 0x65, 0x64, 0x32, 0x35, 0x35, 0x31, 0x39, 0x5f,
	0x70, 0x61, 0x72, 0x61, 0x6d, 0x73, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1d, 0x2e, 0x6b,
	0x65, 0x79, 0x73, 0x70, 0x62, 0x2e, 0x53, 0x70, 0x65, 0x63, 0x69, 0x66, 0x69, 0x63, 0x61, 0x74,
	0x69, 0x6f, 0x6e, 0x2e, 0x45, 0x64, 0x32, 0x35, 0x35, 0x31, 0x39, 0x48, 0x00, 0x52, 0x0d, 0x65,
	0x64, 0x32, 0x35, 0x35, 0x31, 0x39, 0x50, 0x61, 0x72, 0x61, 0x6d, 0x73, 0x1a, 0x7a, 0x0a, 0x05,
	0x45, 0x43, 0x44, 0x53, 0x41, 0x12, 0x37, 0x0a, 0x05, 0x63, 0x75, 0x72, 0x76, 0x65, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x0e, 0x32, 0x21, 0x2e, 0x6b, 0x65, 0x79, 0x73, 0x70, 0x62, 0x2e, 0x53, 0x70,
	0x65, 0x63, 0x69, 0x66, 0x69, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x2e, 0x45, 0x43, 0x44, 0x53,
	0x41, 0x2e, 0x43, 0x75, 0x72, 0x76, 0x65, 0x52, 0x05, 0x63, 0x75, 0x72, 0x76, 0x65, 0x22, 0x38,
	0x0a, 0x05, 0x43, 0x75, 0x72, 0x76, 0x65, 0x12, 0x11, 0x0a, 0x0d, 0x44, 0x45, 0x46, 0x41, 0x55,
	0x4c, 0x54, 0x5f, 0x43, 0x55, 0x52, 0x56, 0x45, 0x10, 0x00, 0x12, 0x08, 0x0a, 0x04, 0x50, 0x32,
	0x35, 0x36, 0x10, 0x01, 0x12, 0x08, 0x0a, 0x04, 0x50, 0x33, 0x38, 0x34, 0x10, 0x02, 0x12, 0x08,
	0x0a, 0x04, 0x50, 0x35, 0x32, 0x31, 0x10, 0x03, 0x1a, 0x19, 0x0a, 0x03, 0x52, 0x53, 0x41, 0x12,
	0x12, 0x0a, 0x04, 0x62, 0x69, 0x74, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28, 0x05, 0x52, 0x04, 0x62,
	0x69, 0x74, 0x73, 0x1a, 0x09, 0x0a, 0x07, 0x45, 0x64, 0x32, 0x35, 0x35, 0x31, 0x39, 0x42, 0x08,
	0x0a, 0x06, 0x70, 0x61, 0x72, 0x61, 0x6d, 0x73, 0x22, 0x3c, 0x0a, 0x0a, 0x50, 0x45, 0x4d, 0x4b,
	0x65, 0x79, 0x46, 0x69, 0x6c, 0x65, 0x12, 0x12, 0x0a, 0x04, 0x70, 0x61, 0x74, 0x68, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x70, 0x61, 0x74, 0x68, 0x12, 0x1a, 0x0a, 0x08, 0x70, 0x61,
	0x73, 0x73, 0x77, 0x6f, 0x72, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x70, 0x61,
	0x73, 0x73, 0x77, 0x6f, 0x72, 0x64, 0x22, 0x1e, 0x0a, 0x0a, 0x50, 0x72, 0x69, 0x76, 0x61, 0x74,
	0x65, 0x4b, 0x65, 0x79, 0x12, 0x10, 0x0a, 0x03, 0x64, 0x65, 0x72, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x0c, 0x52, 0x03, 0x64, 0x65, 0x72, 0x22, 0x1d, 0x0a, 0x09, 0x50, 0x75, 0x62, 0x6c, 0x69, 0x63,
	0x4b, 0x65, 0x79, 0x12, 0x10, 0x0a, 0x03, 0x64, 0x65, 0x72, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0c,
	0x52, 0x03, 0x64, 0x65, 0x72, 0x22, 0x60, 0x0a, 0x0c, 0x50, 0x4b, 0x43, 0x53, 0x31, 0x31, 0x43,
	0x6f, 0x6e, 0x66, 0x69, 0x67, 0x12, 0x1f, 0x0a, 0x0b, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x5f, 0x6c,
	0x61, 0x62, 0x65, 0x6c, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x74, 0x6f, 0x6b, 0x65,
	0x6e, 0x4c, 0x61, 0x62, 0x65, 0x6c, 0x12, 0x10, 0x0a, 0x03, 0x70, 0x69, 0x6e, 0x18, 0x02, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x03, 0x70, 0x69, 0x6e, 0x12, 0x1d, 0x0a, 0x0a, 0x70, 0x75, 0x62, 0x6c,
	0x69, 0x63, 0x5f, 0x6b, 0x65, 0x79, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x70, 0x75,
	0x62, 0x6c, 0x69, 0x63, 0x4b, 0x65, 0x79, 0x42, 0x2a, 0x5a, 0x28, 0x67, 0x69, 0x74, 0x68, 0x75,
	0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x74, 0x72, 0x69,
	0x6c, 0x6c, 0x69, 0x61, 0x6e, 0x2f, 0x63, 0x72, 0x79, 0x70, 0x74, 0x6f, 0x2f, 0x6b, 0x65, 0x79,
	0x73, 0x70, 0x62, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_crypto_keyspb_keyspb_proto_rawDescOnce sync.Once
	file_crypto_keyspb_keyspb_proto_rawDescData = file_crypto_keyspb_keyspb_proto_rawDesc
)

func file_crypto_keyspb_keyspb_proto_rawDescGZIP() []byte {
	file_crypto_keyspb_keyspb_proto_rawDescOnce.Do(func() {
		file_crypto_keyspb_keyspb_proto_rawDescData = protoimpl.X.CompressGZIP(file_crypto_keyspb_keyspb_proto_rawDescData)
	})
	return file_crypto_keyspb_keyspb_proto_rawDescData
}

var file_crypto_keyspb_keyspb_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_crypto_keyspb_keyspb_proto_msgTypes = make([]protoimpl.MessageInfo, 8)
var file_crypto_keyspb_keyspb_proto_goTypes = []any{
	(Specification_ECDSA_Curve)(0), // 0: keyspb.Specification.ECDSA.Curve
	(*Specification)(nil),          // 1: keyspb.Specification
	(*PEMKeyFile)(nil),             // 2: keyspb.PEMKeyFile
	(*PrivateKey)(nil),             // 3: keyspb.PrivateKey
	(*PublicKey)(nil),              // 4: keyspb.PublicKey
	(*PKCS11Config)(nil),           // 5: keyspb.PKCS11Config
	(*Specification_ECDSA)(nil),    // 6: keyspb.Specification.ECDSA
	(*Specification_RSA)(nil),      // 7: keyspb.Specification.RSA
	(*Specification_Ed25519)(nil),  // 8: keyspb.Specification.Ed25519
}
var file_crypto_keyspb_keyspb_proto_depIdxs = []int32{
	6, // 0: keyspb.Specification.ecdsa_params:type_name -> keyspb.Specification.ECDSA
	7, // 1: keyspb.Specification.rsa_params:type_name -> keyspb.Specification.RSA
	8, // 2: keyspb.Specification.ed25519_params:type_name -> keyspb.Specification.Ed25519
	0, // 3: keyspb.Specification.ECDSA.curve:type_name -> keyspb.Specification.ECDSA.Curve
	4, // [4:4] is the sub-list for method output_type
	4, // [4:4] is the sub-list for method input_type
	4, // [4:4] is the sub-list for extension type_name
	4, // [4:4] is the sub-list for extension extendee
	0, // [0:4] is the sub-list for field type_name
}

func init() { file_crypto_keyspb_keyspb_proto_init() }
func file_crypto_keyspb_keyspb_proto_init() {
	if File_crypto_keyspb_keyspb_proto != nil {
		return
	}
	file_crypto_keyspb_keyspb_proto_msgTypes[0].OneofWrappers = []any{
		(*Specification_EcdsaParams)(nil),
		(*Specification_RsaParams)(nil),
		(*Specification_Ed25519Params)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_crypto_keyspb_keyspb_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   8,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_crypto_keyspb_keyspb_proto_goTypes,
		DependencyIndexes: file_crypto_keyspb_keyspb_proto_depIdxs,
		EnumInfos:         file_crypto_keyspb_keyspb_proto_enumTypes,
		MessageInfos:      file_crypto_keyspb_keyspb_proto_msgTypes,
	}.Build()
	File_crypto_keyspb_keyspb_proto = out.File
	file_crypto_keyspb_keyspb_proto_rawDesc = nil
	file_crypto_keyspb_keyspb_proto_goTypes = nil
	file_crypto_keyspb_keyspb_proto_depIdxs = nil
}
