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

package postgresql

import (
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// postgresqlToGRPC converts some types of PostgreSQL errors to GRPC errors. This gives
// clients more signal when the operation can be retried.
func postgresqlToGRPC(err error) error {
	postgresqlErr, ok := err.(*pgconn.PgError)
	if !ok {
		return err
	}
	if postgresqlErr.Code == pgerrcode.DeadlockDetected {
		return status.Errorf(codes.Aborted, "PostgreSQL: %v", postgresqlErr)
	}
	return err
}
