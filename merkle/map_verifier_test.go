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

package merkle

import (
	"testing"

	"github.com/google/trillian/merkle/coniks"
	"github.com/google/trillian/merkle/maphasher"
	"github.com/google/trillian/testonly"
)

func TestConiksHasherTestVectors(t *testing.T) {
	// Test vectors were obtained by copying values from
	// github.com/google/keytransparency/core/client/kt/verify_test.go

	h := coniks.Default
	for _, tc := range []struct {
		desc   string
		index  []byte
		proof  [][]byte
		root   []byte
		leaf   []byte
		treeID int64
	}{
		{
			desc:   "Empty proof",
			index:  []byte{0xa5, 0xd1, 0x37, 0xb4, 0x39, 0x38, 0xce, 0x3f, 0x28, 0x69, 0x55, 0x42, 0x6, 0xb1, 0x96, 0x4b, 0x84, 0x95, 0xda, 0xa2, 0x54, 0x8, 0xf2, 0x75, 0x75, 0x80, 0x1a, 0xc0, 0x71, 0xba, 0xed, 0xa7},
			proof:  make([][]byte, 256),
			root:   []byte{0x0e, 0xfc, 0x54, 0xad, 0xe0, 0xfc, 0xe8, 0x76, 0x55, 0x8c, 0x97, 0x38, 0xf5, 0xaa, 0x89, 0xe4, 0xd9, 0x9c, 0x0b, 0x8b, 0x6f, 0xe0, 0xb6, 0x2d, 0xbf, 0x63, 0x59, 0xcf, 0xc2, 0xad, 0xbb, 0xd7},
			treeID: 9175411803742040796,
		},
		{
			desc:  "One item",
			index: []byte{0xb7, 0x57, 0x2d, 0xf6, 0xe1, 0x9, 0x1f, 0xc0, 0x6, 0x9e, 0x4, 0xbf, 0x80, 0x98, 0x75, 0x25, 0xe7, 0x7a, 0xc9, 0xa6, 0xc2, 0x94, 0xd2, 0x8d, 0xb7, 0xf4, 0xe3, 0x60, 0x25, 0x1d, 0x83, 0xbf},
			proof: append(make([][]byte, 255), []byte{
				92, 215, 13, 113, 97, 138, 214, 158, 13, 29, 227, 67, 236, 34, 215, 4, 76, 188, 79, 247, 149, 223, 227, 147, 86, 214, 90, 126, 192, 212, 113, 64,
			}),
			root:   []byte{0x2c, 0x27, 0x03, 0xe0, 0x34, 0xf4, 0x00, 0x2f, 0x94, 0x1d, 0xfc, 0xea, 0x7a, 0x4e, 0x16, 0x03, 0xee, 0x8b, 0x4e, 0xe3, 0x75, 0xbd, 0xf8, 0x72, 0x5e, 0xb8, 0xaf, 0x04, 0xbf, 0xa3, 0xd1, 0x56},
			treeID: 2595744899657020594,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			if err := VerifyMapInclusionProof(tc.treeID, tc.index, tc.leaf, tc.root, tc.proof, h); err != nil {
				t.Errorf("VerifyMapInclusionProof failed: %v", err)
			}
		})
	}
}

func TestMapHasherTestVectors(t *testing.T) {
	// Test vectors were copied from a python implementation.
	h := maphasher.Default
	tv := struct {
		Index        []byte
		Value        []byte
		Proof        [][]byte
		ExpectedRoot []byte
	}{

		testonly.HashKey("key-0-848"),
		[]byte("value-0-848"),
		[][]byte{
			// 246 x nil
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil,
			testonly.MustDecodeBase64("vMWPHFclXXchQbAGJr6pcB002vQZYHnJTfOC42E1iT8="),
			nil,
			testonly.MustDecodeBase64("C3VKkaOliXmuHXM0zrkSulYX6ORaNG8qWHez/dyQkQs="),
			testonly.MustDecodeBase64("7vmVXjPm0XhOMJlnpxJa/ZKn8eeK0PIthOOy74w+sJc="),
			testonly.MustDecodeBase64("vEWXkf+9ZJQ/oxyyOaQdIfZfsx2GCA/NldZ+UopQF6Y="),
			testonly.MustDecodeBase64("lrGGFxtBKRdE53Dl6p0GeFgM6VomF9Fx5k/6+aIzMWc="),
			testonly.MustDecodeBase64("I5nVuy9wljpxbgv/aE9ivo854GhFRdsAWwmmEXDjaxE="),
			testonly.MustDecodeBase64("yAxifDRQUd+vjc6RaHG9f8tCWSa0mzV4rry50khiD3M="),
			testonly.MustDecodeBase64("YmUpJx/UagsoBYv6PnFRaVYw3x6kAx3N3OOSyiXsGtg="),
			testonly.MustDecodeBase64("CtC2GCsc3/zFn1DNkoUThUnn7k+DMotaNXvmceKIL4Y="),
		},
		testonly.MustDecodeBase64("U6ANU1en3BSbbnWqhV2nTGtQ+scBlaZf9kRPEEDZsHM="),
	}

	// Copy the bad proof so we don't mess up the good proof.
	badProof := make([][]byte, len(tv.Proof))
	for i := range badProof {
		badProof[i] = make([]byte, len(tv.Proof[i]))
		copy(badProof[i], tv.Proof[i])
	}
	badProof[250][15] ^= 0x10

	for _, tc := range []struct {
		desc              string
		index, leaf, root []byte
		proof             [][]byte
		want              bool
	}{
		{"correct", tv.Index, tv.Value, tv.ExpectedRoot, tv.Proof, true},
		{"incorrect key", []byte("w"), tv.Value, tv.ExpectedRoot, tv.Proof, false},
		{"incorrect value", tv.Index, []byte("w"), tv.ExpectedRoot, tv.Proof, false},
		{"incorrect root", tv.Index, tv.Value, []byte("w"), tv.Proof, false},
		{"incorrect proof", tv.Index, tv.Value, tv.ExpectedRoot, badProof, false},
		{"short proof", tv.Index, tv.Value, tv.ExpectedRoot, [][]byte{[]byte("shorty")}, false},
		{"excess proof", tv.Index, tv.Value, tv.ExpectedRoot, make([][]byte, h.Size()*8+1), false},
	} {
		err := VerifyMapInclusionProof(treeID, tc.index, tc.leaf, tc.root, tc.proof, h)
		if got := err == nil; got != tc.want {
			t.Errorf("%v: VerifyMapInclusionProof(): %v, want %v", tc.desc, err, tc.want)
		}
	}
}
