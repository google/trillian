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

package storage

import (
	"bytes"
	"context"

	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/crypto/keys/der"
	"github.com/google/trillian/crypto/sigpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// ValidateTreeForCreation returns nil if tree is valid for insertion, error
// otherwise.
// See the documentation on trillian.Tree for reference on which values are
// valid.
func ValidateTreeForCreation(ctx context.Context, tree *trillian.Tree) error {
	switch {
	case tree == nil:
		return status.Error(codes.InvalidArgument, "a tree is required")
	case tree.TreeState != trillian.TreeState_ACTIVE:
		return status.Errorf(codes.InvalidArgument, "invalid tree_state: %s", tree.TreeState)
	case tree.TreeType == trillian.TreeType_UNKNOWN_TREE_TYPE:
		return status.Errorf(codes.InvalidArgument, "invalid tree_type: %s", tree.TreeType)
	case tree.HashAlgorithm == sigpb.DigitallySigned_NONE:
		return status.Errorf(codes.InvalidArgument, "invalid hash_algorithm: %s", tree.HashAlgorithm)
	case tree.PrivateKey == nil:
		return status.Error(codes.InvalidArgument, "a private_key is required")
	case tree.PublicKey == nil:
		return status.Error(codes.InvalidArgument, "a public_key is required")
	case tree.Deleted:
		return status.Errorf(codes.InvalidArgument, "invalid deleted: %v", tree.Deleted)
	case tree.DeleteTime != nil:
		return status.Errorf(codes.InvalidArgument, "invalid delete_time: %+v (must be nil)", tree.DeleteTime)
	}

	return validateMutableTreeFields(ctx, tree)
}

// validateTreeTypeUpdate returns nil iff oldTree.TreeType can be updated to
// newTree.TreeType. The tree type is changeable only if the Tree is and
// remains in the FROZEN state.
// At the moment only PREORDERED_LOG->LOG type transition is permitted.
func validateTreeTypeUpdate(oldTree, newTree *trillian.Tree) error {
	const prefix = "can't change tree_type"

	const wantState = trillian.TreeState_FROZEN
	if oldState := oldTree.TreeState; oldState != wantState {
		return status.Errorf(codes.InvalidArgument, "%s: tree_state=%v, want %v", prefix, oldState, wantState)
	} else if newTree.TreeState != wantState {
		return status.Errorf(codes.InvalidArgument, "%s: tree_state should stay %v", prefix, wantState)
	}

	if oldTree.TreeType != trillian.TreeType_PREORDERED_LOG || newTree.TreeType != trillian.TreeType_LOG {
		return status.Errorf(codes.InvalidArgument, "%s: %v->%v", prefix, oldTree.TreeType, newTree.TreeType)
	}
	return nil
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
		return status.Error(codes.InvalidArgument, "readonly field changed: tree_id")
	case storedTree.TreeType != newTree.TreeType:
		if err := validateTreeTypeUpdate(storedTree, newTree); err != nil {
			return err
		}
	case storedTree.HashAlgorithm != newTree.HashAlgorithm:
		return status.Error(codes.InvalidArgument, "readonly field changed: hash_algorithm")
	case !proto.Equal(storedTree.CreateTime, newTree.CreateTime):
		return status.Error(codes.InvalidArgument, "readonly field changed: create_time")
	case !proto.Equal(storedTree.UpdateTime, newTree.UpdateTime):
		return status.Error(codes.InvalidArgument, "readonly field changed: update_time")
	case !proto.Equal(storedTree.PublicKey, newTree.PublicKey):
		return status.Error(codes.InvalidArgument, "readonly field changed: public_key")
	case storedTree.Deleted != newTree.Deleted:
		return status.Error(codes.InvalidArgument, "readonly field changed: deleted")
	case !proto.Equal(storedTree.DeleteTime, newTree.DeleteTime):
		return status.Error(codes.InvalidArgument, "readonly field changed: delete_time")
	}
	return validateMutableTreeFields(ctx, newTree)
}

func validateMutableTreeFields(ctx context.Context, tree *trillian.Tree) error {
	if tree.TreeState == trillian.TreeState_UNKNOWN_TREE_STATE {
		return status.Errorf(codes.InvalidArgument, "invalid tree_state: %v", tree.TreeState)
	}
	if err := tree.MaxRootDuration.CheckValid(); err != nil {
		return status.Errorf(codes.InvalidArgument, "max_root_duration malformed: %v", err)
	} else if duration := tree.MaxRootDuration.AsDuration(); duration < 0 {
		return status.Errorf(codes.InvalidArgument, "max_root_duration negative: %v", tree.MaxRootDuration)
	}

	// Implementations may vary, so let's assume storage_settings is mutable.
	// Other than checking that it's a valid Any there isn't much to do at this layer, though.
	if tree.StorageSettings != nil {
		_, err := tree.StorageSettings.UnmarshalNew()
		if err != nil {
			return status.Errorf(codes.InvalidArgument, "invalid storage_settings: %v", err)
		}
	}

	privateKeyProto, err := tree.PrivateKey.UnmarshalNew()
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid private_key: %v", err)
	}

	// Check that the private key can be obtained and matches the public key.
	privateKey, err := keys.NewSigner(ctx, privateKeyProto)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid private_key: %v", err)
	}
	publicKeyDER, err := der.MarshalPublicKey(privateKey.Public())
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid private_key: %v", err)
	}
	if !bytes.Equal(publicKeyDER, tree.PublicKey.GetDer()) {
		return status.Errorf(codes.InvalidArgument, "private_key and public_key are not a matching pair")
	}

	return nil
}
