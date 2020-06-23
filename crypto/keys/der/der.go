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

package der

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"fmt"

	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/crypto/keyspb"
	"golang.org/x/crypto/ed25519"
)

// FromProto takes a PrivateKey protobuf message and returns the private key contained within.
func FromProto(pb *keyspb.PrivateKey) (crypto.Signer, error) {
	return UnmarshalPrivateKey(pb.GetDer())
}

// NewProtoFromSpec creates a new private key based on a key specification.
// It returns a PrivateKey protobuf message that contains the private key.
func NewProtoFromSpec(spec *keyspb.Specification) (*keyspb.PrivateKey, error) {
	key, err := keys.NewFromSpec(spec)
	if err != nil {
		return nil, fmt.Errorf("der: error generating key: %v", err)
	}

	der, err := MarshalPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("der: error marshaling private key: %v", err)
	}

	return &keyspb.PrivateKey{Der: der}, nil
}

// FromPublicProto takes a PublicKey protobuf message and returns the public
// key contained within.
func FromPublicProto(pb *keyspb.PublicKey) (crypto.PublicKey, error) {
	return UnmarshalPublicKey(pb.GetDer())
}

// ToPublicProto returns a keyspb.PublicKey that contains pubKey in DER encoding.
func ToPublicProto(pubKey crypto.PublicKey) (*keyspb.PublicKey, error) {
	keyDER, err := MarshalPublicKey(pubKey)
	if err != nil {
		return nil, err
	}
	return &keyspb.PublicKey{Der: keyDER}, nil
}

// UnmarshalPrivateKey reads a DER-encoded private key.
func UnmarshalPrivateKey(keyDER []byte) (crypto.Signer, error) {
	key1, err1 := x509.ParseECPrivateKey(keyDER)
	if err1 == nil {
		return key1, nil
	}

	key2, err2 := x509.ParsePKCS8PrivateKey(keyDER)
	if err2 == nil {
		switch key2 := key2.(type) {
		case *ecdsa.PrivateKey:
			return key2, nil
		case *rsa.PrivateKey:
			return key2, nil
		case ed25519.PrivateKey:
			return key2, nil
		}
		return nil, fmt.Errorf("der: unsupported private key type: %T", key2)
	}

	key3, err3 := x509.ParsePKCS1PrivateKey(keyDER)
	if err3 == nil {
		return key3, nil
	}

	return nil, fmt.Errorf("der: could not parse private key as SEC1 (%v), PKCS8 (%v) or PKCS1 (%v)", err1, err2, err3)
}

// UnmarshalPublicKey reads a DER-encoded public key.
func UnmarshalPublicKey(keyDER []byte) (crypto.PublicKey, error) {
	key, err := x509.ParsePKIXPublicKey(keyDER)
	if err != nil {
		return nil, fmt.Errorf("der: could not parse public key as PKIX (%v)", err)
	}

	return key, nil
}

// MarshalPublicKey serializes an RSA or ECDSA public key as DER.
func MarshalPublicKey(pubKey crypto.PublicKey) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		return nil, fmt.Errorf("der: could not marshal public key as PKIX (%v)", err)
	}

	return der, nil
}

// MarshalPrivateKey serializes an RSA or ECDSA private key as DER.
func MarshalPrivateKey(key crypto.Signer) ([]byte, error) {
	switch key := key.(type) {
	case *ecdsa.PrivateKey:
		return x509.MarshalECPrivateKey(key)
	case *rsa.PrivateKey:
		return x509.MarshalPKCS1PrivateKey(key), nil
	case ed25519.PrivateKey:
		return x509.MarshalPKCS8PrivateKey(key)
	}

	return nil, fmt.Errorf("der: unsupported key type: %T", key)
}
