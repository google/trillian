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

func (m *manager) GetUser(ctx context.Context, req interface{}) string {
	return "" // Unused
}

func (m *manager) GetTokens(ctx context.Context, numTokens int, specs []quota.Spec) error {
	names, err := configNames(specs)
	if err != nil {
		return err
	}
	return m.qs.Get(ctx, names, int64(numTokens))
}

func (m *manager) PeekTokens(ctx context.Context, specs []quota.Spec) (map[quota.Spec]int, error) {
	names, err := configNames(specs)
	if err != nil {
		return nil, err
	}
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

func (m *manager) PutTokens(ctx context.Context, numTokens int, specs []quota.Spec) error {
	names, err := configNames(specs)
	if err != nil {
		return err
	}
	return m.qs.Put(ctx, names, int64(numTokens))
}

func (m *manager) ResetQuota(ctx context.Context, specs []quota.Spec) error {
	names, err := configNames(specs)
	if err != nil {
		return err
	}
	return m.qs.Reset(ctx, names)
}

func configNames(specs []quota.Spec) ([]string, error) {
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		name, err := configName(spec)
		if err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, nil
}

func configName(spec quota.Spec) (string, error) {
	// TODO(codingllama): Use spec.Name() when available.
	var group string
	var user string
	switch spec.Group {
	case quota.Global:
		group = "global"
	case quota.Tree:
		group = "trees"
		user = fmt.Sprint(spec.TreeID)
	case quota.User:
		group = "users"
		user = spec.User
	default:
		return "", fmt.Errorf("unexpected group: %v", spec.Group)
	}

	var kind string
	switch spec.Kind {
	case quota.Read:
		kind = "read"
	case quota.Write:
		kind = "write"
	default:
		return "", fmt.Errorf("unexpected kind: %v", spec.Kind)
	}

	if spec.Group == quota.Global {
		return fmt.Sprintf("quotas/%v/%v/config", group, kind), nil
	}
	return fmt.Sprintf("quotas/%v/%v/%v/config", group, user, kind), nil
}
