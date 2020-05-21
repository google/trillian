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

package matchers

import (
	"testing"

	_ "github.com/golang/glog"
	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/google/trillian"
)

func TestProtoEquals(t *testing.T) {
	for _, tc := range []struct {
		a, b  proto.Message
		equal bool
	}{
		{a: &trillian.SignedLogRoot{}, b: &trillian.SignedLogRoot{}, equal: true},
		{a: &trillian.SignedLogRoot{}, b: &trillian.SignedLogRoot{LogRoot: []byte{}}, equal: true},
		{a: &trillian.SignedLogRoot{}, b: &trillian.SignedLogRoot{LogRoot: []byte{0x01}}, equal: false},
	} {
		if got, want := ProtoEqual(tc.a).Matches(tc.b), tc.equal; got != want {
			t.Errorf("ProtoEqual(%s).Matches(%s) = %v, want %v", tc.a, tc.b, got, want)
		}
	}
}
