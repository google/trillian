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
	"database/sql"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/google/trillian/testonly"
)

const (
	// createSQLFile is a relative path from the current package.
	createSQLFile = "../../storage/sql/mysql/storage.sql"
	mysqlRootURI  = "root@tcp(127.0.0.1:3306)/"
)

// GetTestDB drops and recreates the test database.
// Returns a database connection to the test database.
func GetTestDB(testID string) (*sql.DB, error) {
	dbName := fmt.Sprintf("test_%v", testID)
	testDBURI := fmt.Sprintf("root@tcp(127.0.0.1:3306)/%v", dbName)

	// Drop existing database.
	dbRoot, err := sql.Open("mysql", mysqlRootURI)
	if err != nil {
		return nil, err
	}
	defer dbRoot.Close()
	resetSQL := []string{
		fmt.Sprintf("DROP DATABASE IF EXISTS %v;", dbName),
		fmt.Sprintf("CREATE DATABASE %v;", dbName),
	}
	for _, sqlText := range resetSQL {
		if _, err := dbRoot.ExecContext(context.TODO(), sqlText); err != nil {
			return nil, err
		}
	}

	// Create new database.
	dbTest, err := sql.Open("mysql", testDBURI)
	if err != nil {
		return nil, err
	}

	createSQL, err := ioutil.ReadFile(testonly.RelativeToPackage(createSQLFile))
	if err != nil {
		return nil, err
	}
	sqlSlice := strings.Split(string(createSQL), ";\n")
	// Omit the last element of the slice, since it will be "".
	for _, sqlText := range sqlSlice[:len(sqlSlice)-1] {
		if _, err := dbTest.ExecContext(context.TODO(), sqlText); err != nil {
			return nil, err
		}
	}

	return dbTest, nil
}
