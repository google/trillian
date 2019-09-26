// Copyright 2019 Google Inc. All Rights Reserved.
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

package hammer

import (
	"testing"

	"github.com/google/trillian/testonly"
	"github.com/google/trillian/types"
)

func TestSmrStash_PushSMR(t *testing.T) {
	r0 := types.MapRootV1{Revision: 0, RootHash: testonly.MustHexDecode("AAAA")}
	r1 := types.MapRootV1{Revision: 1, RootHash: testonly.MustHexDecode("BBBB")}
	r2 := types.MapRootV1{Revision: 2, RootHash: testonly.MustHexDecode("CCCC")}

	for _, test := range []struct {
		desc    string
		seq     []types.MapRootV1
		wantErr bool
	}{
		{
			desc: "single root",
			seq:  []types.MapRootV1{r0},
		},
		{
			desc: "all the roots in order",
			seq:  []types.MapRootV1{r0, r1, r2},
		},
		{
			desc:    "roots out of order",
			seq:     []types.MapRootV1{r2, r1},
			wantErr: true,
		},
		{
			desc: "same revision with same contents",
			seq:  []types.MapRootV1{r0, r0},
		},
		{
			desc:    "same revision but different contents",
			seq:     []types.MapRootV1{r0, {Revision: 0, RootHash: testonly.MustHexDecode("FFFF")}},
			wantErr: true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			var stash smrStash

			var gotErr error
			for _, r := range test.seq {
				err := stash.pushSMR(r)
				if err != nil {
					gotErr = err
				}
			}
			if (gotErr != nil) != test.wantErr {
				t.Errorf("Unexpected error state: %v, wantErr: %v", gotErr, test.wantErr)
			}
		})
	}
}
