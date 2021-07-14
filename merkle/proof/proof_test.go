// Copyright 2021 Google LLC. All Rights Reserved.
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

package proof

import (
	"testing"

	_ "github.com/golang/glog" // Logging flags for overarching "go test" runs.
	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian/merkle/rfc6962"
)

func TestRehash(t *testing.T) {
	th := rfc6962.DefaultHasher
	h := [][]byte{
		th.HashLeaf([]byte("Hash 1")),
		th.HashLeaf([]byte("Hash 2")),
		th.HashLeaf([]byte("Hash 3")),
		th.HashLeaf([]byte("Hash 4")),
		th.HashLeaf([]byte("Hash 5")),
	}

	for _, tc := range []struct {
		desc   string
		hashes [][]byte
		nodes  Nodes
		want   [][]byte
	}{
		{
			desc:   "no-rehash",
			hashes: h[:3],
			nodes:  Inclusion(3, 8),
			want:   h[:3],
		},
		{
			desc:   "rehash",
			hashes: h[:5],
			nodes:  Inclusion(9, 15),
			want:   [][]byte{h[0], h[1], th.HashChildren(h[3], h[2]), h[4]},
		},
		{
			desc:   "rehash-at-the-end",
			hashes: h[:4],
			nodes:  Inclusion(2, 7),
			want:   [][]byte{h[0], h[1], th.HashChildren(h[3], h[2])},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			h := append([][]byte{}, tc.hashes...)
			got, err := tc.nodes.Rehash(h, th.HashChildren)
			if err != nil {
				t.Fatalf("Rehash: %v", err)
			}
			if want := tc.want; !cmp.Equal(got, want) {
				t.Errorf("proofs mismatch:\ngot: %x\nwant: %x", got, want)
			}
		})
	}
}
