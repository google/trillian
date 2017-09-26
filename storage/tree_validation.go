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
	"bytes"
	"context"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/crypto/keys/der"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/errors"
)

const (
	maxDisplayNameLength = 20
	maxDescriptionLength = 200
)

// ValidateTreeForCreation returns nil if tree is valid for insertion, error
// otherwise.
// See the documentation on trillian.Tree for reference on which values are
// valid.
func ValidateTreeForCreation(ctx context.Context, tree *trillian.Tree) error {
	switch {
	case tree == nil:
		return errors.New(errors.InvalidArgument, "a tree is required")
	case tree.TreeState != trillian.TreeState_ACTIVE:
		return errors.Errorf(errors.InvalidArgument, "invalid tree_state: %s", tree.TreeState)
	case tree.TreeType == trillian.TreeType_UNKNOWN_TREE_TYPE:
		return errors.Errorf(errors.InvalidArgument, "invalid tree_type: %s", tree.TreeType)
	case tree.HashStrategy == trillian.HashStrategy_UNKNOWN_HASH_STRATEGY:
		return errors.Errorf(errors.InvalidArgument, "invalid hash_strategy: %s", tree.HashStrategy)
	case tree.HashAlgorithm == sigpb.DigitallySigned_NONE:
		return errors.Errorf(errors.InvalidArgument, "invalid hash_algorithm: %s", tree.HashAlgorithm)
	case tree.SignatureAlgorithm == sigpb.DigitallySigned_ANONYMOUS:
		return errors.Errorf(errors.InvalidArgument, "invalid signature_algorithm: %s", tree.SignatureAlgorithm)
	case tree.PrivateKey == nil:
		return errors.New(errors.InvalidArgument, "a private_key is required")
	case tree.PublicKey == nil:
		return errors.New(errors.InvalidArgument, "a public_key is required")
	case tree.Deleted:
		return errors.Errorf(errors.InvalidArgument, "invalid deleted: %v", tree.Deleted)
	case tree.DeleteTime != nil:
		return errors.Errorf(errors.InvalidArgument, "invalid delete_time: %+v (must be nil)", tree.DeleteTime)
	}

	return validateMutableTreeFields(ctx, tree)
}

// ValidateTreeForUpdate returns nil if newTree is valid for update, error
// otherwise.
// The newTree is compared to the storedTree to determine if readonly fields
// have been changed. It's assumed that storage-generated fields, such as
// update_time, have not yet changed when this method is called.
// See the documentation on trillian.Tree for reference on which fields may be
// changed and what is considered valid for each of them.
func ValidateTreeForUpdate(ctx context.Context, storedTree, newTree *trillian.Tree) error {
	// Check that readonly fields didn't change
	switch {
	case storedTree.TreeId != newTree.TreeId:
		return errors.New(errors.InvalidArgument, "readonly field changed: tree_id")
	case storedTree.TreeType != newTree.TreeType:
		return errors.New(errors.InvalidArgument, "readonly field changed: tree_type")
	case storedTree.HashStrategy != newTree.HashStrategy:
		return errors.New(errors.InvalidArgument, "readonly field changed: hash_strategy")
	case storedTree.HashAlgorithm != newTree.HashAlgorithm:
		return errors.New(errors.InvalidArgument, "readonly field changed: hash_algorithm")
	case storedTree.SignatureAlgorithm != newTree.SignatureAlgorithm:
		return errors.New(errors.InvalidArgument, "readonly field changed: signature_algorithm")
	case !proto.Equal(storedTree.CreateTime, newTree.CreateTime):
		return errors.New(errors.InvalidArgument, "readonly field changed: create_time")
	case !proto.Equal(storedTree.UpdateTime, newTree.UpdateTime):
		return errors.New(errors.InvalidArgument, "readonly field changed: update_time")
	case !proto.Equal(storedTree.PublicKey, newTree.PublicKey):
		return errors.New(errors.InvalidArgument, "readonly field changed: public_key")
	case storedTree.Deleted != newTree.Deleted:
		return errors.New(errors.InvalidArgument, "readonly field changed: deleted")
	case !proto.Equal(storedTree.DeleteTime, newTree.DeleteTime):
		return errors.New(errors.InvalidArgument, "readonly field changed: delete_time")
	}
	return validateMutableTreeFields(ctx, newTree)
}

func validateMutableTreeFields(ctx context.Context, tree *trillian.Tree) error {
	switch {
	case tree.TreeState == trillian.TreeState_UNKNOWN_TREE_STATE:
		return errors.Errorf(errors.InvalidArgument, "invalid tree_state: %v", tree.TreeState)
	case len(tree.DisplayName) > maxDisplayNameLength:
		return errors.Errorf(errors.InvalidArgument, "display_name too big, max length is %v: %v", maxDisplayNameLength, tree.DisplayName)
	case len(tree.Description) > maxDescriptionLength:
		return errors.Errorf(errors.InvalidArgument, "description too big, max length is %v: %v", maxDescriptionLength, tree.Description)
	}
	if duration, err := ptypes.Duration(tree.MaxRootDuration); err != nil {
		return errors.Errorf(errors.InvalidArgument, "max_root_duration malformed: %v", tree.MaxRootDuration)
	} else if duration < 0 {
		return errors.Errorf(errors.InvalidArgument, "max_root_duration negative: %v", tree.MaxRootDuration)
	}

	// Implementations may vary, so let's assume storage_settings is mutable.
	// Other than checking that it's a valid Any there isn't much to do at this layer, though.
	if tree.StorageSettings != nil {
		var settings ptypes.DynamicAny
		if err := ptypes.UnmarshalAny(tree.StorageSettings, &settings); err != nil {
			return errors.Errorf(errors.InvalidArgument, "invalid storage_settings: %v", err)
		}
	}

	var privateKeyProto ptypes.DynamicAny
	if err := ptypes.UnmarshalAny(tree.PrivateKey, &privateKeyProto); err != nil {
		return errors.Errorf(errors.InvalidArgument, "invalid private_key: %v", err)
	}

	// Check that the private key can be obtained and matches the public key.
	privateKey, err := keys.NewSigner(ctx, privateKeyProto.Message)
	if err != nil {
		return errors.Errorf(errors.InvalidArgument, "invalid private_key: %v", err)
	}
	publicKeyDER, err := der.MarshalPublicKey(privateKey.Public())
	if err != nil {
		return errors.Errorf(errors.InvalidArgument, "invalid private_key: %v", err)
	}
	if !bytes.Equal(publicKeyDER, tree.PublicKey.GetDer()) {
		return errors.Errorf(errors.InvalidArgument, "private_key and public_key are not a matching pair")
	}

	return nil
}
