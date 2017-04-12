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
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/trillian"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/trees"
	"golang.org/x/net/context"
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
	tl, err := tx.ListTrees(ctx)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	for _, tree := range tl {
		redact(tree)
	}
	return &trillian.ListTreesResponse{Tree: tl}, nil
}

// GetTree implements trillian.TrillianAdminServer.GetTree.
func (s *Server) GetTree(ctx context.Context, request *trillian.GetTreeRequest) (*trillian.Tree, error) {
	tx, err := s.registry.AdminStorage.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Close()
	// TODO(codingllama): This needs access control
	tree, err := tx.GetTree(ctx, request.GetTreeId())
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
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
	if _, err := trees.Signer(ctx, s.registry.SignerFactory, tree); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to create signer for tree: %v", err.Error())
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
func (s *Server) UpdateTree(context.Context, *trillian.UpdateTreeRequest) (*trillian.Tree, error) {
	// TODO(codingllama): Don't forget to redact tree
	return nil, errNotImplemented
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
