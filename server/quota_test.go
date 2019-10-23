// Copyright 2018 Google Inc. All Rights Reserved.
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

package server

import (
	"testing"

	"github.com/google/trillian/quota"
)

func TestQuotaProviderRegistration(t *testing.T) {
	for _, test := range []struct {
		desc    string
		reg     bool
		wantErr bool
	}{
		{
			desc:    "works",
			reg:     true,
			wantErr: false,
		},
		{
			desc:    "unknown provider",
			reg:     false,
			wantErr: true,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			called := false
			name := test.desc

			if test.reg {
				if err := RegisterQuotaManager(name, func() (quota.Manager, error) {
					called = true
					return nil, nil
				}); err != nil {
					t.Fatalf("RegisterQuotaManager(%s)=%v", name, err)
				}
			}

			_, err := NewQuotaManager(name)
			if err != nil && !test.wantErr {
				t.Fatalf("NewQuotaManager = %v, want no error", err)
			}
			if err == nil && test.wantErr {
				t.Fatalf("NewQuotaManager = no error, want error")
			}

			if !called && !test.wantErr {
				t.Fatal("Registered quota provider was not called")
			}
		})
	}
}

func TestQuotaSystems(t *testing.T) {
	if err := RegisterQuotaManager("a", func() (quota.Manager, error) { return nil, nil }); err != nil {
		t.Fatalf("RegisterQuotaManager(a)=%v", err)
	}
	if err := RegisterQuotaManager("b", func() (quota.Manager, error) { return nil, nil }); err != nil {
		t.Fatalf("RegisterQuotaManager(b)=%v", err)
	}
	qs := quotaSystems()

	if got, want := len(qs), 2; got < want {
		t.Fatalf("Got %d names, want at least %d", got, want)
	}

	a := 0
	b := 0
	for _, n := range qs {
		if n == "a" {
			a++
		}
		if n == "b" {
			b++
		}
	}
	if a != 1 {
		t.Errorf("QuotaSystems() returned %d 'a', want 1", a)
	}
	if b != 1 {
		t.Errorf("QuotaSystems() returned %d 'b', want 1", b)
	}
}
