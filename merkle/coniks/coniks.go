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

// Package coniks provides CONIKS hashing for maps.
package coniks

import (
	"crypto"

	"github.com/google/trillian"
	"github.com/google/trillian/merkle/coniks/hasher"
	"github.com/google/trillian/merkle/hashers/registry"
)

// Default is the standard CONIKS hasher.
//
// Deprecated: moved to hasher subpackage.
var Default = hasher.Default

func init() {
	registry.RegisterMapHasher(trillian.HashStrategy_CONIKS_SHA512_256, Default)
	registry.RegisterMapHasher(trillian.HashStrategy_CONIKS_SHA256, hasher.New(crypto.SHA256))
}

// Hasher implements the sparse merkle tree hashing algorithm specified in the CONIKS paper.
//
// Deprecated: moved to hasher subpackage.
type Hasher = hasher.Hasher

// New creates a new hashers.TreeHasher using the passed in hash function.
//
// Deprecated: moved to hasher subpackage.
func New(h crypto.Hash) *Hasher {
	return hasher.New(h)
}
