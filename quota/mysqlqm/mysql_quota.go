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

// Package mysqlqm defines a MySQL-based quota.Manager implementation.
package mysqlqm

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/trillian/quota"
)

const (
	// DefaultMaxUnsequenced is a suggested value for MaxUnsequencedRows.
	// Note that this is a Global/Write quota suggestion, so it applies across trees.
	DefaultMaxUnsequenced = 500000 // About 2h of non-stop signing at 70QPS.

	countFromInformationSchemaQuery = `
		SELECT table_rows
		FROM information_schema.tables
		WHERE table_schema = schema()
			AND table_name = ?
			AND table_type = ?`
	countFromUnsequencedQuery = "SELECT COUNT(*) FROM Unsequenced"
)

var (
	// ErrTooManyUnsequencedRows is returned when tokens are requested but Unsequenced has grown
	// beyond the configured limit.
	ErrTooManyUnsequencedRows = errors.New("too many unsequenced rows")
)

// QuotaManager is a MySQL-based quota.Manager implementation.
//
// It has two working modes: one queries the information schema for the number of Unsequenced rows,
// the other does a select count(*) on the Unsequenced table. Information schema queries are
// default, even though they are approximate, as they're constant time (select count(*) on InnoDB
// based MySQL needs to traverse the index and may take quite a while to complete).
//
// QuotaManager only implements Global/Write quotas, which is based on the number of Unsequenced
// rows (to be exact, tokens = MaxUnsequencedRows - actualUnsequencedRows).
// Other quotas are considered infinite.
type QuotaManager struct {
	DB                 *sql.DB
	MaxUnsequencedRows int
	UseSelectCount     bool
}

// GetUser implements quota.Manager.GetUser.
// User quotas are not implemented by QuotaManager.
func (m *QuotaManager) GetUser(ctx context.Context, req interface{}) string {
	return "" // Not used
}

// GetTokens implements quota.Manager.GetTokens.
// It doesn't actually reserve or retrieve tokens, instead it allows access based on the number of
// rows in the Unsequenced table.
func (m *QuotaManager) GetTokens(ctx context.Context, numTokens int, specs []quota.Spec) error {
	for _, spec := range specs {
		if spec.Group != quota.Global || spec.Kind != quota.Write {
			continue
		}
		// Only allow global writes if Unsequenced is under the expected limit
		count, err := m.countUnsequenced(ctx)
		if err != nil {
			return err
		}
		if count+numTokens > m.MaxUnsequencedRows {
			return ErrTooManyUnsequencedRows
		}
	}
	return nil
}

// PeekTokens implements quota.Manager.PeekTokens.
// Global/Write tokens reflect the number of rows in the Unsequenced tables, other specs are
// considered infinite.
func (m *QuotaManager) PeekTokens(ctx context.Context, specs []quota.Spec) (map[quota.Spec]int, error) {
	tokens := make(map[quota.Spec]int)
	for _, spec := range specs {
		var num int
		if spec.Group == quota.Global && spec.Kind == quota.Write {
			count, err := m.countUnsequenced(ctx)
			if err != nil {
				return nil, err
			}
			num = m.MaxUnsequencedRows - count
		} else {
			num = quota.MaxTokens
		}
		tokens[spec] = num
	}
	return tokens, nil
}

// PutTokens implements quota.Manager.PutTokens.
// It's a noop for QuotaManager.
func (m *QuotaManager) PutTokens(ctx context.Context, numTokens int, specs []quota.Spec) error {
	return nil
}

// ResetQuota implements quota.Manager.ResetQuota.
// It's a noop for QuotaManager.
func (m *QuotaManager) ResetQuota(ctx context.Context, specs []quota.Spec) error {
	return nil
}

func (m *QuotaManager) countUnsequenced(ctx context.Context) (int, error) {
	if m.UseSelectCount {
		return countFromTable(ctx, m.DB)
	}
	return countFromInformationSchema(ctx, m.DB)
}

func countFromInformationSchema(ctx context.Context, db *sql.DB) (int, error) {
	// information_schema.tables doesn't have an explicit PK, so let's play it safe and ensure
	// the cursor returns a single row.
	rows, err := db.QueryContext(ctx, countFromInformationSchemaQuery, "Unsequenced", "BASE TABLE")
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, errors.New("cursor has no rows after information_schema query")
	}
	var count int
	if err := rows.Scan(&count); err != nil {
		return 0, err
	}
	if rows.Next() {
		return 0, errors.New("too many rows returned from information_schema query")
	}
	return count, nil
}

func countFromTable(ctx context.Context, db *sql.DB) (int, error) {
	var count int
	if err := db.QueryRowContext(ctx, countFromUnsequencedQuery).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}
