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
// limitations under the License.// Package registry provides a mechanism to register and discover tree hashers.

// Package registry provides a global registry for hasher implementations.
package registry

import (
	"crypto"
	"fmt"

	"github.com/google/trillian"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/merkle/rfc6962/hasher"
)

// NewLogHasher returns a LogHasher.
func NewLogHasher(h trillian.HashStrategy) (hashers.LogHasher, error) {
	if h == trillian.HashStrategy_RFC6962_SHA256 {
		return hasher.New(crypto.SHA256), nil
	}
	return nil, fmt.Errorf("LogHasher(%s) is an unknown hasher", h)
}
