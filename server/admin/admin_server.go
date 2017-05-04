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

package admin

import (
	"bytes"
	"crypto/x509"

	"github.com/golang/glog"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/trillian"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/trees"
	"golang.org/x/net/context"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var errNotImplemented = status.Errorf(codes.Unimplemented, "not implemented")

// Server is an implementation of trillian.TrillianAdminServer.
type Server struct {
	registry extension.Registry
}

// New returns a trillian.TrillianAdminServer implementation.
func New(registry extension.Registry) *Server {
	return &Server{registry}
}

// ListTrees implements trillian.TrillianAdminServer.ListTrees.
func (s *Server) ListTrees(ctx context.Context, req *trillian.ListTreesRequest) (*trillian.ListTreesResponse, error) {
	tx, err := s.registry.AdminStorage.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Close()
	// TODO(codingllama): This needs access control
	resp, err := tx.ListTrees(ctx)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	for _, tree := range resp {
		redact(tree)
	}
	return &trillian.ListTreesResponse{Tree: resp}, nil
}

// GetTree implements trillian.TrillianAdminServer.GetTree.
func (s *Server) GetTree(ctx context.Context, request *trillian.GetTreeRequest) (*trillian.Tree, error) {
	// TODO(codingllama): This needs access control
	tree, err := trees.GetTree(ctx, s.registry.AdminStorage, request.GetTreeId(), trees.GetOpts{Readonly: true})
	if err != nil {
		return nil, err
	}
	return redact(tree), nil
}

// CreateTree implements trillian.TrillianAdminServer.CreateTree.
func (s *Server) CreateTree(ctx context.Context, request *trillian.CreateTreeRequest) (*trillian.Tree, error) {
	tree := request.GetTree()
	if tree == nil {
		return nil, status.Errorf(codes.InvalidArgument, "a tree is required")
	}
	if _, err := trees.Hasher(tree); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to create hasher for tree: %v", err.Error())
	}
	signer, err := trees.Signer(ctx, s.registry.SignerFactory, tree)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to create signer for tree: %v", err.Error())
	}

	// Derive the public key that corresponds to the private key for this tree.
	// The caller may have provided the public key, but for safety we shouldn't rely on it being correct.
	publicKeyDER, err := x509.MarshalPKIXPublicKey(signer.Public())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to marshal public key: %v", err.Error())
	}
	// If a public key was provided, check that it matches the one we derived. If it doesn't, this indicates a mistake by the caller.
	if tree.PublicKey != nil && !bytes.Equal(tree.PublicKey.Der, publicKeyDER) {
		return nil, status.Error(codes.InvalidArgument, "the public and private keys are not a pair")
	}
	// If no public key was provided, use the DER that we just marshaled.
	if tree.PublicKey == nil {
		tree.PublicKey = &trillian.PublicKey{Der: publicKeyDER}
	}

	tx, err := s.registry.AdminStorage.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Close()
	newTree, err := tx.CreateTree(ctx, tree)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return redact(newTree), nil
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

	tx, err := s.registry.AdminStorage.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Close()
	updatedTree, err := tx.UpdateTree(ctx, tree.TreeId, func(other *trillian.Tree) {
		if err := applyUpdateMask(tree, other, mask); err != nil {
			// Should never happen (famous last words).
			glog.Errorf("Error applying mask on tree update: %v", err)
		}
	})
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
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
		case "display_name":
			to.DisplayName = from.DisplayName
		case "description":
			to.Description = from.Description
		case "storage_settings":
			to.StorageSettings = from.StorageSettings
		default:
			return status.Errorf(codes.InvalidArgument, "invalid update_mask path: %q", path)
		}
	}
	return nil
}

// DeleteTree implements trillian.TrillianAdminServer.DeleteTree.
func (s *Server) DeleteTree(context.Context, *trillian.DeleteTreeRequest) (*empty.Empty, error) {
	return nil, errNotImplemented
}

// redact removes sensitive information from t. Returns t for convenience.
func redact(t *trillian.Tree) *trillian.Tree {
	t.PrivateKey = nil
	return t
}
