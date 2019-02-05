// Copyright 2019 Google Inc. All Rights Reserved.
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

// Package mdm provides test-only code for checking the merge delay of a
// Trillian log.
package mdm

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/client"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/types"
)

// MergeDelayMonitor submits leaves to a Log and measures merge delay.
type MergeDelayMonitor struct {
	client []*client.LogClient
	logID  int64
	opts   MergeDelayOptions
}

// MergeDelayOptions holds the parameters for a MergeDelayMonitor.
type MergeDelayOptions struct {
	ParallelAdds  int
	LeafSize      int
	NewLeafChance int // percentage
	EmitInterval  time.Duration
	MetricFactory monitoring.MetricFactory
}

// NewMonitor creates a MergeDelayMonitor instance for the given log ID, accessed
// via the cl client.
func NewMonitor(ctx context.Context, logID int64, cl trillian.TrillianLogClient, adminCl trillian.TrillianAdminClient, opts MergeDelayOptions) (*MergeDelayMonitor, error) {
	if opts.MetricFactory == nil {
		opts.MetricFactory = monitoring.InertMetricFactory{}
	}
	if opts.EmitInterval == 0 {
		opts.EmitInterval = 10 * time.Second
	}
	metricsOnce.Do(func() { initMetrics(opts.MetricFactory) })

	tree, err := adminCl.GetTree(ctx, &trillian.GetTreeRequest{TreeId: logID})
	if err != nil {
		return nil, fmt.Errorf("failed to get tree %d: %v", logID, err)
	}
	verifier, err := client.NewLogVerifierFromTree(tree)
	if err != nil {
		return nil, fmt.Errorf("failed to build verifier: %v", err)
	}
	clients := make([]*client.LogClient, opts.ParallelAdds)
	for i := 0; i < opts.ParallelAdds; i++ {
		clients[i] = client.New(logID, cl, verifier, types.LogRootV1{})
	}

	mdm := MergeDelayMonitor{logID: logID, opts: opts, client: clients}
	return &mdm, nil
}

// Monitor runs merge delay monitoring until its context is cancelled or an error occurs.
func (m *MergeDelayMonitor) Monitor(ctx context.Context) error {
	glog.Infof("starting %d parallel monitor instances", m.opts.ParallelAdds)
	errs := make(chan error, m.opts.ParallelAdds)
	var wg sync.WaitGroup
	wg.Add(m.opts.ParallelAdds)
	for i := 0; i < m.opts.ParallelAdds; i++ {
		go func(i int) {
			defer wg.Done()
			if err := m.monitor(ctx, i); err != nil {
				errs <- err
			}
		}(i)
	}

	ticker := time.NewTicker(m.opts.EmitInterval)
	defer ticker.Stop()
	go func(c <-chan time.Time) {
		for range c {
			countT, totalT := m.Stats(true)
			if countT > 0 {
				glog.Infof("new leaves: %d in %f secs, average %f secs", countT, totalT, totalT/float64(countT))
			}
			countF, totalF := m.Stats(false)
			if countF > 0 {
				glog.Infof("dup leaves: %d in %f secs, average %f secs", countF, totalF, totalF/float64(countF))
			}
		}
	}(ticker.C)

	wg.Wait()
	close(errs)
	var lastErr error
	for err := range errs {
		glog.Errorf("monitor failure: %v", err)
		lastErr = err
	}
	return lastErr
}

func (m *MergeDelayMonitor) monitor(ctx context.Context, idx int) error {
	logIDLabel := strconv.FormatInt(m.logID, 10)
	data := make([]byte, m.opts.LeafSize)
	createNew := true // Always need a new leaf to start with
	for {
		if rand.Intn(100) < m.opts.NewLeafChance {
			createNew = true
		}
		if createNew {
			rand.Read(data)
		}

		// Add the leaf data and wait for its inclusion.
		start := time.Now()
		if err := m.client[idx].AddLeaf(ctx, data); err != nil {
			return fmt.Errorf("failed to QueueLeaf: %v", err)
		}
		mergeDelay := time.Since(start)
		mergeDelayDist.Observe(mergeDelay.Seconds(), logIDLabel, newLeafLabel[createNew])
		glog.V(1).Infof("[%d] merge delay for new=%t leaf = %v", idx, createNew, mergeDelay)

		select {
		case <-ctx.Done():
			return nil
		default:
		}
		createNew = false
	}
}

// Stats returns the total count of requests and the total elapsed time across
// all invocations.
func (m *MergeDelayMonitor) Stats(newLeaf bool) (uint64, float64) {
	logIDLabel := strconv.FormatInt(m.logID, 10)
	return mergeDelayDist.Info(logIDLabel, newLeafLabel[newLeaf])
}

var (
	metricsOnce    sync.Once
	mergeDelayDist monitoring.Histogram
	newLeafLabel   = map[bool]string{true: "true", false: "false"}
)

func initMetrics(mf monitoring.MetricFactory) {
	mergeDelayDist = mf.NewHistogram("merge_delay", "Merge delay for submitted leaves", "log_id", "new_leaf")
}
