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
		if err != nil {
			t.Errorf("GetLogStorage(%v) failed with error: %v", test.treeID, err)
		} else if got != test.want {
			t.Errorf("GetLogStorage(%v) failed, want %q, got %q", test.treeID, test.want, got)
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
		if err == nil || err != want {
			t.Errorf("want %q, got %q", want, err)
		} else if ls != nil {
			t.Errorf("returned LogStorage should be nil, got %q", ls)
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
		if err != nil {
			t.Errorf("GetMapStorage(%v) failed with error: %v", test.treeID, err)
		} else if got != test.want {
			t.Errorf("GetMapStorage(%v) failed, want %q, got %q", test.treeID, test.want, got)
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
		if err == nil || err != want {
			t.Errorf("want %q, got %q", want, err)
		} else if ls != nil {
			t.Errorf("returned MapStorage should be nil, got %q", ls)
		}
	}
}
