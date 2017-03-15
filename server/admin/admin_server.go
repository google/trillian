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
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

var errNotImplemented = grpc.Errorf(codes.Unimplemented, "not implemented")

// Server is an implementation of trillian.TrillianAdminServer.
type Server struct {
	registry extension.Registry
}

// New returns a trillian.TrillianAdminServer implementation.
func New(registry extension.Registry) *Server {
	return &Server{registry}
}

func (s *Server) ListTrees(context.Context, *trillian.ListTreesRequest) (*trillian.ListTreesResponse, error) {
	return nil, errNotImplemented
}

func (s *Server) GetTree(ctx context.Context, request *trillian.GetTreeRequest) (*trillian.Tree, error) {
	storage := s.registry.GetAdminStorage()
	tx, err := storage.Snapshot(ctx)
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
	return tree, nil
}

func (s *Server) CreateTree(ctx context.Context, request *trillian.CreateTreeRequest) (*trillian.Tree, error) {
	storage := s.registry.GetAdminStorage()
	tx, err := storage.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Close()
	tree, err := tx.CreateTree(ctx, request.GetTree())
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return tree, nil
}

func (s *Server) UpdateTree(context.Context, *trillian.UpdateTreeRequest) (*trillian.Tree, error) {
	return nil, errNotImplemented
}

func (s *Server) DeleteTree(context.Context, *trillian.DeleteTreeRequest) (*empty.Empty, error) {
	return nil, errNotImplemented
}
