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

// Package etcdqm contains an etcd-based quota.Manager implementation.
package etcdqm

import (
	"context"
	"fmt"

	"github.com/coreos/etcd/clientv3"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/quota/etcd/storage"
)

type manager struct {
	qs *storage.QuotaStorage
}

// New returns a new etcd-based quota.Manager.
func New(client *clientv3.Client) quota.Manager {
	return &manager{qs: &storage.QuotaStorage{Client: client}}
}

// GetTokens implements the quota.Manager API.
func (m *manager) GetTokens(ctx context.Context, numTokens int, specs []quota.Spec) error {
	return m.qs.Get(ctx, configNames(specs), int64(numTokens))
}

// PeekTokens implements the quota.Manager API.
func (m *manager) PeekTokens(ctx context.Context, specs []quota.Spec) (map[quota.Spec]int, error) {
	names := configNames(specs)
	nameToSpec := make(map[string]quota.Spec)
	for i, name := range names {
		nameToSpec[name] = specs[i]
	}

	nameToTokens, err := m.qs.Peek(ctx, names)
	if err != nil {
		return nil, err
	}

	tokens := make(map[quota.Spec]int)
	for k, v := range nameToTokens {
		tokens[nameToSpec[k]] = int(v)
	}
	return tokens, nil
}

// PutTokens implements the quota.Manager API.
func (m *manager) PutTokens(ctx context.Context, numTokens int, specs []quota.Spec) error {
	return m.qs.Put(ctx, configNames(specs), int64(numTokens))
}

// ResetQuota implements the quota.Manager API.
func (m *manager) ResetQuota(ctx context.Context, specs []quota.Spec) error {
	return m.qs.Reset(ctx, configNames(specs))
}

func configNames(specs []quota.Spec) []string {
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, configName(spec))
	}
	return names
}

func configName(spec quota.Spec) string {
	return fmt.Sprintf("quotas/%v/config", spec.Name())
}
