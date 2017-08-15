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

package der

import (
	"bytes"
	"encoding/pem"
	"testing"

	"github.com/google/trillian/testonly"
)

func mustMarshalPublicPEMToDER(keyPEM string) []byte {
	block, rest := pem.Decode([]byte(keyPEM))
	if block == nil {
		panic("empty pem block")
	}
	if len(rest) > 0 {
		panic("more than one pem block")
	}
	return block.Bytes
}

func TestMarshalUnmarshalPublicKey(t *testing.T) {
	for _, tc := range []struct {
		pubPEM string
	}{
		{pubPEM: testonly.DemoPublicKey},
	} {
		pubDER := mustMarshalPublicPEMToDER(tc.pubPEM)
		pubKey, err := UnmarshalPublicKey(pubDER)
		if err != nil {
			t.Errorf("UnmarshalPublicKey(%v): %v", pubDER, err)
			continue
		}

		pubDER2, err := MarshalPublicKey(pubKey)
		if err != nil {
			t.Errorf("MarshalPublicKey2(%v): %v", pubKey, err)
			continue
		}

		if got, want := pubDER2, pubDER; !bytes.Equal(got, want) {
			t.Errorf("MarshalPublicKey(): %x, want %x", got, want)
		}
	}
}

func TestFromToPublicProto(t *testing.T) {
	for _, tc := range []struct {
		pubPEM string
	}{
		{pubPEM: testonly.DemoPublicKey},
	} {
		pubDER := mustMarshalPublicPEMToDER(tc.pubPEM)
		pubKey, err := UnmarshalPublicKey(pubDER)
		if err != nil {
			t.Errorf("UnmarshalPublicKey(%v): %v", pubDER, err)
			continue
		}

		pubProto, err := ToPublicProto(pubKey)
		if err != nil {
			t.Errorf("ToPublicProto(%v): %v", pubKey, err)
			continue
		}

		pubKey2, err := FromPublicProto(pubProto)
		if err != nil {
			t.Errorf("FromPublicProto(%v): %v", pubKey, err)
			continue
		}

		pubDER2, err := MarshalPublicKey(pubKey2)
		if err != nil {
			t.Errorf("MarshalPublicKey2(%v): %v", pubKey, err)
			continue
		}

		if got, want := pubDER2, pubDER; !bytes.Equal(got, want) {
			t.Errorf("MarshalPublicKey(): %x, want %x", got, want)
		}

	}
}
