// Copyright 2024 Trillian Authors. All Rights Reserved.
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

package postgresqlqm

import (
	"flag"

	"github.com/google/trillian/quota"
	"github.com/google/trillian/storage/postgresql"
	"k8s.io/klog/v2"
)

// QuotaManagerName identifies the PostgreSQL quota implementation.
const QuotaManagerName = "postgresql"

var maxUnsequencedRows = flag.Int("pg_max_unsequenced_rows", DefaultMaxUnsequenced, "Max number of unsequenced rows before rate limiting kicks in. "+
	"Only effective for quota_system=postgresql.")

func init() {
	if err := quota.RegisterProvider(QuotaManagerName, newPostgreSQLQuotaManager); err != nil {
		klog.Fatalf("Failed to register quota manager %v: %v", QuotaManagerName, err)
	}
}

func newPostgreSQLQuotaManager() (quota.Manager, error) {
	db, err := postgresql.GetDatabase()
	if err != nil {
		return nil, err
	}
	qm := &QuotaManager{
		DB:                 db,
		MaxUnsequencedRows: *maxUnsequencedRows,
	}
	klog.Info("Using PostgreSQL QuotaManager")
	return qm, nil
}
