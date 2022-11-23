// Copyright 2022 Trillian Authors
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

// Package crdbqm defines a CockroachDB-based quota.Manager implementation.
package crdbqm

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

	// TODO(jaosorior): Come up with a more optimal solution for CRDB, as this is
	// linear and too costly.
	// Using a follower read here to reduce latency on the query. While this will
	// slightly less accurate than a read from the leader, it should be good enough.
	countFromUnsequencedTable = "SELECT COUNT(*) FROM Unsequenced AS OF SYSTEM TIME follower_read_timestamp()"
)

// ErrTooManyUnsequencedRows is returned when tokens are requested but Unsequenced has grown
// beyond the configured limit.
var ErrTooManyUnsequencedRows = errors.New("too many unsequenced rows")

// QuotaManager is a CockroachDB-based quota.Manager implementation.
//
// QuotaManager only implements Global/Write quotas, which is based on the number of Unsequenced
// rows (to be exact, tokens = MaxUnsequencedRows - actualUnsequencedRows).
// Other quotas are considered infinite.
type QuotaManager struct {
	DB                 *sql.DB
	MaxUnsequencedRows int
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
	// table names are lowercase for some reason
	rows, err := m.DB.QueryContext(ctx, countFromUnsequencedTable)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, errors.New("cursor has no rows after quota limit determination query")
	}
	var count int
	if err := rows.Scan(&count); err != nil {
		return 0, err
	}
	if rows.Next() {
		return 0, errors.New("too many rows returned from quota limit determination query")
	}
	return count, nil
}
