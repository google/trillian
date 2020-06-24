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

package etcd

import (
	"strings"
	"time"

	"go.etcd.io/etcd/clientv3"
)

// NewClient returns an etcd Client connecting to the passed in servers'
// endpoints, with the specified dialing timeout.
//
// The return type belongs to etcd package in Trillian vendor/ directory, which
// allows external clients/codebases to build an object that matches the
// Trillian internal implementation (a clientv3.Client built from a different
// codebase/location, even if it's the same code, wouldn't have the required
// matching type).
//
// TODO(pavelkalinnikov): Remove this when there is a way to compatibly import
// the same version of etcd in external codebases. Could Go modules help?
func NewClient(endpoints []string, dialTimeout time.Duration) (*clientv3.Client, error) {
	return clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: dialTimeout,
	})
}

// NewClientFromString returns an etcd client, or nil if servers is empty.
// The servers parameter must be a comma-separated list of etcd server URIs.
func NewClientFromString(servers string) (*clientv3.Client, error) {
	if servers == "" {
		return nil, nil
	}
	return NewClient(strings.Split(servers, ","), 5*time.Second)
}
