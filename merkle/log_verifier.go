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

package merkle

import (
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/merkle/logverifier"
)

// RootMismatchError occurs when an inclusion proof fails.
//
// Deprecated: moved to github.com/google/trillian/merkle/logverifier package
type RootMismatchError = logverifier.RootMismatchError

// LogVerifier verifies inclusion and consistency proofs for append only logs.
//
// Deprecated: moved to github.com/google/trillian/merkle/logverifier package
type LogVerifier = logverifier.LogVerifier

// NewLogVerifier returns a new LogVerifier for a tree.
//
// Deprecated: moved to github.com/google/trillian/merkle/logverifier package
func NewLogVerifier(hasher hashers.LogHasher) LogVerifier {
	return logverifier.New(hasher)
}
