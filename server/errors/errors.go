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

package errors

import (
	"database/sql"

	te "github.com/google/trillian/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// WrapError wraps err as a gRPC error if err is a TrillianError or a well-known
// error instance (such as canonical sql errors), else err is returned
// unmodified.
func WrapError(err error) error {
	if err == sql.ErrNoRows {
		return status.Errorf(codes.NotFound, err.Error())
	}

	switch err := err.(type) {
	case te.TrillianError:
		return status.Errorf(codes.Code(err.Code()), err.Error())
	default:
		// Nothing to do: if it's a gRPC error it's already correct, if not gRPC will assume
		// codes.Unknown.
		return err
	}
}
