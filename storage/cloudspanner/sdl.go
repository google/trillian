// Copyright 2020 Google LLC. All Rights Reserved.
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

package cloudspanner

import (
	"encoding/base64"

	"cloud.google.com/go/spanner/spansql"
)

//go:generate sh gen.sh

// readDDL returns a list of DDL statements from the database schema.
func readDDL() ([]string, error) {
	ddlString, err := base64.StdEncoding.DecodeString(base64DDL)
	if err != nil {
		return nil, err
	}
	ddl, err := spansql.ParseDDL("spanner.sdl.go", string(ddlString))
	if err != nil {
		return nil, err
	}
	stmts := make([]string, 0, len(ddl.List))
	for _, s := range ddl.List {
		stmts = append(stmts, s.SQL())
	}
	return stmts, nil
}
