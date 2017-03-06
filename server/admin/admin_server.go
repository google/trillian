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
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

var errNotImplemented = grpc.Errorf(codes.Unimplemented, "not implemented")

// adminServer is an implementation of trillian.TrillianAdminServer.
type adminServer struct{}

// New returns a trillian.TrillianAdminServer implementation.
func New() trillian.TrillianAdminServer {
	return &adminServer{}
}

func (s *adminServer) ListTrees(context.Context, *trillian.ListTreesRequest) (*trillian.ListTreesResponse, error) {
	return nil, errNotImplemented
}

func (s *adminServer) GetTree(context.Context, *trillian.GetTreeRequest) (*trillian.Tree, error) {
	return nil, errNotImplemented
}

func (s *adminServer) CreateTree(context.Context, *trillian.CreateTreeRequest) (*trillian.Tree, error) {
	return nil, errNotImplemented
}

func (s *adminServer) UpdateTree(context.Context, *trillian.UpdateTreeRequest) (*trillian.Tree, error) {
	return nil, errNotImplemented
}

func (s *adminServer) DeleteTree(context.Context, *trillian.DeleteTreeRequest) (*empty.Empty, error) {
	return nil, errNotImplemented
}
