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
	"context"
	"math/rand"
	"time"

	"github.com/golang/glog"
	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
)

var (
	timeNow   = time.Now
	timeSleep = time.Sleep
)

// DeletedTreeGC garbage collects deleted trees.
//
// Tree deletion goes through two separate stages:
// * Soft deletion, which "flips a bit" (see Tree.Deleted) but otherwise leaves the tree unaltered
// * Hard deletion, which effectively removes all tree data
//
// DeletedTreeGC performs the transition from soft to hard deletion. Trees that have been deleted
// for at least DeletedThreshold are eligible for garbage collection.
type DeletedTreeGC struct {
	// Admin is the storage.AdminStorage interface.
	Admin storage.AdminStorage

	// DeleteThreshold defines the minimum time a tree has to remain in the soft-deleted state
	// before it's eligible for garbage collection.
	DeleteThreshold time.Duration

	// MinRunInterval defines how frequently sweeps for deleted trees are performed.
	// Actual runs happen randomly between [minInterval,2*minInterval).
	MinRunInterval time.Duration
}

// Run starts the tree garbage collection process. It runs until ctx is cancelled.
func (gc *DeletedTreeGC) Run(ctx context.Context) {
	for true {
		select {
		case <-ctx.Done():
			return
		default:
		}

		gc.RunOnce(ctx)

		d := gc.MinRunInterval + time.Duration(rand.Int63n(gc.MinRunInterval.Nanoseconds()))
		timeSleep(d)
	}
}

// RunOnce performs a single tree garbage collection sweep.
// RunOnce never errors, instead it attempts to delete as many eligible trees as possible. Failures
// are simply logged.
func (gc *DeletedTreeGC) RunOnce(ctx context.Context) {
	now := timeNow()

	// List and delete trees in separate transactions. Hard-deletes may cascade to a lot of data, so
	// each delete should be in its own transaction as well.
	// It's OK to list and delete separately because HardDelete does its own state checking, plus
	// deleted trees are unlikely to change, specially those deleted for a while.
	tx, err := gc.Admin.Snapshot(ctx)
	if err != nil {
		glog.Errorf("DeletedTreeGC.RunOnce: error creating snapshot: %v", err)
		return
	}
	defer tx.Close()
	trees, err := tx.ListTrees(ctx, true /* includeDeleted */)
	if err != nil {
		glog.Errorf("DeletedTreeGC.RunOnce: error listing trees: %v", err)
		return
	}
	if err := tx.Commit(); err != nil {
		glog.Errorf("DeletedTreeGC.RunOnce: error committing snapshot: %v", err)
		return
	}

	for _, tree := range trees {
		if !tree.Deleted {
			continue
		}
		deleteTime, err := ptypes.Timestamp(tree.DeleteTime)
		if err != nil {
			glog.Errorf("DeletedTreeGC.RunOnce: error parsing delete_time of tree %v: %v", tree.TreeId, err)
			continue
		}
		durationSinceDelete := now.Sub(deleteTime)
		if durationSinceDelete <= gc.DeleteThreshold {
			continue
		}

		glog.Infof("DeletedTreeGC.RunOnce: Hard-deleting tree %v after %v", tree.TreeId, durationSinceDelete)
		if err := gc.hardDeleteTree(ctx, tree); err != nil {
			glog.Errorf("DeletedTreeGC.RunOnce: Error hard-deleting tree %v: %v", tree.TreeId, err)
		}
	}
}

func (gc *DeletedTreeGC) hardDeleteTree(ctx context.Context, tree *trillian.Tree) error {
	tx, err := gc.Admin.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Close()
	if err := tx.HardDeleteTree(ctx, tree.TreeId); err != nil {
		return err
	}
	return tx.Commit()
}
