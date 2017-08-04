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

	"github.com/google/trillian/merkle/maphasher"
	"github.com/google/trillian/testonly"
)

func TestMapHasherTestVectors(t *testing.T) {
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
