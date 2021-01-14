// Copyright 2021 Google LLC. All Rights Reserved.
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

// Package rfc6962 provides hashing functionality according to RFC6962.
package rfc6962

import (
	"crypto"

	"github.com/google/trillian"
	"github.com/google/trillian/merkle/hashers/registry"
	"github.com/google/trillian/merkle/rfc6962/hasher"
)

func init() {
	registry.RegisterLogHasher(trillian.HashStrategy_RFC6962_SHA256, New(crypto.SHA256))
}

// Domain separation prefixes
//
// Deprecated: moved to hasher subpackage.
const (
	RFC6962LeafHashPrefix = hasher.RFC6962LeafHashPrefix
	RFC6962NodeHashPrefix = hasher.RFC6962NodeHashPrefix
)

// DefaultHasher is a SHA256 based LogHasher.
//
// Deprecated: moved to hasher subpackage.
var DefaultHasher = hasher.DefaultHasher

// Hasher implements the RFC6962 tree hashing algorithm.
//
// Deprecated: moved to hasher subpackage.
type Hasher = hasher.Hasher

// New creates a new Hashers.LogHasher on the passed in hash function.
//
// Deprecated: moved to hasher subpackage.
func New(h crypto.Hash) *Hasher {
	return hasher.New(h)
}
