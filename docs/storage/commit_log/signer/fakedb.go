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

package signer

// FakeDatabase simulates a local database
type FakeDatabase struct {
	leaves []string
	sth    STH
}

// Size returns the tree size of the local database
func (f *FakeDatabase) Size() int {
	return len(f.leaves)
}

// AddLeaves simulates adding leaves to the local database.  It returns
// the STH in the database.
func (f *FakeDatabase) AddLeaves(when int64, offset int, leaves []string) STH {
	f.leaves = append(f.leaves, leaves...)
	f.sth.TimeStamp = when
	f.sth.Offset = offset
	f.sth.TreeSize = f.Size()
	return f.sth
}
