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

package admin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/storage"
)

const (
	deleteErrReason        = "delete_error"
	timestampParseErrReson = "timestamp_parse_error"
)

var (
	timeNow   = time.Now
	timeSleep = time.Sleep

	hardDeleteCounter monitoring.Counter
	metricsOnce       sync.Once
)

func incHardDeleteCounter(treeID int64, success bool, reason string) {
	hardDeleteCounter.Inc(fmt.Sprint(treeID), fmt.Sprint(success), reason)
}

// DeletedTreeGC garbage collects deleted trees.
//
// Tree deletion goes through two separate stages:
// * Soft deletion, which "flips a bit" (see Tree.Deleted) but otherwise leaves the tree unaltered
// * Hard deletion, which effectively removes all tree data
//
// DeletedTreeGC performs the transition from soft to hard deletion. Trees that have been deleted
// for at least DeletedThreshold are eligible for garbage collection.
type DeletedTreeGC struct {
	// admin is the storage.AdminStorage interface.
	admin storage.AdminStorage

	// deleteThreshold defines the minimum time a tree has to remain in the soft-deleted state
	// before it's eligible for garbage collection.
	deleteThreshold time.Duration

	// minRunInterval defines how frequently sweeps for deleted trees are performed.
	// Actual runs happen randomly between [minInterval,2*minInterval).
	minRunInterval time.Duration
}

// NewDeletedTreeGC returns a new DeletedTreeGC.
func NewDeletedTreeGC(admin storage.AdminStorage, threshold, minRunInterval time.Duration, mf monitoring.MetricFactory) *DeletedTreeGC {
	gc := &DeletedTreeGC{
		admin:           admin,
		deleteThreshold: threshold,
		minRunInterval:  minRunInterval,
	}
	metricsOnce.Do(func() {
		if mf == nil {
			mf = monitoring.InertMetricFactory{}
		}
		hardDeleteCounter = mf.NewCounter("tree_hard_delete_counter", "Counter of hard-deleted trees", monitoring.TreeIDLabel, "success", "reason")
	})
	return gc
}

// Run starts the tree garbage collection process. It runs until ctx is cancelled.
func (gc *DeletedTreeGC) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		count, err := gc.RunOnce(ctx)
		if err != nil {
			glog.Errorf("DeletedTreeGC.Run: %v", err)
		}
		if count > 0 {
			glog.Infof("DeletedTreeGC.Run: successfully deleted %v trees", count)
		}

		d := gc.minRunInterval + time.Duration(rand.Int63n(gc.minRunInterval.Nanoseconds()))
		timeSleep(d)
	}
}

// RunOnce performs a single tree garbage collection sweep. Returns the number of successfully
// deleted trees.
//
// It attempts to delete as many eligible trees as possible, regardless of failures. If it
// encounters any failures while deleting the resulting error is non-nil.
func (gc *DeletedTreeGC) RunOnce(ctx context.Context) (int, error) {
	now := timeNow()

	// List and delete trees in separate transactions. Hard-deletes may cascade to a lot of data, so
	// each delete should be in its own transaction as well.
	// It's OK to list and delete separately because HardDelete does its own state checking, plus
	// deleted trees are unlikely to change, specially those deleted for a while.
	trees, err := storage.ListTrees(ctx, gc.admin, true /* includeDeleted */)
	if err != nil {
		return 0, fmt.Errorf("error listing trees: %w", err)
	}

	count := 0
	var errs []error
	for _, tree := range trees {
		if !tree.Deleted {
			continue
		}
		deleteTime, err := ptypes.Timestamp(tree.DeleteTime)
		if err != nil {
			errs = append(errs, fmt.Errorf("error parsing delete_time of tree %v: %w", tree.TreeId, err))
			incHardDeleteCounter(tree.TreeId, false, timestampParseErrReson)
			continue
		}
		durationSinceDelete := now.Sub(deleteTime)
		if durationSinceDelete <= gc.deleteThreshold {
			continue
		}

		glog.Infof("DeletedTreeGC.RunOnce: Hard-deleting tree %v after %v", tree.TreeId, durationSinceDelete)
		if err := storage.HardDeleteTree(ctx, gc.admin, tree.TreeId); err != nil {
			errs = append(errs, fmt.Errorf("error hard-deleting tree %v: %w", tree.TreeId, err))
			incHardDeleteCounter(tree.TreeId, false, deleteErrReason)
			continue
		}

		count++
		incHardDeleteCounter(tree.TreeId, true, "")
	}

	if len(errs) == 0 {
		return count, nil
	}

	buf := &bytes.Buffer{}
	buf.WriteString("encountered errors hard-deleting trees:")
	for _, err := range errs {
		buf.WriteString("\n\t")
		buf.WriteString(err.Error())
	}
	return count, errors.New(buf.String())
}
