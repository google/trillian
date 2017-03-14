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
	"errors"
	"fmt"

	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/sigpb"
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
	// Check that the private_key proto contains a valid serialized proto.
	// TODO(robpercival): Could we attempt to produce an STH at this point,
	// to verify that the key works?
	var privateKey ptypes.DynamicAny
	if err := ptypes.UnmarshalAny(tree.PrivateKey, &privateKey); err != nil {
		return fmt.Errorf("invalid private_key: %v", err)
	}

	switch {
	case tree.TreeState != trillian.TreeState_ACTIVE:
		return fmt.Errorf("invalid tree_state: %s", tree.TreeState)
	case tree.TreeType == trillian.TreeType_UNKNOWN_TREE_TYPE:
		return fmt.Errorf("invalid tree_type: %s", tree.TreeType)
	case tree.HashStrategy == trillian.HashStrategy_UNKNOWN_HASH_STRATEGY:
		return fmt.Errorf("invalid hash_strategy: %s", tree.HashStrategy)
	case tree.HashAlgorithm == sigpb.DigitallySigned_NONE:
		return fmt.Errorf("invalid hash_algorithm: %s", tree.HashAlgorithm)
	case tree.SignatureAlgorithm == sigpb.DigitallySigned_ANONYMOUS:
		return fmt.Errorf("invalid signature_algorithm: %s", tree.SignatureAlgorithm)
	case tree.DuplicatePolicy == trillian.DuplicatePolicy_UNKNOWN_DUPLICATE_POLICY:
		return fmt.Errorf("invalid duplicate_policy: %s", tree.DuplicatePolicy)
	}
	return validateMutableTreeFields(tree)
}

// ValidateTreeForUpdate returns nil if newTree is valid for update, error
// otherwise.
// The newTree is compared to the storedTree to determine if readonly fields
// have been changed. It's assumed that storage-generated fields, such as
// update_time, have not yet changed when this method is called.
// See the documentation on trillian.Tree for reference on which fields may be
// changed and what is considered valid for each of them.
func ValidateTreeForUpdate(storedTree, newTree *trillian.Tree) error {
	// Check that readonly fields didn't change
	switch {
	case storedTree.TreeId != newTree.TreeId:
		return errors.New("readonly field changed: tree_id")
	case storedTree.TreeType != newTree.TreeType:
		return errors.New("readonly field changed: tree_type")
	case storedTree.HashStrategy != newTree.HashStrategy:
		return errors.New("readonly field changed: hash_strategy")
	case storedTree.HashAlgorithm != newTree.HashAlgorithm:
		return errors.New("readonly field changed: hash_algorithm")
	case storedTree.SignatureAlgorithm != newTree.SignatureAlgorithm:
		return errors.New("readonly field changed: signature_algorithm")
	case storedTree.DuplicatePolicy != newTree.DuplicatePolicy:
		return errors.New("readonly field changed: duplicate_policy")
	case storedTree.CreateTimeMillisSinceEpoch != newTree.CreateTimeMillisSinceEpoch:
		return errors.New("readonly field changed: create_time")
	case storedTree.UpdateTimeMillisSinceEpoch != newTree.UpdateTimeMillisSinceEpoch:
		return errors.New("readonly field changed: update_time")
	case storedTree.PrivateKey != newTree.PrivateKey:
		return errors.New("readonly field changed: private_key")
	}
	return validateMutableTreeFields(newTree)
}

func validateMutableTreeFields(tree *trillian.Tree) error {
	switch {
	case tree.TreeState == trillian.TreeState_UNKNOWN_TREE_STATE:
		return fmt.Errorf("invalid tree_state: %v", tree.TreeState)
	case len(tree.DisplayName) > maxDisplayNameLength:
		return fmt.Errorf("display_name too big, max length is %v: %v", maxDisplayNameLength, tree.DisplayName)
	case len(tree.Description) > maxDescriptionLength:
		return fmt.Errorf("description too big, max length is %v: %v", maxDescriptionLength, tree.Description)
	}
	return nil
}
