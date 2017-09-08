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

package server

import (
	"database/sql"
	"fmt"

	"github.com/coreos/etcd/clientv3"
	"github.com/golang/glog"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/quota/cacheqm"
	"github.com/google/trillian/quota/etcd/etcdqm"
	"github.com/google/trillian/quota/mysqlqm"
)

const (
	// QuotaNoop represents the noop quota implementation.
	QuotaNoop = "noop"

	// QuotaMySQL represents the MySQL quota implementation.
	QuotaMySQL = "mysql"

	// QuotaEtcd represents the etcd quota implementation.
	QuotaEtcd = "etcd"
)

// QuotaParams represents all parameters required to initialize a quota.Manager.
//
// Depending on the supplied QuotaSystem, the actual quota.Manager implementation, as returned by
// NewQuotaManager, may differ.
//
// See fields for details.
type QuotaParams struct {
	// QuotaSystem represents the underlying quota implementation used.
	// Valid values are "noop", "mysql" and "etcd".
	QuotaSystem string

	// DB is the database used by MySQL quotas.
	DB *sql.DB

	// MaxUnsequencedRows is the max number of rows that may be in Unsequenced before new write
	// requests are blocked.
	// Used by MySQL quotas.
	MaxUnsequencedRows int

	// Client is used by etcd quotas.
	Client *clientv3.Client

	// MinBatchSize and MaxCacheEntries are used for token caching.
	// Applicable to etcd quotas.
	MinBatchSize, MaxCacheEntries int
}

// NewQuotaManager returns a quota.Manager implementation according to params.
// See QuotaParams for details.
func NewQuotaManager(params *QuotaParams) (quota.Manager, error) {
	var qm quota.Manager
	switch params.QuotaSystem {
	case QuotaNoop:
		qm = quota.Noop()
	case QuotaMySQL:
		qm = &mysqlqm.QuotaManager{DB: params.DB, MaxUnsequencedRows: params.MaxUnsequencedRows}
	case QuotaEtcd:
		// Client is more likely to be nil than all other params, due to etcd being an optional
		// dependency in some cases.
		// As such, let's fail fast here if that's the case.
		if params.Client == nil {
			return nil, fmt.Errorf("etcd servers required for %v quota", params.QuotaSystem)
		}
		qm = etcdqm.New(params.Client)
	default:
		return nil, fmt.Errorf("unknown quota system: %v", params.QuotaSystem)
	}
	qmType := fmt.Sprintf("%T", qm)

	if params.QuotaSystem == QuotaEtcd && params.MinBatchSize > 0 && params.MaxCacheEntries > 0 {
		cachedQM, err := cacheqm.NewCachedManager(qm, params.MinBatchSize, params.MaxCacheEntries)
		if err != nil {
			return nil, err
		}
		qm = cachedQM
		qmType = fmt.Sprintf("%T/%v", qm, qmType)
	}

	glog.Infof("Using QuotaManager %v", qmType)
	return qm, nil
}
