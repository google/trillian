// Copyright 2017 Google Inc. All Rights Reserved.
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

import (
	"fmt"

	"github.com/google/trillian"
	spb "github.com/google/trillian/proto/signature"
)

const (
	maxDisplayNameLength = 20
	maxDescriptionLength = 200
)

// ValidateTreeForCreation returns nil if tree is valid for insertion, error
// otherwise.
// See the documentation on trillian.Tree for reference on which values are
// valid.
func ValidateTreeForCreation(tree *trillian.Tree) error {
	switch {
	case tree.TreeState != trillian.TreeState_ACTIVE:
		return fmt.Errorf("invalid tree_state: %s", tree.TreeState)
	case tree.TreeType == trillian.TreeType_UNKNOWN_TREE_TYPE:
		return fmt.Errorf("invalid tree_type: %s", tree.TreeType)
	case tree.HashStrategy == trillian.HashStrategy_UNKNOWN_HASH_STRATEGY:
		return fmt.Errorf("invalid hash_strategy: %s", tree.HashStrategy)
	case tree.HashAlgorithm == spb.DigitallySigned_NONE:
		return fmt.Errorf("invalid hash_algorithm: %s", tree.HashStrategy)
	case tree.SignatureAlgorithm == spb.DigitallySigned_ANONYMOUS:
		return fmt.Errorf("invalid signature_algorithm: %s", tree.HashStrategy)
	case tree.DuplicatePolicy == trillian.DuplicatePolicy_UNKNOWN_DUPLICATE_POLICY:
		return fmt.Errorf("invalid duplicate_policy: %s", tree.HashStrategy)
	case len(tree.DisplayName) > maxDisplayNameLength:
		return fmt.Errorf("display_name too big, max length is %v: %v", maxDisplayNameLength, tree.HashStrategy)
	case len(tree.Description) > maxDescriptionLength:
		return fmt.Errorf("display_name too big, max length is %v: %v", maxDisplayNameLength, tree.HashStrategy)
	}
	return nil
}
