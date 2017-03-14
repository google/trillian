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
	"testing"

	"google.golang.org/grpc/codes"
)

func TestCodes(t *testing.T) {
	tests := []struct {
		got  Code
		want codes.Code
	}{
		{got: Canceled, want: codes.Canceled},
		{got: Unknown, want: codes.Unknown},
		{got: InvalidArgument, want: codes.InvalidArgument},
		{got: DeadlineExceeded, want: codes.DeadlineExceeded},
		{got: NotFound, want: codes.NotFound},
		{got: AlreadyExists, want: codes.AlreadyExists},
		{got: PermissionDenied, want: codes.PermissionDenied},
		{got: Unauthenticated, want: codes.Unauthenticated},
		{got: ResourceExhausted, want: codes.ResourceExhausted},
		{got: FailedPrecondition, want: codes.FailedPrecondition},
		{got: Aborted, want: codes.Aborted},
		{got: OutOfRange, want: codes.OutOfRange},
		{got: Unimplemented, want: codes.Unimplemented},
		{got: Internal, want: codes.Internal},
		{got: Unavailable, want: codes.Unavailable},
		{got: DataLoss, want: codes.DataLoss},
	}
	for _, test := range tests {
		if uint64(test.got) != uint64(test.want) {
			t.Errorf("got = %v, want = %v", test.got, test.want)
		}
	}
}

func TestErrorf(t *testing.T) {
	tests := []struct {
		code    Code
		msg     string
		param   string
		wantMsg string
	}{
		// No need to test all values, just a couple is enough.
		{code: InvalidArgument, msg: "InvalidArgument: %v", param: "foo", wantMsg: "InvalidArgument: foo"},
		{code: NotFound, msg: "NotFound: %v", param: "bar", wantMsg: "NotFound: bar"},
	}
	for _, test := range tests {
		err := Errorf(test.code, test.msg, test.param)
		assertError(t, err, test.code, test.wantMsg)
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		code Code
		msg  string
	}{
		// No need to test all values, just a couple is enough.
		{code: InvalidArgument, msg: "err InvalidArgument"},
		{code: NotFound, msg: "err NotFound"},
	}
	for _, test := range tests {
		err := New(test.code, test.msg)
		assertError(t, err, test.code, test.msg)
	}
}

func assertError(t *testing.T, err error, wantCode Code, wantMsg string) {
	if got := err.Error(); got != wantMsg {
		t.Errorf("Error() = %v, want = %v", got, wantMsg)
	}
	terr, ok := err.(TrillianError)
	if !ok {
		t.Errorf("err is not a TrillianError: %T", err)
		return
	}
	if got := terr.Code(); got != wantCode {
		t.Errorf("Code() = %v, want = %v", got, wantCode)
	}
}
