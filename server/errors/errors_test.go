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
	"errors"
	"testing"

	te "github.com/google/trillian/errors"
	"github.com/kylelemons/godebug/pretty"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

func TestWrapError(t *testing.T) {
	trillianErr := te.New(te.InvalidArgument, "invalid argument err")
	grpcErr := grpc.Errorf(codes.NotFound, "not found err")
	err := errors.New("generic error")

	tests := []struct {
		err     error
		wantErr error
	}{
		{
			err:     trillianErr,
			wantErr: grpc.Errorf(codes.InvalidArgument, trillianErr.Error()),
		},
		{
			err:     grpcErr,
			wantErr: grpcErr,
		},
		{
			err:     err,
			wantErr: err,
		},
	}
	for _, test := range tests {
		// We can't use == for rpcErrors because grpc.Errorf returns *rpcError.
		if diff := pretty.Compare(WrapError(test.err), test.wantErr); diff != "" {
			t.Errorf("WrapError('%v') diff:\n%s", test.err, diff)
		}
	}
}
