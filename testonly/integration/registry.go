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

package integration

import (
	"context"

	"github.com/google/trillian/extension"
	"github.com/google/trillian/quota/mysqlqm"
	"github.com/google/trillian/storage/mysql"
	"github.com/google/trillian/storage/testdb"
)

// NewRegistryForTests returns an extension.Registry for integration tests.
// Callers should call the returned cleanup function when they're finished
// with the registry and its contents.
func NewRegistryForTests(ctx context.Context, driver testdb.TestDBDriverName) (extension.Registry, func(context.Context), error) {
	db, done, err := testdb.NewTrillianDB(ctx, driver)
	if err != nil {
		return extension.Registry{}, nil, err
	}

	return extension.Registry{
		AdminStorage: mysql.NewAdminStorage(db),
		LogStorage:   mysql.NewLogStorage(db, nil),
		QuotaManager: &mysqlqm.QuotaManager{DB: db, MaxUnsequencedRows: mysqlqm.DefaultMaxUnsequenced},
	}, done, nil
}
