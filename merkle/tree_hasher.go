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

// TreeHasher is the interface that the previous tree hasher struct implemented.
type TreeHasher interface {
	HashEmpty() []byte
	HashLeaf(leaf []byte) []byte
	HashChildren(l, r []byte) []byte
	// TODO(gbelvin): Replace Size() with BitLength().
	Size() int
}
