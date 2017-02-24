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

package cache

import (
	"database/sql"
	"sync"

	"github.com/golang/glog"
	"golang.org/x/net/context"
)

type StmtCache struct {
	// The database object this cache will generate statements for.
	db *sql.DB
	// Must hold the mutex before manipulating the statement map. Sharing a lock because
	// it only needs to be held while the statements are built, not while they execute and
	// this will be a short time. These maps are from the number of placeholder '?'
	// in the query to the statement that should be used.
	statementMutex *sync.Mutex
	statements     map[string]map[int]*sql.Stmt
}

// NewStmtCache creates a new statement cache associated with the specified database.
func NewStmtCache(db *sql.DB) *StmtCache {
	return &StmtCache{
		db: db,
		statementMutex: &sync.Mutex{},
		statements:     make(map[string]map[int]*sql.Stmt),
	}
}

// GetStmt creates and caches sql.Stmt structs based on the passed in statement
// and number of bound arguments. The caller must supply a well formed SQL statement,
// including parameter placeholders as necessary for the specific database API.
func (s StmtCache) GetStmt(statement string, num int) (*sql.Stmt, error) {
	s.statementMutex.Lock()
	defer s.statementMutex.Unlock()

	if s.statements[statement] != nil {
		if s.statements[statement][num] != nil {
			// TODO(al,martin): we'll possibly need to expire Stmts from the cache,
			// e.g. when DB connections break etc.
			return s.statements[statement][num], nil
		}
	} else {
		s.statements[statement] = make(map[int]*sql.Stmt)
	}

	stmt, err := s.db.PrepareContext(context.TODO(), statement)
	if err != nil {
		glog.Warningf("Failed to prepare statement %d: %s", num, err)
		return nil, err
	}

	s.statements[statement][num] = stmt

	return stmt, nil
}
