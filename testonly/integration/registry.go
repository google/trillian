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

package integration

import (
	"context"

	"github.com/google/trillian/extension"
	"github.com/google/trillian/quota/mysqlqm"
	"github.com/google/trillian/storage/mysql"
	"github.com/google/trillian/storage/testdb"
)

// NewRegistryForTests returns an extension.Registry for integration tests.
func NewRegistryForTests(ctx context.Context) (extension.Registry, error) {
	db, err := testdb.NewTrillianDB(ctx)
	if err != nil {
		return extension.Registry{}, err
	}

	// *****************************************************************
	// TODO(Martin2112): Do something about hardcoded LogStorageOptions.
	// *****************************************************************
	return extension.Registry{
		AdminStorage: mysql.NewAdminStorage(db),
		LogStorage:   mysql.NewLogStorage(db, nil, mysql.LogStorageOptions{mysql.TreeStorageOptions{FetchSingleSubtrees: true}}),
		MapStorage:   mysql.NewMapStorage(db),
		QuotaManager: &mysqlqm.QuotaManager{DB: db, MaxUnsequencedRows: mysqlqm.DefaultMaxUnsequenced},
	}, nil
}
