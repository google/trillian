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

package cache

import (
	"testing"

	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/storage/storagepb"
	"github.com/kylelemons/godebug/pretty"
)

func TestLogPopulation(t *testing.T) {
	f := LogPopulateFunc(rfc6962.DefaultHasher)

	for _, test := range []struct {
		name    string
		subtree *storagepb.SubtreeProto
		want    *storagepb.SubtreeProto
	}{} {
		t.Run(test.name, func(t *testing.T) {
			if err := f(test.subtree); err != nil {
				t.Fatalf("failed to repopulate subtree: %v", err)
			}
			if diff := pretty.Compare(test.want, test.subtree); diff != "" {
				t.Fatalf("unexpected repopulation - diff: %s", diff)
			}
		})
	}
}
