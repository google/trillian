// Copyright 2017 Google LLC. All Rights Reserved.
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

package cacheqm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/testonly/matchers"
)

const (
	minBatchSize = 20
	maxEntries   = 10
)

var (
	specs = []quota.Spec{
		{Group: quota.Global, Kind: quota.Read},
		{Group: quota.Global, Kind: quota.Write},
	}
)

func TestNewCachedManagerErrors(t *testing.T) {
	tests := []struct {
		minBatchSize, maxEntries int
	}{
		{minBatchSize: 0, maxEntries: 10},
		{minBatchSize: -1, maxEntries: 10},
		{minBatchSize: 10, maxEntries: 0},
		{minBatchSize: 10, maxEntries: -1},
	}
	qm := quota.Noop()
	for _, test := range tests {
		if _, err := NewCachedManager(qm, test.minBatchSize, test.maxEntries); err == nil {
			t.Errorf("NewCachedManager(_, %v, %v) returned err = nil, want non-nil", test.minBatchSize, test.maxEntries)
		}
	}
}

func TestCachedManager_PeekTokens(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		desc       string
		wantTokens map[quota.Spec]int
		wantErr    error
	}{
		{
			desc: "success",
			wantTokens: map[quota.Spec]int{
				{Group: quota.Global, Kind: quota.Read}: 10,
				{Group: quota.Global, Kind: quota.Read}: 11,
			},
		},
		{
			desc:    "error",
			wantErr: errors.New("llama ate all tokens"),
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		mock := quota.NewMockManager(ctrl)
		mock.EXPECT().PeekTokens(ctx, specs).Return(test.wantTokens, test.wantErr)

		qm, err := NewCachedManager(mock, minBatchSize, maxEntries)
		if err != nil {
			t.Fatalf("NewCachedManager() returned err = %v", err)
		}

		tokens, err := qm.PeekTokens(ctx, specs)
		if diff := cmp.Diff(tokens, test.wantTokens); diff != "" {
			t.Errorf("%v: post-PeekTokens() diff (-got +want):\n%v", test.desc, diff)
		}
		if err != test.wantErr {
			t.Errorf("%v: PeekTokens() returned err = %#v, want = %#v", test.desc, err, test.wantErr)
		}
	}
}

// TestCachedManager_DelegatedMethods tests all delegated methods that have a single error return.
func TestCachedManager_DelegatedMethods(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	tokens := 5
	for _, want := range []error{nil, errors.New("llama ate all tokens")} {
		mock := quota.NewMockManager(ctrl)
		qm, err := NewCachedManager(mock, minBatchSize, maxEntries)
		if err != nil {
			t.Fatalf("NewCachedManager() returned err = %v", err)
		}

		mock.EXPECT().PutTokens(ctx, tokens, specs).Return(want)
		if err := qm.PutTokens(ctx, tokens, specs); err != want {
			t.Errorf("PutTokens() returned err = %#v, want = %#v", err, want)
		}

		mock.EXPECT().ResetQuota(ctx, specs).Return(want)
		if err := qm.ResetQuota(ctx, specs); err != want {
			t.Errorf("ResetQuota() returned err = %#v, want = %#v", err, want)
		}
	}
}

func TestCachedManager_GetTokens_CachesTokens(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	tokens := 3
	mock := quota.NewMockManager(ctrl)
	mock.EXPECT().GetTokens(ctx, matchers.AtLeast(minBatchSize), specs).Times(2).Return(nil)

	qm, err := NewCachedManager(mock, minBatchSize, maxEntries)
	if err != nil {
		t.Fatalf("NewCachedManager() returned err = %v", err)
	}

	// Quota requests happen in tokens+minBatchSize steps, so that minBatchSize tokens get cached
	// after the the request is satisfied.
	// Therefore, the call pattern below is satisfied by just 2 underlying GetTokens() calls.
	calls := []int{tokens, minBatchSize, tokens, minBatchSize / 2, minBatchSize / 2}
	for i, call := range calls {
		if err := qm.GetTokens(ctx, call, specs); err != nil {
			t.Fatalf("GetTokens() returned err = %v (call #%v)", err, i+1)
		}
	}
}

func TestCachedManager_GetTokens_EvictsCache(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := quota.NewMockManager(ctrl)
	mock.EXPECT().GetTokens(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil)

	ctx := context.Background()
	maxEntries := 100
	qm, err := NewCachedManager(mock, minBatchSize, maxEntries)
	if err != nil {
		t.Fatalf("NewCachedManager() returned err = %v", err)
	}

	originalNow := now
	defer func() { now = originalNow }()
	currentTime := time.Time{}
	now = func() time.Time {
		currentTime = currentTime.Add(1 * time.Second)
		return currentTime
	}

	// Ensure Global quotas are the oldest, we don't want those to get evicted regardless of age.
	tokens := 5
	if err := qm.GetTokens(ctx, tokens, []quota.Spec{
		{Group: quota.Global, Kind: quota.Read},
		{Group: quota.Global, Kind: quota.Write},
	}); err != nil {
		t.Fatalf("GetTokens() returned err = %v", err)
	}

	// Fill the cache up to maxEntries
	firstTree := int64(10)
	tree := firstTree
	for i := 0; i < maxEntries-2; i++ {
		if err := qm.GetTokens(ctx, tokens, treeSpecs(tree)); err != nil {
			t.Fatalf("GetTokens() returned err = %v (i = %v)", err, i)
		}
		tree++
	}

	// All entries added from now on must cause eviction of the oldest entries.
	// Evict trees in pairs to exercise the inner evict loop.
	evicts := 20
	for i := 0; i < evicts; i += 2 {
		mock.EXPECT().PutTokens(ctx, minBatchSize, treeSpecs(firstTree+int64(i))).Return(nil)
		mock.EXPECT().PutTokens(ctx, minBatchSize, treeSpecs(firstTree+int64(i+1))).Return(nil)

		specs := []quota.Spec{treeSpec(tree), treeSpec(tree + 1)}
		tree += 2
		if err := qm.GetTokens(ctx, tokens, specs); err != nil {
			t.Fatalf("GetTokens() returned err = %v (i = %v)", err, i)
		}
	}

	waitChan := make(chan bool, 1)
	go func() {
		qm.(*manager).wait()
		waitChan <- true
	}()

	select {
	case <-waitChan:
		// OK, test exited cleanly
	case <-time.After(5 * time.Second):
		t.Errorf("Timed out waiting for qm.wait(), failing test")
	}
}

func TestManager_GetTokensErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	want := errors.New("llama ate all tokens")
	mock := quota.NewMockManager(ctrl)
	mock.EXPECT().GetTokens(ctx, gomock.Any(), specs).Return(want)

	qm, err := NewCachedManager(mock, minBatchSize, maxEntries)
	if err != nil {
		t.Fatalf("NewCachedManager() returned err = %v", err)
	}

	if err := qm.GetTokens(ctx, 5 /* numTokens */, specs); err != want {
		t.Errorf("GetTokens() returned err = %#v, want = %#v", err, want)
	}
}

func treeSpecs(treeID int64) []quota.Spec {
	return []quota.Spec{treeSpec(treeID)}
}

func treeSpec(treeID int64) quota.Spec {
	return quota.Spec{Group: quota.Tree, Kind: quota.Write, TreeID: treeID}
}
