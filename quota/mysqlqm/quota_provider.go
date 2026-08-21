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

package mysqlqm

import (
	"flag"

	"github.com/google/trillian/quota"
	"github.com/google/trillian/quota/cacheqm"
	"github.com/google/trillian/storage/mysql"
	"k8s.io/klog/v2"
)

// QuotaManagerName identifies the MySQL quota implementation.
const QuotaManagerName = "mysql"

var (
	maxUnsequencedRows = flag.Int("max_unsequenced_rows", DefaultMaxUnsequenced, "Max number of unsequenced rows before rate limiting kicks in. "+
		"Only effective for quota_system=mysql.")
	mysqlQuotaMinBatchSize = flag.Int("mysql_quota_min_batch_size", 0, "Minimum number of tokens to request from the MySQL quota system. "+
		"Zero or lower disables batching. Only effective for quota_system=mysql.")
	mysqlQuotaMaxCacheEntries = flag.Int("mysql_quota_max_cache_entries", cacheqm.DefaultMaxCacheEntries, "Maximum number of quota specs in the MySQL quota cache. "+
		"Only effective when mysql_quota_min_batch_size is positive.")
)

func init() {
	if err := quota.RegisterProvider(QuotaManagerName, newMySQLQuotaManager); err != nil {
		klog.Fatalf("Failed to register quota manager %v: %v", QuotaManagerName, err)
	}
}

func newMySQLQuotaManager() (quota.Manager, error) {
	db, err := mysql.GetDatabase()
	if err != nil {
		return nil, err
	}
	qm := &QuotaManager{
		DB:                 db,
		MaxUnsequencedRows: *maxUnsequencedRows,
	}
	if *mysqlQuotaMinBatchSize > 0 {
		if *mysqlQuotaMinBatchSize >= *maxUnsequencedRows {
			return nil, fmt.Errorf("mysql_quota_min_batch_size (%d) must be less than max_unsequenced_rows (%d)", *mysqlQuotaMinBatchSize, *maxUnsequencedRows)
		}
		cachedqm, err := cacheqm.NewCachedManager(qm, *mysqlQuotaMinBatchSize, *mysqlQuotaMaxCacheEntries)
		if err != nil {
			return nil, err
		}
		klog.Infof("Using cached MySQL QuotaManager with minimum batch size %d", *mysqlQuotaMinBatchSize)
		return cachedqm, nil
	}
	klog.Info("Using MySQL QuotaManager")
	return qm, nil
}
