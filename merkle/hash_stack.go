// Copyright 2018 Google Inc. All Rights Reserved.
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

import "github.com/google/trillian/merkle/hashers"

type hashStack struct {
	hasher hashers.LogHasher
	hashes [][]byte
	begin  int64
	index  int64
}

func newHashStack(hasher hashers.LogHasher, begin int64) hashStack {
	return hashStack{hasher: hasher, begin: begin, index: begin}
}

func (s *hashStack) pushAndChain(h []byte) {
	pop, index, ln := 0, s.index, len(s.hashes)
	for one := int64(1); index&one != 0; one <<= 1 {
		index ^= one
		if index < s.begin || pop >= ln {
			break
		}
		pop++
		h = s.hasher.HashChildren(s.hashes[ln-pop], h)
	}
	s.hashes = append(s.hashes[:ln-pop], h)
	s.index++
}
