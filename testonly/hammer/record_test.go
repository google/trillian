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
	"fmt"
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/types"
)

func TestSharedState_advertiseSMR(t *testing.T) {
	r0 := types.MapRootV1{Revision: 0, RootHash: testonly.MustHexDecode("AAAA")}
	r1 := types.MapRootV1{Revision: 1, RootHash: testonly.MustHexDecode("BBBB")}
	r2 := types.MapRootV1{Revision: 2, RootHash: testonly.MustHexDecode("CCCC")}

	for _, test := range []struct {
		desc      string
		seq       []types.MapRootV1
		writeRevs uint64
		wantErr   bool
	}{
		{
			desc: "single root",
			seq:  []types.MapRootV1{r0},
		},
		{
			desc:      "all the roots in order",
			seq:       []types.MapRootV1{r0, r1, r2},
			writeRevs: 2,
		},
		{
			desc:      "read ahead of writes",
			seq:       []types.MapRootV1{r0, r1, r2},
			writeRevs: 1,
			wantErr:   true,
		},
		{
			desc:      "roots out of order",
			seq:       []types.MapRootV1{r2, r1},
			writeRevs: 2,
			wantErr:   false,
		},
		{
			desc: "same revision with same contents",
			seq:  []types.MapRootV1{r0, r0},
		},
		{
			desc:      "same revision but different contents",
			seq:       []types.MapRootV1{r0, r1, r2, {Revision: 1, RootHash: testonly.MustHexDecode("FFFF")}},
			writeRevs: 2,
			wantErr:   true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			g := newSharedState()

			for i := uint64(1); i <= test.writeRevs; i++ {
				leaves := []*trillian.MapLeaf{}
				g.proposeLeaves(i, leaves)
			}

			var gotErr error
			for _, r := range test.seq {
				err := g.advertiseSMR(r)
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

func TestSharedState_keepsReadableRevisions(t *testing.T) {
	g := newSharedState()

	for _, test := range []struct {
		writeRevs   uint64
		readSMRs    uint64
		lookupRev   uint64
		expectFound bool
	}{
		{
			writeRevs:   0,
			readSMRs:    0,
			lookupRev:   0,
			expectFound: false,
		},
		{
			writeRevs:   1,
			readSMRs:    1,
			lookupRev:   1,
			expectFound: true,
		},
		{
			writeRevs:   30,
			readSMRs:    10,
			lookupRev:   5,
			expectFound: true,
		},
		{
			writeRevs:   30,
			readSMRs:    25,
			lookupRev:   5,
			expectFound: false,
		},
	} {
		t.Run(fmt.Sprintf("write=%d,read=%d,lookup=%d", test.writeRevs, test.readSMRs, test.lookupRev), func(t *testing.T) {
			for i := uint64(1); i <= test.writeRevs; i++ {
				leaves := []*trillian.MapLeaf{}
				g.proposeLeaves(i, leaves)
			}
			for i := uint64(1); i <= test.readSMRs; i++ {
				g.advertiseSMR(types.MapRootV1{Revision: i, RootHash: testonly.MustHexDecode("AAAA")})
			}

			got := g.contents.PickRevision(test.lookupRev)
			if (got != nil) != test.expectFound {
				t.Errorf("Unexpected found status: got %v, want: %v", (got != nil), test.expectFound)
			}
			if got != nil && got.Rev != int64(test.lookupRev) {
				t.Errorf("Unexpected rev: got %d, want: %d", got.Rev, test.lookupRev)
			}
		})
	}
}
