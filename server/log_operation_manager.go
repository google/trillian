// Copyright 2016 Google Inc. All Rights Reserved.
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
	"context"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/util"
)

// LogOperation defines a task that operates on logs. Examples are scheduling, signing,
// consistency checking or cleanup.
type LogOperation interface {
	// Name returns the name of the task.
	Name() string
	// ExecutePass performs a single pass of processing on a set of logs.
	ExecutePass(ctx context.Context, logIDs []int64, info *LogOperationInfo)
}

// LogOperationInfo bundles up information needed for running a LogOperation.
type LogOperationInfo struct {
	// registry provides access to Trillian storage
	registry extension.Registry
	// batchSize is the batch size to be passed to tasks run by this manager
	batchSize int
	// sleepBetweenRuns is the time to pause after all active logs have processed a batch
	sleepBetweenRuns time.Duration
	// timeSource allows us to mock this in tests
	timeSource util.TimeSource
	// numWorkers is the number of worker goroutines to run in parallel.
	numWorkers int
}

// LogOperationManager controls scheduling activities for logs. At the moment it's very simple
// with a single task running over active logs one at a time. This will be expanded later.
// This is meant for embedding into the actual operation implementations and should not
// be created separately.
type LogOperationManager struct {
	info LogOperationInfo
	// logOperation is the task that gets run across active logs in the scheduling loop
	logOperation LogOperation
}

// NewLogOperationManager creates a new LogOperationManager instance.
func NewLogOperationManager(registry extension.Registry, batchSize, numWorkers int, sleepBetweenRuns time.Duration, timeSource util.TimeSource, logOperation LogOperation) *LogOperationManager {
	return &LogOperationManager{
		info: LogOperationInfo{
			registry:         registry,
			batchSize:        batchSize,
			sleepBetweenRuns: sleepBetweenRuns,
			timeSource:       timeSource,
			numWorkers:       numWorkers,
		},
		logOperation: logOperation,
	}
}

func (l LogOperationManager) getLogsAndExecutePass(ctx context.Context) bool {
	tx, err := l.info.registry.LogStorage.Snapshot(ctx)
	if err != nil {
		glog.Warningf("Failed to get tx for run: %v", err)
		return false
	}
	defer tx.Close()

	// Inner loop is across all active logs, currently one at a time
	logIDs, err := tx.GetActiveLogIDs()
	if err != nil {
		glog.Warningf("Failed to get log list for run: %v", err)
		return false
	}

	if err := tx.Commit(); err != nil {
		glog.Warningf("Failed to commit getting logs: %v", err)
		return false
	}

	// Process each active log once.
	l.logOperation.ExecutePass(ctx, logIDs, &l.info)

	// See if it's time to quit
	select {
	case <-ctx.Done():
		return true
	default:
	}
	return false
}

// OperationSingle performs a single pass of the manager.
func (l LogOperationManager) OperationSingle(ctx context.Context) {
	l.getLogsAndExecutePass(ctx)
}

// OperationLoop starts the manager working. It continues until told to exit.
// TODO(Martin2112): No mechanism for error reporting etc., this is OK for v1 but needs work
func (l LogOperationManager) OperationLoop(ctx context.Context) {
	glog.Infof("Log operation manager starting")

	// Outer loop, runs until terminated
	for {
		// TODO(alcutter): want a child context with deadline here?
		quit := l.getLogsAndExecutePass(ctx)

		glog.V(1).Infof("Log operation manager pass complete")

		// We might want to bail out early when testing
		if quit {
			glog.Infof("Log operation manager shutting down")
			return
		}

		// Wait for the configured time before going for another pass
		time.Sleep(l.info.sleepBetweenRuns)
	}
}
