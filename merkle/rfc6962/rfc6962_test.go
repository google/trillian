// Copyright 2016 Google Inc. All Rights Reserved.
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
package rfc6962

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestRfc6962Hasher(t *testing.T) {
	hasher := DefaultHasher

	leafHash, err := hasher.HashLeaf([]byte("L123456"))
	if err != nil {
		t.Fatalf("HashLeaf(): %v", err)
	}
	for _, tc := range []struct {
		desc string
		got  []byte
		want string
	}{
		// echo -n | sha256sum
		{
			desc: "RFC962 Empty",
			want: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			got:  hasher.EmptyRoot(),
		},
		// echo -n 004C313233343536 | xxd -r -p | sha256sum
		{
			desc: "RFC6962 Leaf",
			want: "395aa064aa4c29f7010acfe3f25db9485bbd4b91897b6ad7ad547639252b4d56",
			got:  leafHash,
		},
		// echo -n 014E3132334E343536 | xxd -r -p | sha256sum
		{
			desc: "RFC6962 Node",
			want: "aa217fe888e47007fa15edab33c2b492a722cb106c64667fc2b044444de66bbb",
			got:  hasher.HashChildren([]byte("N123"), []byte("N456")),
		},
	} {
		wantBytes, err := hex.DecodeString(tc.want)
		if err != nil {
			t.Errorf("hex.DecodeString(%v): %v", tc.want, err)
			continue
		}

		if got, want := tc.got, wantBytes; !bytes.Equal(got, want) {
			t.Fatalf("%v: got %x, want %x", tc.desc, got, want)
		}
	}
}
