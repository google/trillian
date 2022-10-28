// Copyright 2022 <TBD>
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

package crdb

import (
	"github.com/lib/pq"
)

var uniqueViolationErrorCode = pq.ErrorCode("23505")

// crdbToGRPC converts some types of CockroachDB errors to GRPC errors. This gives
// clients more signal when the operation can be retried.
func crdbToGRPC(err error) error {
	_, ok := err.(*pq.Error)
	if !ok {
		return err
	}
	// TODO(jaosorior): Do we have a crdb equivalent for a deadlock
	// error code?
	return err
}

func isDuplicateErr(err error) bool {
	switch err := err.(type) {
	case *pq.Error:
		return err.Code == uniqueViolationErrorCode
	default:
		return false
	}
}
