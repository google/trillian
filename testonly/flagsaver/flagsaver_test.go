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

package flagsaver

import (
	"flag"
	"testing"
	"time"

	_ "github.com/golang/glog"
)

var (
	_ = flag.Int("int_flag", 123, "test integer flag")
	_ = flag.String("str_flag", "foo", "test string flag")
	_ = flag.Duration("duration_flag", 5*time.Second, "test duration flag")
)

// TestRestore checks that flags are saved and restore correctly.
// Checks are performed on flags with both their default values and with explicit values set.
// Only a subset of the possible flag types are currently tested.
func TestRestore(t *testing.T) {
	tests := []struct {
		desc string
		// flag is the name of the flag to save and restore.
		flag string
		// oldValue is the value the flag should have when saved. If empty, this indicates the flag should have its default value.
		oldValue string
		// newValue is the value the flag should have just before being restored to oldValue.
		newValue string
	}{
		{
			desc:     "RestoreDefaultIntValue",
			flag:     "int_flag",
			newValue: "666",
		},
		{
			desc:     "RestoreDefaultStrValue",
			flag:     "str_flag",
			newValue: "baz",
		},
		{
			desc:     "RestoreDefaultDurationValue",
			flag:     "duration_flag",
			newValue: "1m0s",
		},
		{
			desc:     "RestoreSetIntValue",
			flag:     "int_flag",
			oldValue: "555",
			newValue: "666",
		},
		{
			desc:     "RestoreSetStrValue",
			flag:     "str_flag",
			oldValue: "bar",
			newValue: "baz",
		},
		{
			desc:     "RestoreSetDurationValue",
			flag:     "duration_flag",
			oldValue: "10s",
			newValue: "1m0s",
		},
	}

	for _, test := range tests {
		f := flag.Lookup(test.flag)
		if f == nil {
			t.Errorf("%v: flag.Lookup(%q) = nil, want not nil", test.desc, test.flag)
			continue
		}

		if test.oldValue != "" {
			if err := flag.Set(test.flag, test.oldValue); err != nil {
				t.Errorf("%v: flag.Set(%q, %q) = %q, want nil", test.desc, test.flag, test.oldValue, err)
				continue
			}
		} else {
			// Use the default value of the flag as the oldValue if none was set.
			test.oldValue = f.DefValue
		}

		func() {
			defer Save().MustRestore()
			// If the Set() fails the value won't have been updated but some of the
			// test cases set the same value so it's safer to have this check.
			if err := flag.Set(test.flag, test.newValue); err != nil {
				t.Errorf("%v: flag.Set(%q) = %q, want nil", test.desc, test.flag, err)
			}
			if gotValue := f.Value.String(); gotValue != test.newValue {
				t.Errorf("%v: flag.Lookup(%q).Value.String() = %q, want %q", test.desc, test.flag, gotValue, test.newValue)
			}
		}()

		if gotValue := f.Value.String(); gotValue != test.oldValue {
			t.Errorf("%v: flag.Lookup(%q).Value.String() = %q, want %q", test.desc, test.flag, gotValue, test.oldValue)
		}
	}
}
