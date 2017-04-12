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

package coresql

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"sort"
	"testing"

	"github.com/google/trillian"
)

func TestSortByLeafIdentityHash(t *testing.T) {
	l := make([]*trillian.LogLeaf, 30)
	for i := range l {
		hash := sha256.Sum256([]byte{byte(i)})
		leaf := trillian.LogLeaf{
			LeafIdentityHash: hash[:],
			LeafValue:        []byte(fmt.Sprintf("Value %d", i)),
			ExtraData:        []byte(fmt.Sprintf("Extra %d", i)),
			LeafIndex:        int64(i),
		}
		l[i] = &leaf
	}
	sort.Sort(byLeafIdentityHash(l))
	for i := range l {
		if i == 0 {
			continue
		}
		if bytes.Compare(l[i-1].LeafIdentityHash, l[i].LeafIdentityHash) != -1 {
			t.Errorf("sorted leaves not in order, [%d] = %x, [%d] = %x", i-1, l[i-1].LeafIdentityHash, i, l[i].LeafIdentityHash)
		}
	}
}
