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
	"context"

	"github.com/golang/glog"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/trillian/quota/etcd/quotapb"
	"github.com/google/trillian/quota/etcd/storage"
	"github.com/google/trillian/quota/etcd/storagepb"
	"go.etcd.io/etcd/clientv3"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
	if req.Config == nil {
		return nil, status.Errorf(codes.InvalidArgument, "config is required")
	}
	if err := validateName(req.Name); err != nil {
		return nil, err
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
	return s.getConfig(ctx, req.Name, updated, codes.Internal)
}

// DeleteConfig implements quotapb.QuotaServer.DeleteConfig.
func (s *Server) DeleteConfig(ctx context.Context, req *quotapb.DeleteConfigRequest) (*empty.Empty, error) {
	if err := validateName(req.Name); err != nil {
		return nil, err
	}

	notFound := false
	_, err := s.qs.UpdateConfigs(ctx, false /* reset */, func(cfgs *storagepb.Configs) {
		for i, cfg := range cfgs.Configs {
			if cfg.Name == req.Name {
				cfgs.Configs = append(cfgs.Configs[:i], cfgs.Configs[i+1:]...)
				return
			}
		}
		notFound = true
	})
	if notFound {
		return nil, status.Errorf(codes.NotFound, "%q not found", req.Name)
	}
	return &empty.Empty{}, err
}

// GetConfig implements quotapb.QuotaServer.GetConfig.
func (s *Server) GetConfig(ctx context.Context, req *quotapb.GetConfigRequest) (*quotapb.Config, error) {
	if err := validateName(req.Name); err != nil {
		return nil, err
	}

	cfgs, err := s.qs.Configs(ctx)
	if err != nil {
		return nil, err
	}
	return s.getConfig(ctx, req.Name, cfgs, codes.NotFound)
}

// getConfig finds the Config named "name" on "cfgs", converts it to API and returns it.
// If the config cannot be found an error with code "code" is returned.
func (s *Server) getConfig(ctx context.Context, name string, cfgs *storagepb.Configs, code codes.Code) (*quotapb.Config, error) {
	storedCfg, ok := findByName(name, cfgs)
	if !ok {
		return nil, status.Errorf(code, "%q not found", name)
	}
	cfg := convertToAPI(storedCfg)

	tokens, err := s.qs.Peek(ctx, []string{cfg.Name})
	if err == nil {
		cfg.CurrentTokens = tokens[cfg.Name]
	} else {
		glog.Warningf("Unexpected error peeking token count for %q: %v", cfg.Name, err)
	}

	return cfg, nil
}

// ListConfigs implements quotapb.QuotaServer.ListConfigs.
func (s *Server) ListConfigs(ctx context.Context, req *quotapb.ListConfigsRequest) (*quotapb.ListConfigsResponse, error) {
	nfs := make([]nameFilter, 0, len(req.Names))
	for _, name := range req.Names {
		nf, err := newNameFilter(name)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		nfs = append(nfs, nf)
	}

	cfgs, err := s.qs.Configs(ctx)
	if err != nil {
		return nil, err
	}
	resp := &quotapb.ListConfigsResponse{}
	for _, cfg := range cfgs.Configs {
		if listMatches(nfs, cfg) {
			resp.Configs = append(resp.Configs, listView(req.View, cfg))
		}
	}

	// Peek token counts for FULL view
	if req.View == quotapb.ListConfigsRequest_FULL {
		names := make([]string, 0, len(resp.Configs))
		for _, cfg := range resp.Configs {
			names = append(names, cfg.Name)
		}
		tokens, err := s.qs.Peek(ctx, names)
		if err == nil {
			for _, cfg := range resp.Configs {
				cfg.CurrentTokens = tokens[cfg.Name]
			}
		} else {
			glog.Infof("Error peeking token counts for %v: %v", names, err)
		}
	}
	return resp, nil
}

func listMatches(nfs []nameFilter, cfg *storagepb.Config) bool {
	if len(nfs) == 0 {
		return true // Match all
	}
	for _, nf := range nfs {
		if nf.matches(cfg.Name) {
			return true
		}
	}
	return false
}

func listView(view quotapb.ListConfigsRequest_ListView, src *storagepb.Config) *quotapb.Config {
	switch view {
	case quotapb.ListConfigsRequest_BASIC:
		return &quotapb.Config{Name: src.Name}
	default:
		return convertToAPI(src)
	}
}

// UpdateConfig implements quotapb.QuotaServer.UpdateConfig.
func (s *Server) UpdateConfig(ctx context.Context, req *quotapb.UpdateConfigRequest) (*quotapb.Config, error) {
	cfg, mask := req.Config, req.UpdateMask
	hasConfig := cfg != nil
	hasMask := mask != nil && len(mask.Paths) > 0
	switch {
	case req.ResetQuota && !hasConfig && !hasMask:
		// For convenience, reset-only requests are allowed.
		cfg = &quotapb.Config{}
		mask = &field_mask.FieldMask{}
	case !hasConfig:
		return nil, status.Errorf(codes.InvalidArgument, "config must be specified")
	case !hasMask:
		return nil, status.Errorf(codes.InvalidArgument, "update_mask must be specified")
	}
	if err := validateName(req.Name); err != nil {
		return nil, err
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
	return s.getConfig(ctx, req.Name, updated, codes.Internal)
}

func findByName(name string, cfgs *storagepb.Configs) (*storagepb.Config, bool) {
	for _, cfg := range cfgs.Configs {
		if cfg.Name == name {
			return cfg, true
		}
	}
	return nil, false
}

func validateName(name string) error {
	switch {
	case name == "":
		return status.Errorf(codes.InvalidArgument, "name is required")
	case !storage.IsNameValid(name):
		return status.Errorf(codes.InvalidArgument, "invalid name: %q", name)
	}
	return nil
}
