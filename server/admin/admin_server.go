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

package admin

import (
	"bytes"
	"context"
	"fmt"

	"github.com/golang/glog"
	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys/der"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/merkle/hashers/registry"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/trees"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server is an implementation of trillian.TrillianAdminServer.
type Server struct {
	registry         extension.Registry
	allowedTreeTypes []trillian.TreeType
}

// New returns a trillian.TrillianAdminServer implementation.
// registry is the extension.Registry used by the Server.
// allowedTreeTypes defines which tree types may be created through this server,
// with nil meaning unrestricted.
func New(registry extension.Registry, allowedTreeTypes []trillian.TreeType) *Server {
	return &Server{
		registry:         registry,
		allowedTreeTypes: allowedTreeTypes,
	}
}

// IsHealthy returns nil if the server is healthy, error otherwise.
// TODO(Martin2112): This method (and the one in the log server) should probably have ctx as a param
func (s *Server) IsHealthy() error {
	return s.registry.AdminStorage.CheckDatabaseAccessible(context.Background())
}

// ListTrees implements trillian.TrillianAdminServer.ListTrees.
func (s *Server) ListTrees(ctx context.Context, req *trillian.ListTreesRequest) (*trillian.ListTreesResponse, error) {
	// TODO(codingllama): This needs access control
	resp, err := storage.ListTrees(ctx, s.registry.AdminStorage, req.GetShowDeleted())
	if err != nil {
		return nil, err
	}
	for _, tree := range resp {
		redact(tree)
	}
	return &trillian.ListTreesResponse{Tree: resp}, nil
}

// GetTree implements trillian.TrillianAdminServer.GetTree.
func (s *Server) GetTree(ctx context.Context, req *trillian.GetTreeRequest) (*trillian.Tree, error) {
	tree, err := storage.GetTree(ctx, s.registry.AdminStorage, req.GetTreeId())
	if err != nil {
		return nil, err
	}
	return redact(tree), nil
}

// CreateTree implements trillian.TrillianAdminServer.CreateTree.
func (s *Server) CreateTree(ctx context.Context, req *trillian.CreateTreeRequest) (*trillian.Tree, error) {
	tree := req.GetTree()
	if tree == nil {
		return nil, status.Errorf(codes.InvalidArgument, "a tree is required")
	}
	if err := s.validateAllowedTreeType(tree.TreeType); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	switch tree.TreeType {
	case trillian.TreeType_LOG, trillian.TreeType_PREORDERED_LOG:
		if _, err := registry.NewLogHasher(tree.HashStrategy); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "failed to create hasher for tree: %v", err.Error())
		}
	default:
		return nil, status.Errorf(codes.InvalidArgument, "invalid tree type: %v", tree.TreeType)
	}

	// If a key specification was provided, generate a new key.
	if req.KeySpec != nil {
		if tree.PrivateKey != nil {
			return nil, status.Errorf(codes.InvalidArgument, "the tree.private_key and key_spec fields are mutually exclusive")
		}
		if tree.PublicKey != nil {
			return nil, status.Errorf(codes.InvalidArgument, "the tree.public_key and key_spec fields are mutually exclusive")
		}
		if s.registry.NewKeyProto == nil {
			return nil, status.Errorf(codes.FailedPrecondition, "key generation is not enabled")
		}

		keyProto, err := s.registry.NewKeyProto(ctx, req.KeySpec)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "failed to generate private key: %v", err.Error())
		}

		tree.PrivateKey, err = ptypes.MarshalAny(keyProto)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to marshal private key: %v", err.Error())
		}
	}

	if tree.PrivateKey == nil {
		return nil, status.Errorf(codes.InvalidArgument, "tree.private_key or key_spec is required")
	}

	// Check that the tree.PrivateKey is valid by trying to get a signer.
	signer, err := trees.Signer(ctx, tree)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to create signer for tree: %v", err.Error())
	}

	// Derive the public key that corresponds to the private key for this tree.
	// The caller may have provided the public key, but for safety we shouldn't rely on it being correct.
	publicKey, err := der.ToPublicProto(signer.Public())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to marshal public key: %v", err.Error())
	}

	// If a public key was provided, check that it matches the one we derived. If it doesn't, this indicates a mistake by the caller.
	if tree.PublicKey != nil && !bytes.Equal(tree.PublicKey.Der, publicKey.Der) {
		return nil, status.Error(codes.InvalidArgument, "the public and private keys are not a pair")
	}

	// If no public key was provided, use the DER that we just marshaled.
	if tree.PublicKey == nil {
		tree.PublicKey = publicKey
	}

	// Clear generated fields, storage must set those
	tree.TreeId = 0
	tree.CreateTime = nil
	tree.UpdateTime = nil
	tree.Deleted = false
	tree.DeleteTime = nil

	createdTree, err := storage.CreateTree(ctx, s.registry.AdminStorage, tree)
	if err != nil {
		return nil, err
	}
	return redact(createdTree), nil
}

