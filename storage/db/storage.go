// Copyright 2016 Google Inc. All Rights Reserved.
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

package db

import (
	"database/sql"
	"fmt"

	"github.com/golang/glog"
	"github.com/google/trillian/storage/mysql"
	"github.com/google/trillian/storage/wrapper"
)

// OpenDB opens, and creates a wrapper for, the SQL databases that we know about. The wrapper
// encapsulates things that differ between database implementations and abstracts away
// from the SQL queries.
func OpenDB(driver, dbURL string) (wrapper.DBWrapper, error) {
	db, err := sql.Open(driver, dbURL)
	if err != nil {
		// Don't log uri as it could contain credentials
		glog.Warningf("Could not open database, check config: %s", err)
		return nil, err
	}

	switch driver {
	case "mysql":
		wrapper := mysql.NewWrapper(db)
		if err := wrapper.OnOpenDB(); err != nil {
			return nil, err
		}
		return wrapper, nil

	default:
		return nil, fmt.Errorf("unknown database driver: '%s' cannot be used", driver)
	}
}
