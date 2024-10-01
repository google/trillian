// Copyright 2021 Google LLC. All Rights Reserved.
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

package mysql

import (
	"github.com/go-sql-driver/mysql"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// ER_DUP_ENTRY: Error returned by driver when inserting a duplicate row.
	errNumDuplicate = 1062
	// ER_LOCK_DEADLOCK: Error returned when there was a deadlock.
	errNumDeadlock = 1213
)

// mysqlToGRPC converts some types of MySQL errors to GRPC errors. This gives
// clients more signal when the operation can be retried.
func mysqlToGRPC(err error) error {
	mysqlErr, ok := err.(*mysql.MySQLError)
	if !ok {
		return err
	}
	if mysqlErr.Number == errNumDeadlock {
		return status.Errorf(codes.Aborted, "MySQL: %v", mysqlErr)
	}
	return err
}

func isDuplicateErr(err error) bool {
	switch err := err.(type) {
	case *mysql.MySQLError:
		return err.Number == errNumDuplicate
	default:
		return false
	}
}
