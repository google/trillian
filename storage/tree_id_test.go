// Copyright 2017 Google LLC. All Rights Reserved.
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

package storage

import "testing"

func TestNewTreeID(t *testing.T) {
	// Grab a few IDs, check that they're not zero and not repeating.
	ids := make(map[int64]bool)
	for i := 0; i < 5; i++ {
		id, err := NewTreeID()
		switch _, ok := ids[id]; {
		case err != nil:
			t.Errorf("%v: NewTreeID() = (_ %v), want = (_, nil)", i, err)
		case ok:
			t.Errorf("%v: NewTreeID() return repeated ID: %v", i, id)
		case id <= 0:
			t.Errorf("%v: NewTreeID() = %v, want > 0", i, id)
		default:
			ids[id] = true
		}
	}
}
