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

package signer

import (
	"reflect"
	"testing"
)

func TestSTH(t *testing.T) {
	sth := STH{TreeSize: 12, TimeStamp: 100}
	enc := sth.String()
	if got, want := enc, `{"sz":12,"tm":100,"off":0}`; got != want {
		t.Errorf("sth.String=%q; want %q", got, want)
	}
	dec, err := sthFromString(enc)
	if err != nil {
		t.Errorf("sthFromString()=%v, want: nil", err)
	}
	if !reflect.DeepEqual(dec, &sth) {
		t.Errorf("sthFromString(%q)=%+v; want %+v", enc, *dec, sth)
	}
}
