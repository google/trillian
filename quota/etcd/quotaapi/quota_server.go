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

// Package quotapi provides a Quota admin server implementation.
package quotaapi

import (
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/trillian/quota/etcd/quotapb"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var errNotImplemented = status.Error(codes.Unimplemented, "not implemented")

// Server is a quotapb.QuotaServer implementation.
type Server struct{}

// CreateConfig implements quotapb.QuotaServer.CreateConfig.
func (s *Server) CreateConfig(ctx context.Context, req *quotapb.CreateConfigRequest) (*quotapb.Config, error) {
	return nil, errNotImplemented
}

// DeleteConfig implements quotapb.QuotaServer.DeleteConfig.
func (s *Server) DeleteConfig(ctx context.Context, req *quotapb.DeleteConfigRequest) (*empty.Empty, error) {
	return nil, errNotImplemented
}

// GetConfig implements quotapb.QuotaServer.GetConfig.
func (s *Server) GetConfig(ctx context.Context, req *quotapb.GetConfigRequest) (*quotapb.Config, error) {
	return nil, errNotImplemented
}

// ListConfig implements quotapb.QuotaServer.ListConfig.
func (s *Server) ListConfig(ctx context.Context, req *quotapb.ListConfigRequest) (*quotapb.ListConfigResponse, error) {
	return nil, errNotImplemented
}

// UpdateConfig implements quotapb.QuotaServer.UpdateConfig.
func (s *Server) UpdateConfig(ctx context.Context, req *quotapb.UpdateConfigRequest) (*quotapb.Config, error) {
	return nil, errNotImplemented
}
