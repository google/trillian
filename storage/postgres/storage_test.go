// Copyright 2018 Google Inc. All Rights Reserved.
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
package postgres

import (
	"context"
	"database/sql"
	"flag"
	"os"
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian/storage/postgres/testdb"
)

// db is shared throughout all postgres tests
var db *sql.DB

func TestMain(m *testing.M) {
	flag.Parse()
	ec := 0
	defer func() { os.Exit(ec) }()
	if !testdb.PGAvailable() {
		glog.Errorf("PG not available, skipping all PG storage tests")
		ec = 1
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(time.Second*30))
	defer cancel()
	db = testdb.NewTrillianDBOrDie(ctx)
	defer db.Close()
	ec = m.Run()
}
