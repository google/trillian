// Copyright 2018 Google LLC. All Rights Reserved.
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

package storage

import (
	"testing"

	"github.com/google/trillian/monitoring"
)

type provider struct {
}

func (p *provider) LogStorage() LogStorage     { return nil }
func (p *provider) MapStorage() MapStorage     { return nil }
func (p *provider) AdminStorage() AdminStorage { return nil }
func (p *provider) Close() error               { return nil }

func TestProviderRegistration(t *testing.T) {
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
				RegisterProvider(name, func(_ monitoring.MetricFactory) (Provider, error) {
					called = true
					return &provider{}, nil
				})
			}

			_, err := NewProvider(name, nil)
			if err != nil && !test.wantErr {
				t.Fatalf("NewProvider = %v, want no error", err)
			}
			if err == nil && test.wantErr {
				t.Fatalf("NewProvider = no error, want error")
			}
			if !called && !test.wantErr {
				t.Fatal("Registered storage provider was not called")
			}
		})
	}
}

func TestProviders(t *testing.T) {
	RegisterProvider("a", func(_ monitoring.MetricFactory) (Provider, error) {
		return &provider{}, nil
	})
	RegisterProvider("b", func(_ monitoring.MetricFactory) (Provider, error) {
		return &provider{}, nil
	})
	sp := providers()

	if got, want := len(sp), 2; got < want {
		t.Fatalf("Got %d names, want at least %d", got, want)
	}

	a := 0
	b := 0
	for _, n := range sp {
		if n == "a" {
			a++
		}
		if n == "b" {
			b++
		}
	}
	if a != 1 {
		t.Errorf("Providers() gave %d 'a', want 1", a)
	}
	if b != 1 {
		t.Errorf("Providers() gave %d 'b', want 1", b)
	}
}
