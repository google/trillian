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
	"github.com/google/trillian/extension"
	mysqlq "github.com/google/trillian/quota/mysql"
	"github.com/google/trillian/storage/mysql"
)

// NewRegistryForTests returns an extension.Registry for integration tests.
// A new database will be recreated, as per GetTestDB.
func NewRegistryForTests(testID string) (extension.Registry, error) {
	db, err := GetTestDB(testID)
	if err != nil {
		return extension.Registry{}, err
	}

	return extension.Registry{
		AdminStorage: mysql.NewAdminStorage(db),
		LogStorage:   mysql.NewLogStorage(db, nil),
		MapStorage:   mysql.NewMapStorage(db),
		QuotaManager: &mysqlq.QuotaManager{DB: db, MaxUnsequencedRows: mysqlq.DefaultMaxUnsequenced},
	}, nil
}
