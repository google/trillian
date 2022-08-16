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
	"github.com/google/trillian/storage/mysql"
	"k8s.io/klog/v2"
)

// QuotaManagerName identifies the MySQL quota implementation.
const QuotaManagerName = "mysql"

var maxUnsequencedRows = flag.Int("max_unsequenced_rows", DefaultMaxUnsequenced, "Max number of unsequenced rows before rate limiting kicks in. "+
	"Only effective for quota_system=mysql.")

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
	klog.Info("Using MySQL QuotaManager")
	return qm, nil
}
