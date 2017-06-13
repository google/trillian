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

package simkafka

import (
	"reflect"
	"testing"
)

func TestReadEmpty(t *testing.T) {
	topic := "test"
	offset := 0
	if got := Read(topic, offset); got != "" {
		t.Errorf("Read(%q, %d)=%q; want ''", topic, offset, got)
	}
	offset = 2
	if got := Read(topic, offset); got != "" {
		t.Errorf("Read(%q, %d)=%q; want ''", topic, offset, got)
	}
	if got := ReadMultiple(topic, offset, 1); got != nil {
		t.Errorf("ReadMultiple(%q, %d, 1)=%q; want ''", topic, offset, got)
	}
	if got, gotOffset := ReadLast(topic); got != "" || gotOffset != -1 {
		t.Errorf("ReadLast(%q, %d)=%q,%d; want '',-1", topic, offset, got, gotOffset)
	}
}
func TestRead(t *testing.T) {
	topic := "test2"
	want := "abc"
	if got := Append(topic, want); got != 0 {
		t.Errorf("Append(%q, %q)=%d; want 0", topic, want, got)
	}
	offset := 0
	if got := Read(topic, offset); got != want {
		t.Errorf("Read(%q, %d)=%q; want %q", topic, offset, got, want)
	}
	if got := ReadMultiple(topic, offset, 1); !reflect.DeepEqual(got, []string{want}) {
		t.Errorf("Read(%q, %d)=%q; want [%q]", topic, offset, got, want)
	}
	if got := ReadMultiple(topic, offset, 10); !reflect.DeepEqual(got, []string{want}) {
		t.Errorf("Read(%q, %d)=%q; want [%q]", topic, offset, got, want)
	}
	offset = 2
	if got := Read(topic, offset); got != "" {
		t.Errorf("Read(%q, %d)=%v; want ''", topic, offset, got)
	}
	if got := ReadMultiple(topic, offset, 1); got != nil {
		t.Errorf("ReadMultiple(%q, %d, 1)=%v; want ''", topic, offset, got)
	}
}

func TestAppend(t *testing.T) {
	topic := "test3"
	values := []string{"ab", "cd", "ef", "gh"}
	for offset, value := range values {
		if got := Append(topic, value); got != offset {
			t.Errorf("Append(%q, %q)=%d; want %d", topic, value, got, offset)
		}
		if got, gotOffset := ReadLast("test3"); got != value || gotOffset != offset {
			t.Errorf("ReadLast('test3')=%q,%d; want %q,%d", got, gotOffset, value, offset)
		}
	}
	want := "| 0:ab | 1:cd | 2:ef | 3:gh |"
	if got := topics["test3"].String(); got != want {
		t.Errorf("topic.String()=%q; want %q", got, want)
	}
}
