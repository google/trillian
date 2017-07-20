// Copyright 2017 Google Inc. All Rights Reserved.
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

package crypto

import (
	"bytes"
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/testonly"

	"github.com/golang/protobuf/proto"
	"github.com/kylelemons/godebug/pretty"
)

// toFromBytes serializes obj using proto.Marshal, and returns the unmarshaled copy.
func toFromBytes(obj proto.Message) (proto.Message, error) {
	b, err := proto.Marshal(obj)
	if err != nil {
		return nil, err
	}

	obj2 := obj
	obj2.Reset()
	if err := proto.Unmarshal(b, obj2); err != nil {
		return nil, err
	}
	return obj2, nil
}

// toFromJSON serializes obj using protopb.Marshal, and returns the unmarshaled copy.
func toFromJSON(obj proto.Message) (proto.Message, error) {
	// Serialize and deserialize to json
	b := new(bytes.Buffer)
	if err := marshaler.Marshal(b, obj); err != nil {
		return nil, err
	}

	obj2 := obj
	obj2.Reset()
	if err := unmarshaler.Unmarshal(bytes.NewBuffer(b.Bytes()), obj2); err != nil {
		return nil, err
	}
	return obj2, nil
}

func TestSignVerifyProto(t *testing.T) {
	key, err := keys.NewFromPrivatePEM(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass)
	if err != nil {
		t.Fatalf("Failed to open test key, err=%v", err)
	}
	signer := NewSHA256Signer(key)

	for _, tc := range []struct {
		obj proto.Message
	}{
		{&trillian.MapperMetadata{}},
		{&trillian.MapperMetadata{HighestFullyCompletedSeq: 0}},
		{&trillian.MapperMetadata{HighestFullyCompletedSeq: 1}},

		{&trillian.SignedMapRoot{}},
		{&trillian.SignedMapRoot{
			MapId: 0xcafe,
		}},
		{&trillian.SignedMapRoot{
			Metadata: &trillian.MapperMetadata{},
		}},
		{&trillian.SignedMapRoot{
			Metadata: &trillian.MapperMetadata{
				HighestFullyCompletedSeq: 0,
			},
		}},
		{obj: &trillian.SignedMapRoot{
			Metadata: &trillian.MapperMetadata{
				HighestFullyCompletedSeq: 1,
			},
		}},
	} {
		sig, err := signer.SignObject(tc.obj)
		if err != nil {
			t.Errorf("SignObject(%#v): %v", tc.obj, err)
			continue
		}

		objFromBytes, err := toFromBytes(tc.obj)
		t.Logf("fromBytes: %v", pretty.Sprint(objFromBytes))
		if err != nil {
			t.Errorf("round trip to bytes failed for obj: %#v: %v", tc.obj, err)
		} else {
			if err := VerifyObject(key.Public(), objFromBytes, sig); err != nil {
				t.Errorf("fromBytes: VerifyObject(%#v): %v", objFromBytes, err)
			}
		}
		objFromJSON, err := toFromJSON(tc.obj)
		t.Logf("fromJSON: %v", pretty.Sprint(objFromJSON))
		if err != nil {
			t.Errorf("round trip to bytes failed for obj: %#v: %v", tc.obj, err)
		} else {
			if err := VerifyObject(key.Public(), objFromJSON, sig); err != nil {
				t.Errorf("fromJSON: VerifyObject(%#v): %v", objFromJSON, err)
			}
		}
	}
}

func TestSignVerifyObject(t *testing.T) {
	key, err := keys.NewFromPrivatePEM(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass)
	if err != nil {
		t.Fatalf("Failed to open test key, err=%v", err)
	}
	signer := NewSHA256Signer(key)

	type subfield struct {
		c int
	}

	for _, tc := range []struct {
		obj interface{}
	}{
		{obj: struct{ a string }{a: "foo"}},
		{obj: struct {
			a int
			b *subfield
		}{a: 1, b: &subfield{c: 0}}},
		{obj: struct {
			a int
			b *subfield
		}{a: 1, b: nil}},
	} {
		sig, err := signer.SignObject(tc.obj)
		if err != nil {
			t.Errorf("SignObject(%#v): %v", tc.obj, err)
			continue
		}
		if err := VerifyObject(key.Public(), tc.obj, sig); err != nil {
			t.Errorf("SignObject(%#v): %v", tc.obj, err)
		}
	}
}
