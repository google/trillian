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
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/storage/sql/coresql"
	"github.com/google/trillian/storage/sql/mysql"
)

// NewRegistryForTests returns an extension.Registry for integration tests.
// A new database will be recreated, as per GetTestDB.
func NewRegistryForTests(testID string) (extension.Registry, error) {
	db, err := GetTestDB(testID)
	if err != nil {
		return extension.Registry{}, err
	}
	wrap := mysql.NewWrapper(db)

	return extension.Registry{
		AdminStorage:  coresql.NewAdminStorage(wrap),
		SignerFactory: keys.PEMSignerFactory{},
		LogStorage:    coresql.NewLogStorage(wrap),
		MapStorage:    coresql.NewMapStorage(wrap),
		QuotaManager:  quota.Noop(),
	}, nil
}
