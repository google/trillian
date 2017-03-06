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

package admin

import (
	"testing"

	"github.com/google/trillian"
	"golang.org/x/net/context"
)

func TestAdminServer_Unimplemented(t *testing.T) {
	tests := []struct {
		desc string
		fn   func(context.Context, trillian.TrillianAdminServer) error
	}{
		{
			desc: "ListTrees",
			fn: func(ctx context.Context, s trillian.TrillianAdminServer) error {
				_, err := s.ListTrees(ctx, &trillian.ListTreesRequest{})
				return err
			},
		},
		{
			desc: "GetTree",
			fn: func(ctx context.Context, s trillian.TrillianAdminServer) error {
				_, err := s.GetTree(ctx, &trillian.GetTreeRequest{})
				return err
			},
		},
		{
			desc: "CreateTree",
			fn: func(ctx context.Context, s trillian.TrillianAdminServer) error {
				_, err := s.CreateTree(ctx, &trillian.CreateTreeRequest{})
				return err
			},
		},
		{
			desc: "UpdateTree",
			fn: func(ctx context.Context, s trillian.TrillianAdminServer) error {
				_, err := s.UpdateTree(ctx, &trillian.UpdateTreeRequest{})
				return err
			},
		},
		{
			desc: "DeleteTree",
			fn: func(ctx context.Context, s trillian.TrillianAdminServer) error {
				_, err := s.DeleteTree(ctx, &trillian.DeleteTreeRequest{})
				return err
			},
		},
	}
	ctx := context.Background()
	s := &adminServer{}
	for _, test := range tests {
		if err := test.fn(ctx, s); err != errNotImplemented {
			t.Errorf("%v: got = %v, want = %v", test.desc, err, errNotImplemented)
		}
	}
}
