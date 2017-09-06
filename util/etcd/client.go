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

package etcd

import (
	"strings"
	"time"

	"github.com/coreos/etcd/clientv3"
)

// NewClient returns an etcd client, or nil if servers is empty.
// The servers parameter should be a comma-separated list of etcd server URIs.
func NewClient(servers string) (*clientv3.Client, error) {
	if servers == "" {
		return nil, nil
	}
	return clientv3.New(clientv3.Config{
		Endpoints:   strings.Split(servers, ","),
		DialTimeout: 5 * time.Second,
	})
}
