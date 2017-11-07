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

package objhasher

import (
	"bytes"
	"crypto"
	"encoding/hex"
	"testing"

	"github.com/google/trillian/merkle/maphasher"
	"github.com/google/trillian/merkle/rfc6962"
)

const treeID = int64(0)

func TestLeafHash(t *testing.T) {
	h := NewLogHasher(rfc6962.New(crypto.SHA256))

	for _, tc := range []struct {
		json []byte
		want string
	}{
		{
			// Verify that ordering does not affect output hash.
			json: []byte(`{"k1":"v1","k2":"v2","k3":"v3"}`),
			want: "ddd65f1f7568269a30df7cafc26044537dc2f02a1a0d830da61762fc3e687057",
		},
		{
			// Same values, different order.
			json: []byte(`{"k2":"v2","k1":"v1","k3":"v3"}`),
			want: "ddd65f1f7568269a30df7cafc26044537dc2f02a1a0d830da61762fc3e687057",
		},
	} {
		leaf, err := h.HashLeaf(tc.json)
		if err != nil {
			t.Errorf("HashLeaf(%v): %v", tc.json, err)
		}
		if got := hex.EncodeToString(leaf); got != tc.want {
			t.Errorf("HashLeaf(%v): \n%v, want \n%v", tc.json, got, tc.want)
		}
	}
}

func TestHashEmpty(t *testing.T) {
	h := NewMapHasher(maphasher.New(crypto.SHA256))
	rfc := maphasher.New(crypto.SHA256)

	if got, want := h.HashEmpty(treeID, nil, 0), rfc.HashEmpty(treeID, nil, 0); !bytes.Equal(got, want) {
		t.Errorf("HashEmpty():\n%x, want\n%x", got, want)
	}
}

func TestHashChildren(t *testing.T) {
	h := NewMapHasher(maphasher.New(crypto.SHA256))
	rfc := maphasher.New(crypto.SHA256)

	for _, tc := range []struct {
		r, l []byte
	}{
		{
			r: []byte("a"), l: []byte("b"),
		},
	} {
		if got, want := h.HashChildren(tc.r, tc.l), rfc.HashChildren(tc.r, tc.l); !bytes.Equal(got, want) {
			t.Errorf("HashChildren(%x, %x):\n%x, want\n%x", tc.r, tc.l, got, want)
		}
	}
}
