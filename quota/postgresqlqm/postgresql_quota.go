// Copyright 2024 Trillian Authors. All Rights Reserved.
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

// Package postgresqlqm defines a PostgreSQL-based quota.Manager implementation.
package postgresqlqm

import (
	"context"
	"errors"

	"github.com/google/trillian/quota"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// DefaultMaxUnsequenced is a suggested value for MaxUnsequencedRows.
	// Note that this is a Global/Write quota suggestion, so it applies across trees.
	DefaultMaxUnsequenced = 500000 // About 2h of non-stop signing at 70QPS.

	countFromExplainOutputQuery = "SELECT count_estimate($1)"
	countFromUnsequencedQuery   = "SELECT COUNT(*) FROM Unsequenced"
)

// ErrTooManyUnsequencedRows is returned when tokens are requested but Unsequenced has grown
// beyond the configured limit.
var ErrTooManyUnsequencedRows = errors.New("too many unsequenced rows")

// QuotaManager is a PostgreSQL-based quota.Manager implementation.
//
// QuotaManager only implements Global/Write quotas, which is based on the number of Unsequenced
// rows (to be exact, tokens = MaxUnsequencedRows - actualUnsequencedRows).
// Other quotas are considered infinite. In other words, it attempts to protect the MMD SLO of all
// logs in the instance, but it does not make any attempt to ensure fairness, whether per-tree,
// per-intermediate-CA (in the case of Certificate Transparency), or any other dimension.
//
// It has two working modes: one estimates the number of Unsequenced rows by collecting information
// from EXPLAIN output; the other does a select count(*) on the Unsequenced table. Estimates are
// default, even though they are approximate, as they're constant time (select count(*) on
// PostgreSQL needs to traverse the index and may take quite a while to complete).
// Other estimation methods exist (see https://wiki.postgresql.org/wiki/Count_estimate), but using
// EXPLAIN output is the most accurate because it "fetches the actual current number of pages in
// the table (this is a cheap operation, not requiring a table scan). If that is different from
// relpages then reltuples is scaled accordingly to arrive at a current number-of-rows estimate."
// (quoting https://www.postgresql.org/docs/current/row-estimation-examples.html)
type QuotaManager struct {
	DB                 *pgxpool.Pool
	MaxUnsequencedRows int
	UseSelectCount     bool
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
	if m.UseSelectCount {
		return countFromTable(ctx, m.DB)
	}
	return countFromExplainOutput(ctx, m.DB)
}

func countFromExplainOutput(ctx context.Context, db *pgxpool.Pool) (int, error) {
	var count int
	if err := db.QueryRow(ctx, countFromExplainOutputQuery, "Unsequenced").Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func countFromTable(ctx context.Context, db *pgxpool.Pool) (int, error) {
	var count int
	if err := db.QueryRow(ctx, countFromUnsequencedQuery).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}
