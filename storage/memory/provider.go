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

package memory

import (
	"github.com/golang/glog"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/storage"
)

func init() {
	if err := storage.RegisterProvider("memory", newMemoryStorageProvider); err != nil {
		glog.Fatalf("Failed to register storage provider memory: %v", err)
	}
}

type memProvider struct {
	mf monitoring.MetricFactory
	ts *TreeStorage
}

func newMemoryStorageProvider(mf monitoring.MetricFactory) (storage.Provider, error) {
	return &memProvider{
		mf: mf,
		ts: NewTreeStorage(),
	}, nil
}

func (s *memProvider) LogStorage() storage.LogStorage {
	return NewLogStorage(s.ts, s.mf)
}

func (s *memProvider) MapStorage() storage.MapStorage {
	return nil
}

func (s *memProvider) AdminStorage() storage.AdminStorage {
	return NewAdminStorage(s.ts)
}

func (s *memProvider) Close() error {
	return nil
}
