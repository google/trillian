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

// Package quotaapi provides a Quota admin server implementation.
package quotaapi

import (
	"github.com/coreos/etcd/clientv3"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/trillian/quota/etcd/quotapb"
	"github.com/google/trillian/quota/etcd/storage"
	"github.com/google/trillian/quota/etcd/storagepb"
	"golang.org/x/net/context"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var errNotImplemented = status.Error(codes.Unimplemented, "not implemented")

// Server is a quotapb.QuotaServer implementation backed by etcd.
type Server struct {
	qs *storage.QuotaStorage
}

// NewServer returns a new Server instance backed by client.
func NewServer(client *clientv3.Client) *Server {
	return &Server{qs: &storage.QuotaStorage{Client: client}}
}

// CreateConfig implements quotapb.QuotaServer.CreateConfig.
func (s *Server) CreateConfig(ctx context.Context, req *quotapb.CreateConfigRequest) (*quotapb.Config, error) {
	switch {
	case req.Name == "":
		return nil, status.Errorf(codes.InvalidArgument, "name is required")
	case req.Config == nil:
		return nil, status.Errorf(codes.InvalidArgument, "config is required")
	}
	req.Config.Name = req.Name

	var alreadyExists bool
	updated, err := s.qs.UpdateConfigs(ctx, false /* reset */, func(cfgs *storagepb.Configs) {
		if _, alreadyExists = findByName(req.Name, cfgs); alreadyExists {
			return
		}
		cfgs.Configs = append(cfgs.Configs, convertToStorage(req.Config))
	})
	switch {
	case alreadyExists:
		return nil, status.Errorf(codes.AlreadyExists, "%q already exists", req.Name)
	case err != nil:
		return nil, err
	}
	return getConfig(req.Name, updated, codes.Internal)
}

// DeleteConfig implements quotapb.QuotaServer.DeleteConfig.
func (s *Server) DeleteConfig(ctx context.Context, req *quotapb.DeleteConfigRequest) (*empty.Empty, error) {
	return nil, errNotImplemented
}

// GetConfig implements quotapb.QuotaServer.GetConfig.
func (s *Server) GetConfig(ctx context.Context, req *quotapb.GetConfigRequest) (*quotapb.Config, error) {
	if req.Name == "" {
		return nil, status.Errorf(codes.InvalidArgument, "name is required")
	}

	cfgs, err := s.qs.Configs(ctx)
	if err != nil {
		return nil, err
	}
	return getConfig(req.Name, cfgs, codes.NotFound)
}

// getConfig finds the Config named "name" on "cfgs", converts it to API and returns it.
// If the config cannot be found an error with code "code" is returned.
func getConfig(name string, cfgs *storagepb.Configs, code codes.Code) (*quotapb.Config, error) {
	cfg, ok := findByName(name, cfgs)
	if !ok {
		return nil, status.Errorf(code, "%q not found", name)
	}
	return convertToAPI(cfg), nil
}

// ListConfigs implements quotapb.QuotaServer.ListConfigs.
func (s *Server) ListConfigs(ctx context.Context, req *quotapb.ListConfigsRequest) (*quotapb.ListConfigsResponse, error) {
	return nil, errNotImplemented
}

// UpdateConfig implements quotapb.QuotaServer.UpdateConfig.
func (s *Server) UpdateConfig(ctx context.Context, req *quotapb.UpdateConfigRequest) (*quotapb.Config, error) {
	cfg, mask := req.Config, req.UpdateMask
	hasConfig := cfg != nil
	hasMask := mask != nil && len(mask.Paths) > 0
	switch {
	case req.Name == "":
		return nil, status.Errorf(codes.InvalidArgument, "name must be specified")
	case req.ResetQuota && !hasConfig && !hasMask:
		// For convenience, reset-only requests are allowed.
		cfg = &quotapb.Config{}
		mask = &field_mask.FieldMask{}
	case !hasConfig:
		return nil, status.Errorf(codes.InvalidArgument, "config must be specified")
	case !hasMask:
		return nil, status.Errorf(codes.InvalidArgument, "update_mask must be specified")
	}
	if err := validateMask(mask); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid update_mask: %v", err)
	}

	var notFound bool
	updated, err := s.qs.UpdateConfigs(ctx, req.ResetQuota, func(cfgs *storagepb.Configs) {
		existingCfg, ok := findByName(req.Name, cfgs)
		if !ok {
			notFound = true
			return
		}
		applyMask(cfg, existingCfg, mask)
	})
	switch {
	case notFound:
		return nil, status.Errorf(codes.NotFound, "%q not found", req.Name)
	case err != nil:
		return nil, err
	}
	return getConfig(req.Name, updated, codes.Internal)
}

func findByName(name string, cfgs *storagepb.Configs) (*storagepb.Config, bool) {
	for _, cfg := range cfgs.Configs {
		if cfg.Name == name {
			return cfg, true
		}
	}
	return nil, false
}