func (s *Server) validateAllowedTreeType(tt trillian.TreeType) error {
	if s.allowedTreeTypes == nil {
		return nil // All types OK
	}
	for _, allowedType := range s.allowedTreeTypes {
		if tt == allowedType {
			return nil
		}
	}
	return fmt.Errorf("tree type %s not allowed by this server", tt)
}

// UpdateTree implements trillian.TrillianAdminServer.UpdateTree.
func (s *Server) UpdateTree(ctx context.Context, req *trillian.UpdateTreeRequest) (*trillian.Tree, error) {
	tree := req.GetTree()
	mask := req.GetUpdateMask()
	if tree == nil {
		return nil, status.Errorf(codes.InvalidArgument, "a tree is required")
	}
	// Apply the mask to a couple of empty trees just to check that the paths are correct.
	if err := applyUpdateMask(&trillian.Tree{}, &trillian.Tree{}, mask); err != nil {
		return nil, err
	}

	updatedTree, err := storage.UpdateTree(ctx, s.registry.AdminStorage, tree.TreeId, func(other *trillian.Tree) {
		if err := applyUpdateMask(tree, other, mask); err != nil {
			// Should never happen (famous last words).
			glog.Errorf("Error applying mask on tree update: %v", err)
		}
	})
	if err != nil {
		return nil, err
	}
	return redact(updatedTree), nil
}

func applyUpdateMask(from, to *trillian.Tree, mask *field_mask.FieldMask) error {
	if mask == nil || len(mask.Paths) == 0 {
		return status.Errorf(codes.InvalidArgument, "an update_mask is required")
	}
	for _, path := range mask.Paths {
		switch path {
		case "tree_state":
			to.TreeState = from.TreeState
		case "tree_type":
			to.TreeType = from.TreeType
		case "display_name":
			to.DisplayName = from.DisplayName
		case "description":
			to.Description = from.Description
		case "storage_settings":
			to.StorageSettings = from.StorageSettings
		case "max_root_duration":
			to.MaxRootDuration = from.MaxRootDuration
		case "private_key":
			to.PrivateKey = from.PrivateKey
		default:
			return status.Errorf(codes.InvalidArgument, "invalid update_mask path: %q", path)
		}
	}
	return nil
}

// DeleteTree implements trillian.TrillianAdminServer.DeleteTree.
func (s *Server) DeleteTree(ctx context.Context, req *trillian.DeleteTreeRequest) (*trillian.Tree, error) {
	tree, err := storage.SoftDeleteTree(ctx, s.registry.AdminStorage, req.GetTreeId())
	if err != nil {
		return nil, err
	}
	return redact(tree), nil
}

// UndeleteTree implements trillian.TrillianAdminServer.UndeleteTree.
func (s *Server) UndeleteTree(ctx context.Context, req *trillian.UndeleteTreeRequest) (*trillian.Tree, error) {
	tree, err := storage.UndeleteTree(ctx, s.registry.AdminStorage, req.GetTreeId())
	if err != nil {
		return nil, err
	}
	return redact(tree), nil
}

// redact removes sensitive information from t. Returns t for convenience.
func redact(t *trillian.Tree) *trillian.Tree {
	t.PrivateKey = nil
	return t
}
