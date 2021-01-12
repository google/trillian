// Copyright 2016 Google LLC. All Rights Reserved.
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

package verifier

import (
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/merkle/coniks"
	"github.com/google/trillian/merkle/maphasher"
	"github.com/google/trillian/testonly"
)

var h2b = testonly.MustHexDecode

func TestConiksHasherTestVectors(t *testing.T) {
	// Test vectors were obtained by copying values from
	// github.com/google/keytransparency/core/client/kt/verify_test.go

	h := coniks.Default
	for _, tc := range []struct {
		desc   string
		index  []byte
		proof  [][]byte
		root   []byte
		value  []byte
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
			desc:  "One item, empty proof",
			index: []byte{0xb7, 0x57, 0x2d, 0xf6, 0xe1, 0x9, 0x1f, 0xc0, 0x6, 0x9e, 0x4, 0xbf, 0x80, 0x98, 0x75, 0x25, 0xe7, 0x7a, 0xc9, 0xa6, 0xc2, 0x94, 0xd2, 0x8d, 0xb7, 0xf4, 0xe3, 0x60, 0x25, 0x1d, 0x83, 0xbf},
			proof: append(make([][]byte, 255), []byte{
				92, 215, 13, 113, 97, 138, 214, 158, 13, 29, 227, 67, 236, 34, 215, 4, 76, 188, 79, 247, 149, 223, 227, 147, 86, 214, 90, 126, 192, 212, 113, 64,
			}),
			root:   []byte{0x2c, 0x27, 0x03, 0xe0, 0x34, 0xf4, 0x00, 0x2f, 0x94, 0x1d, 0xfc, 0xea, 0x7a, 0x4e, 0x16, 0x03, 0xee, 0x8b, 0x4e, 0xe3, 0x75, 0xbd, 0xf8, 0x72, 0x5e, 0xb8, 0xaf, 0x04, 0xbf, 0xa3, 0xd1, 0x56},
			treeID: 2595744899657020594,
		},
		{
			desc:   "One item, exists proof",
			index:  h2b("6e39bd1b2aec80f29204d63b9aff74b17cc0255b8e9d6a8fc4c069cfefc01ce9"),
			proof:  make([][]byte, 256),
			value:  h2b("1290010a4030393264343162666564666665666639376261313963653430356165633433383631333766373435376439303234633537663361333439366630303938366538124c080410031a463044022064cf8e276381a5fc993e471e29463dbebf99b7e361a389024a92c3cb0fa1571402200bb9af4bd2e61fa5e2ea46cd1e653ee0f264703293d4ddd6bc88fc2cde5049181a206e39bd1b2aec80f29204d63b9aff74b17cc0255b8e9d6a8fc4c069cfefc01ce932200f30aff51b696d25d423f0d0fa208efe6038d83133cdc58b8d68f9d30e029efc3a5d0a5b3059301306072a8648ce3d020106082a8648ce3d03010703420004fb154e7698647e912d97b385f280b2bd6c37d5d57886719b5c33db74594bd6799aca19eac847d1757365a414f653d857712c6588a235ac7ac02450f1de772db442201b16b1df538ba12dc3f97edbb85caa7050d46c148134290feba80f8236c83db9"),
			root:   h2b("9986cc7a6ede7443a877cb8b971bb4cf8628ba30f41a002f6eb231fd093bb49f"),
			treeID: 6099875953263526152,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			leaf := trillian.MapLeaf{Index: tc.index, LeafValue: tc.value, LeafHash: nil}
			if err := VerifyMapInclusionProof(tc.treeID, &leaf, tc.root, tc.proof, h); err != nil {
				t.Errorf("VerifyMapInclusionProof failed: %v", err)
			}
		})
	}
}

func TestMapHasherTestVectors(t *testing.T) {
	const treeID = 0

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
		leaf := trillian.MapLeaf{Index: tc.index, LeafValue: tc.leaf}
		err := VerifyMapInclusionProof(treeID, &leaf, tc.root, tc.proof, h)
		if got := err == nil; got != tc.want {
			t.Errorf("%v: VerifyMapInclusionProof(): %v, want %v", tc.desc, err, tc.want)
		}
	}
}
