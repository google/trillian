// Copyright 2018 Google Inc. All Rights Reserved.
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
	"time"

	"github.com/coreos/etcd/clientv3"
)

// NewClient returns an etcd client connecting to the passed in servers'
// endpoints, with the specified dialing timeout.
//
// TODO(pavelkalinnikov): Remove this function, and create Client directly. At
// the moment we can't do this as there are external repos depending on this
// package, and it's not clear how they could import etcd package vendored into
// this repo. Maybe go modules could help when we switch to Go 1.11+.
func NewClient(endpoints []string, dialTimeout time.Duration) (*clientv3.Client, error) {
	return clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: dialTimeout,
	})
}
