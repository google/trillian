// Copyright 2017 Google LLC. All Rights Reserved.
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

package quota

import "testing"

func TestSpec_Name(t *testing.T) {
	tests := []struct {
		spec Spec
		want string
	}{
		{spec: Spec{Group: Global, Kind: Read}, want: "global/read"},
		{spec: Spec{Group: Global, Kind: Write}, want: "global/write"},
		{spec: Spec{Group: Tree, Kind: Read, TreeID: 11}, want: "trees/11/read"},
		{spec: Spec{Group: Tree, Kind: Write, TreeID: 10}, want: "trees/10/write"},
		{spec: Spec{Group: User, Kind: Read, User: "alpaca"}, want: "users/alpaca/read"},
		{spec: Spec{Group: User, Kind: Write, User: "llama"}, want: "users/llama/write"},
	}
	for _, test := range tests {
		if got := test.spec.Name(); got != test.want {
			t.Errorf("%#v.Name() = %v, want = %v", test.spec, got, test.want)
		}
	}
}
