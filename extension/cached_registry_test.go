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

package extension

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/trillian/storage"
)

func TestGetLogStorage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	registry := NewMockRegistry(ctrl)
	cachedRegistry := NewCachedRegistry(registry)

	ls1 := storage.NewMockLogStorage(ctrl)
	ls2 := storage.NewMockLogStorage(ctrl)
	registry.EXPECT().GetLogStorage(int64(1)).Times(1).Return(ls1, nil)
	registry.EXPECT().GetLogStorage(int64(2)).Times(1).Return(ls2, nil)

	var tests = []struct {
		treeID int64
		want   storage.LogStorage
	}{
		{1, ls1},
		{1, ls1}, // Same key twice to test caching
		{2, ls2},
	}
	for _, test := range tests {
		got, err := cachedRegistry.GetLogStorage(test.treeID)
		switch {
		case err != nil:
			t.Errorf("GetLogStorage(%v) = (_, %v)", test.treeID, err)
		case got != test.want:
			t.Errorf("GetLogStorage(%v) = (%q, nil), want %q", test.treeID, got, test.want)
		}
	}
}

func TestGetLogStorageError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	registry := NewMockRegistry(ctrl)
	cachedRegistry := NewCachedRegistry(registry)

	want := fmt.Errorf("Error getting log storage")
	registry.EXPECT().GetLogStorage(int64(1)).Times(2).Return(nil, want)

	// Run twice to make sure caching isn't doing anything funky
	for i := 0; i < 2; i++ {
		ls, err := cachedRegistry.GetLogStorage(1)
		switch {
		case err != want:
			t.Errorf("GetLogStorage(1) = (_, %q), want %q", err, want)
		case ls != nil:
			t.Errorf("GetLogStorage(1) = (%q, _), want nil", ls)
		}
	}
}

func TestGetMapStorage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	registry := NewMockRegistry(ctrl)
	cachedRegistry := NewCachedRegistry(registry)

	ms1 := storage.NewMockMapStorage(ctrl)
	ms2 := storage.NewMockMapStorage(ctrl)
	registry.EXPECT().GetMapStorage(int64(1)).Times(1).Return(ms1, nil)
	registry.EXPECT().GetMapStorage(int64(2)).Times(1).Return(ms2, nil)

	var tests = []struct {
		treeID int64
		want   storage.MapStorage
	}{
		{1, ms1},
		{1, ms1}, // Same key twice to test caching
		{2, ms2},
	}
	for _, test := range tests {
		got, err := cachedRegistry.GetMapStorage(test.treeID)
		switch {
		case err != nil:
			t.Errorf("GetMapStorage(%v) = (_, %v)", test.treeID, err)
		case got != test.want:
			t.Errorf("GetMapStorage(%v) = (%q, nil), want %q", test.treeID, got, test.want)
		}
	}
}

func TestGetMapStorageError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	registry := NewMockRegistry(ctrl)
	cachedRegistry := NewCachedRegistry(registry)

	want := fmt.Errorf("Error getting map storage")
	registry.EXPECT().GetMapStorage(int64(1)).Times(2).Return(nil, want)

	// Run twice to make sure caching isn't doing anything funky
	for i := 0; i < 2; i++ {
		ls, err := cachedRegistry.GetMapStorage(1)
		switch {
		case err != want:
			t.Errorf("GetMapStorage(1) = (_, %q), want %q", err, want)
		case ls != nil:
			t.Errorf("GetMapStorage(1) = (%q, _), want nil", ls)
		}
	}
}
