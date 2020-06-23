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

// Package cacheqm contains a caching quota.Manager implementation.
package cacheqm

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian/quota"
)

const (
	// DefaultMinBatchSize is the suggested default for minBatchSize.
	DefaultMinBatchSize = 100

	// DefaultMaxCacheEntries is the suggested default for maxEntries.
	DefaultMaxCacheEntries = 1000
)

// now is used in place of time.Now to allow tests to take control of time.
var now = time.Now

type manager struct {
	qm                       quota.Manager
	minBatchSize, maxEntries int

	// mu guards cache
	mu    sync.Mutex
	cache map[quota.Spec]*bucket

	// evictWg tracks evict() goroutines.
	evictWg sync.WaitGroup
}

type bucket struct {
	tokens       int
	lastModified time.Time
}

// NewCachedManager wraps a quota.Manager with an implementation that caches tokens locally.
//
// minBatchSize determines the minimum number of tokens requested from qm for each GetTokens()
// request.
//
// maxEntries determines the maximum number of cache entries, apart from global quotas. The oldest
// entries are evicted as necessary, their tokens replenished via PutTokens() to avoid excessive
// leakage.
func NewCachedManager(qm quota.Manager, minBatchSize, maxEntries int) (quota.Manager, error) {
	switch {
	case minBatchSize <= 0:
		return nil, fmt.Errorf("invalid minBatchSize: %v", minBatchSize)
	case maxEntries <= 0:
		return nil, fmt.Errorf("invalid maxEntries: %v", minBatchSize)
	}
	return &manager{
		qm:           qm,
		minBatchSize: minBatchSize,
		maxEntries:   maxEntries,
		cache:        make(map[quota.Spec]*bucket),
	}, nil
}

// PeekTokens implements Manager.PeekTokens.
func (m *manager) PeekTokens(ctx context.Context, specs []quota.Spec) (map[quota.Spec]int, error) {
	return m.qm.PeekTokens(ctx, specs)
}

// PutTokens implements Manager.PutTokens.
func (m *manager) PutTokens(ctx context.Context, numTokens int, specs []quota.Spec) error {
	return m.qm.PutTokens(ctx, numTokens, specs)
}

// ResetQuota implements Manager.ResetQuota.
func (m *manager) ResetQuota(ctx context.Context, specs []quota.Spec) error {
	return m.qm.ResetQuota(ctx, specs)
}

// GetTokens implements Manager.GetTokens.
func (m *manager) GetTokens(ctx context.Context, numTokens int, specs []quota.Spec) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Verify which buckets need more tokens, if any
	specsToRefill := []quota.Spec{}
	for _, spec := range specs {
		bucket, ok := m.cache[spec]
		if !ok || bucket.tokens < numTokens {
			specsToRefill = append(specsToRefill, spec)
		}
	}

	// Request the required number of tokens and add them to buckets
	if len(specsToRefill) != 0 {
		defer func() {
			// Do not hold GetTokens on eviction, it won't change the result.
			m.evictWg.Add(1)
			go func() {
				m.evict(ctx)
				m.evictWg.Done()
			}()
		}()

		// A more accurate count would be numTokens+m.minBatchSize-bucket.tokens, but that might
		// force us to make a GetTokens call for each spec. A single call is likely to be more
		// efficient.
		tokens := numTokens + m.minBatchSize
		if err := m.qm.GetTokens(ctx, tokens, specsToRefill); err != nil {
			return err
		}
		for _, spec := range specsToRefill {
			b, ok := m.cache[spec]
			if !ok {
				b = &bucket{}
				m.cache[spec] = b
			}
			b.tokens += tokens
		}
	}

	// Subtract tokens from cache
	lastModified := now()
	for _, spec := range specs {
		bucket, ok := m.cache[spec]
		// Sanity check
		if !ok || bucket.tokens < 0 || bucket.tokens < numTokens {
			glog.Errorf("Bucket invariants failed for spec %+v: ok = %v, bucket = %+v", spec, ok, bucket)
			return nil // Something is wrong with the implementation, let requests go through.
		}
		bucket.tokens -= numTokens
		bucket.lastModified = lastModified
	}
	return nil
}

func (m *manager) evict(ctx context.Context) {
	m.mu.Lock()
	// m.mu is explicitly unlocked, so we don't have to hold it while we wait for goroutines to
	// complete.

	if len(m.cache) <= m.maxEntries {
		m.mu.Unlock()
		return
	}

	// Find and evict the oldest entries. To avoid excessive token leakage, let's try and
	// replenish the tokens held for the evicted entries.
	var buckets bucketsByTime = make([]specBucket, 0, len(m.cache))
	for spec, b := range m.cache {
		if spec.Group != quota.Global {
			buckets = append(buckets, specBucket{bucket: b, spec: spec})
		}
	}
	sort.Sort(buckets)

	wg := sync.WaitGroup{}
	evicts := len(m.cache) - m.maxEntries
	for i := 0; i < evicts; i++ {
		b := buckets[i]
		glog.V(1).Infof("Too many tokens cached, returning least recently used (%v tokens for %+v)", b.tokens, b.spec)
		delete(m.cache, b.spec)

		// goroutines must not access the cache, the lock is released before they complete.
		wg.Add(1)
		go func() {
			if err := m.qm.PutTokens(ctx, b.tokens, []quota.Spec{b.spec}); err != nil {
				glog.Warningf("Error replenishing tokens from evicted bucket (spec = %+v, bucket = %+v): %v", b.spec, b.bucket, err)
			}
			wg.Done()
		}()
	}

	m.mu.Unlock()
	wg.Wait()
}

// wait waits for spawned goroutines to complete. Used by eviction tests.
func (m *manager) wait() {
	m.evictWg.Wait()
}

// specBucket is a bucket with the corresponding spec.
type specBucket struct {
	*bucket
	spec quota.Spec
}

// bucketsByTime is a sortable slice of specBuckets.
type bucketsByTime []specBucket

// Len provides sort.Interface.Len.
func (b bucketsByTime) Len() int {
	return len(b)
}

// Less provides sort.Interface.Less.
func (b bucketsByTime) Less(i, j int) bool {
	return b[i].lastModified.Before(b[j].lastModified)
}

// Swap provides sort.Interface.Swap.
func (b bucketsByTime) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}
