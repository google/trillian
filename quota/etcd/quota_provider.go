// Copyright 2018 Google LLC. All Rights Reserved.
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

// Package etcd provides the configuration and initialization of the etcd
// quota manager.
package etcd

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/google/trillian/quota"
	"github.com/google/trillian/quota/cacheqm"
	"github.com/google/trillian/quota/etcd/etcdqm"
	clientv3 "go.etcd.io/etcd/client/v3"
	"k8s.io/klog/v2"
)

// QuotaManagerName identifies the etcd quota implementation.
const QuotaManagerName = "etcd"

var (
	// Servers is a flag containing the address(es) of etcd servers
	Servers = flag.String("etcd_servers", "", "A comma-separated list of etcd servers; no etcd registration if empty")
	// TODO(Martin2112): suggested renaming these to etc_... to avoid clashes, but will it break existing deploys?
	quotaMinBatchSize = flag.Int("quota_min_batch_size", cacheqm.DefaultMinBatchSize, "Minimum number of tokens to request from the quota system. "+
		"Zero or lower means batching is disabled. Applicable for etcd quotas.")
	quotaMaxCacheEntries = flag.Int("quota_max_cache_entries", cacheqm.DefaultMaxCacheEntries, "Max number of quota specs in the quota cache. "+
		"Zero or lower means batching is disabled. Applicable for etcd quotas.")
)

func init() {
	if err := quota.RegisterProvider(QuotaManagerName, newEtcdQuotaManager); err != nil {
		klog.Fatalf("Failed to register quota manager %v: %v", QuotaManagerName, err)
	}
}

func newEtcdQuotaManager() (quota.Manager, error) {
	if *Servers == "" {
		return nil, fmt.Errorf("can't create etcd quotamanager - etcd_servers flag is unset")
	}
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   strings.Split(*Servers, ","),
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to etcd at %v: %v", *Servers, err)
	}

	var qm quota.Manager = etcdqm.New(client)
	if *quotaMinBatchSize > 0 && *quotaMaxCacheEntries > 0 {
		cachedQM, err := cacheqm.NewCachedManager(qm, *quotaMinBatchSize, *quotaMaxCacheEntries)
		if err != nil {
			return nil, err
		}
		qm = cachedQM
	}
	klog.Info("Using Etcd QuotaManager")
	return qm, nil
}
