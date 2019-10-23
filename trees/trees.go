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

// Package trees contains utility method for retrieving trees and acquiring objects (hashers,
// signers) associated with them.
package trees

import (
	"context"
	"crypto"
	"fmt"

	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/storage"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tcrypto "github.com/google/trillian/crypto"
)

const traceSpanRoot = "/trillian/trees"

type treeKey struct{}

type accessRule struct {
	// Tree states are accepted if there is a 'true' value for them in this map.
	okStates map[trillian.TreeState]bool
	// Allows the error code to be specified for specific rejected states.
	rejectCodes map[trillian.TreeState]codes.Code
	// Tree types are accepted if there is a 'true' value for them in this map.
	okTypes map[trillian.TreeType]bool
}

// These rules define the permissible combinations of tree state and type
// for each operation type.
var rules = map[OpType]accessRule{
	Unknown: {},
	Admin: {
		okStates: map[trillian.TreeState]bool{
			trillian.TreeState_UNKNOWN_TREE_STATE: true,
			trillian.TreeState_ACTIVE:             true,
			trillian.TreeState_DRAINING:           true,
			trillian.TreeState_FROZEN:             true,
		},
		okTypes: map[trillian.TreeType]bool{
			trillian.TreeType_LOG:            true,
			trillian.TreeType_MAP:            true,
			trillian.TreeType_PREORDERED_LOG: true,
		},
	},
	Query: {
		okStates: map[trillian.TreeState]bool{
			// Have to allow queries on unknown state so storage can get a chance
			// to return ErrTreeNeedsInit.
			trillian.TreeState_UNKNOWN_TREE_STATE: true,
			trillian.TreeState_ACTIVE:             true,
			trillian.TreeState_DRAINING:           true,
			trillian.TreeState_FROZEN:             true,
		},
		okTypes: map[trillian.TreeType]bool{
			trillian.TreeType_LOG:            true,
			trillian.TreeType_MAP:            true,
			trillian.TreeType_PREORDERED_LOG: true,
		},
	},
	QueueLog: {
		okStates: map[trillian.TreeState]bool{
			trillian.TreeState_ACTIVE: true,
		},
		rejectCodes: map[trillian.TreeState]codes.Code{
			trillian.TreeState_DRAINING: codes.PermissionDenied,
			trillian.TreeState_FROZEN:   codes.PermissionDenied,
		},
		okTypes: map[trillian.TreeType]bool{
			trillian.TreeType_LOG:            true,
			trillian.TreeType_PREORDERED_LOG: true,
		},
	},
	SequenceLog: {
		okStates: map[trillian.TreeState]bool{
			trillian.TreeState_ACTIVE:   true,
			trillian.TreeState_DRAINING: true,
		},
		okTypes: map[trillian.TreeType]bool{
			trillian.TreeType_LOG:            true,
			trillian.TreeType_PREORDERED_LOG: true,
		},
		rejectCodes: map[trillian.TreeState]codes.Code{
			trillian.TreeState_FROZEN: codes.PermissionDenied,
		},
	},
	UpdateMap: {
		okStates: map[trillian.TreeState]bool{
			trillian.TreeState_ACTIVE: true,
		},
		okTypes: map[trillian.TreeType]bool{
			trillian.TreeType_MAP: true,
		},
	},
}

// NewContext returns a ctx with the given tree.
func NewContext(ctx context.Context, tree *trillian.Tree) context.Context {
	return context.WithValue(ctx, treeKey{}, tree)
}

// FromContext returns the tree within ctx if present, together with an indication of whether a
// tree was present.
func FromContext(ctx context.Context) (*trillian.Tree, bool) {
	tree, ok := ctx.Value(treeKey{}).(*trillian.Tree)
	return tree, ok && tree != nil
}

func validate(o GetOpts, tree *trillian.Tree) error {
	// Do the special case checks first
	if len(o.TreeTypes) > 0 && !o.TreeTypes[tree.TreeType] {
		return status.Errorf(codes.InvalidArgument, "operation not allowed for %s-type trees (wanted one of %v)", tree.TreeType, o.TreeTypes)
	}

	// Reject any operation types we don't know about.
	rule, ok := rules[o.Operation]
	if !ok {
		return status.Errorf(codes.Internal, "invalid operation type in GetOpts: %v", o)
	}

	// Apply the rule, ensure it allows the tree type and state that we have.
	if !rule.okTypes[tree.TreeType] || !rule.okStates[tree.TreeState] {
		// If we have a status code to use it takes precedence, otherwise it's
		// a generic InvalidArgument code.
		code, ok := rule.rejectCodes[tree.TreeState]
		if !ok {
			code = codes.InvalidArgument
		}
		return status.Errorf(code, "operation: %v not allowed for tree type: %v state: %v", o.Operation, tree.TreeType, tree.TreeState)
	}

	return nil
}

// GetTree returns the specified tree, either from the ctx (if present) or read from storage.
// The tree will be validated according to GetOpts before returned. Tree state is also considered
// (for example, deleted tree will return NotFound errors).
func GetTree(ctx context.Context, s storage.AdminStorage, treeID int64, opts GetOpts) (*trillian.Tree, error) {
	ctx, spanEnd := spanFor(ctx, "GetTree")
	defer spanEnd()
	tree, ok := FromContext(ctx)
	if !ok {
		var err error
		tree, err = storage.GetTree(ctx, s, treeID)
		if err != nil {
			return nil, err
		}
	}
	if tree.TreeId != treeID {
		// No operations should span multiple trees. If a tree is already in the context
		// it had better be the one that we want. If the tree comes back from the DB with
		// the wrong ID then this checks that too.
		return nil, status.Errorf(codes.Internal, "got tree %v, want %v", tree.TreeId, treeID)
	}

	if err := validate(opts, tree); err != nil {
		return nil, err
	}
	if tree.Deleted {
		return nil, status.Errorf(codes.NotFound, "tree %v not found", tree.TreeId)
	}

	return tree, nil
}

// Hash returns the crypto.Hash configured by the tree.
func Hash(tree *trillian.Tree) (crypto.Hash, error) {
	switch tree.HashAlgorithm {
	case sigpb.DigitallySigned_SHA256:
		return crypto.SHA256, nil
	}
	// There's no nil-like value for crypto.Hash, something has to be returned.
	return crypto.SHA256, fmt.Errorf("unexpected hash algorithm: %s", tree.HashAlgorithm)
}

// Signer returns a Trillian crypto.Signer configured by the tree.
func Signer(ctx context.Context, tree *trillian.Tree) (*tcrypto.Signer, error) {
	if tree.SignatureAlgorithm == sigpb.DigitallySigned_ANONYMOUS {
		return nil, fmt.Errorf("signature algorithm not supported: %s", tree.SignatureAlgorithm)
	}

	hash, err := Hash(tree)
	if err != nil {
		return nil, err
	}

	var keyProto ptypes.DynamicAny
	if err := ptypes.UnmarshalAny(tree.PrivateKey, &keyProto); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tree.PrivateKey: %v", err)
	}

	signer, err := keys.NewSigner(ctx, keyProto.Message)
	if err != nil {
		return nil, err
	}

	if tcrypto.SignatureAlgorithm(signer.Public()) != tree.SignatureAlgorithm {
		return nil, fmt.Errorf("%s signature not supported by signer of type %T", tree.SignatureAlgorithm, signer)
	}

	return tcrypto.NewSigner(tree.GetTreeId(), signer, hash), nil
}

func spanFor(ctx context.Context, name string) (context.Context, func()) {
	return monitoring.StartSpan(ctx, fmt.Sprintf("%s.%s", traceSpanRoot, name))
}
