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

	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/storage"
)

type provider struct {
}

func (p *provider) LogStorage() storage.LogStorage     { return nil }
func (p *provider) MapStorage() storage.MapStorage     { return nil }
func (p *provider) AdminStorage() storage.AdminStorage { return nil }
func (p *provider) Close() error                       { return nil }

func TestStorageProviderRegistration(t *testing.T) {
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
				RegisterStorageProvider(name, func(_ monitoring.MetricFactory) (StorageProvider, error) {
					called = true
					return &provider{}, nil
				})
			}

			_, err := NewStorageProvider(name, nil)
			if err != nil && !test.wantErr {
				t.Fatalf("NewStorageProvider = %v, want no error", err)
			}
			if err == nil && test.wantErr {
				t.Fatalf("NewStorageProvider = no error, want error")
			}
			if !called && !test.wantErr {
				t.Fatal("Registered storage provider was not called")
			}
		})
	}
}

func TestStorageProviders(t *testing.T) {
	RegisterStorageProvider("a", func(_ monitoring.MetricFactory) (StorageProvider, error) {
		return &provider{}, nil
	})
	RegisterStorageProvider("b", func(_ monitoring.MetricFactory) (StorageProvider, error) {
		return &provider{}, nil
	})
	sp := StorageProviders()

	if got, want := len(sp), 2; got < want {
		t.Fatalf("Got %d names, want at least %d", got, want)
	}

	a := false
	b := false
	for _, n := range sp {
		if n == "a" {
			a = true
		}
		if n == "b" {
			b = true
		}
	}
	if !a {
		t.Error("StorageProviders() didn't include 'a'")
	}
	if !b {
		t.Error("StorageProviders() didn't include 'b'")
	}
}
