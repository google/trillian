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
	ExecutePass(logIDs []int64, context LogOperationManagerContext)
}

// LogOperationManagerContext bundles up the values so testing can be made easier
type LogOperationManagerContext struct {
	// ctx is general context for cancellation and diagnostic info
	ctx context.Context
	// registry provides access to Trillian storage
	registry extension.Registry
	// batchSize is the batch size to be passed to tasks run by this manager
	batchSize int
	// sleepBetweenRuns is the time to pause after all active logs have processed a batch
	sleepBetweenRuns time.Duration
	// oneShot is for use by tests only, it exits after one pass
	oneShot bool
	// timeSource allows us to mock this in tests
	timeSource util.TimeSource
	// numSequencers is the number of sequencers to run in parallel.
	numSequencers int
}

// LogOperationManager controls scheduling activities for logs. At the moment it's very simple
// with a single task running over active logs one at a time. This will be expanded later.
// This is meant for embedding into the actual operation implementations and should not
// be created separately.
type LogOperationManager struct {
	context LogOperationManagerContext
	// logOperation is the task that gets run across active logs in the scheduling loop
	logOperation LogOperation
}

// NewLogOperationManager creates a new LogOperationManager instance.
func NewLogOperationManager(ctx context.Context, registry extension.Registry, batchSize, numSequencers int, sleepBetweenRuns time.Duration, timeSource util.TimeSource, logOperation LogOperation) *LogOperationManager {
	return &LogOperationManager{
		context: LogOperationManagerContext{
			ctx:              ctx,
			registry:         registry,
			batchSize:        batchSize,
			sleepBetweenRuns: sleepBetweenRuns,
			timeSource:       timeSource,
			numSequencers:    numSequencers,
		},
		logOperation: logOperation,
	}
}

// NewLogOperationManagerForTest creates a one-shot LogOperationManager instance, for use by tests only.
func NewLogOperationManagerForTest(ctx context.Context, registry extension.Registry, batchSize int, sleepBetweenRuns time.Duration, timeSource util.TimeSource, logOperation LogOperation) *LogOperationManager {
	return &LogOperationManager{
		context: LogOperationManagerContext{
			ctx:              ctx,
			registry:         registry,
			batchSize:        batchSize,
			sleepBetweenRuns: sleepBetweenRuns,
			timeSource:       timeSource,
			oneShot:          true,
			numSequencers:    5,
		},
		logOperation: logOperation,
	}
}

func (l LogOperationManager) getLogsAndExecutePass(ctx context.Context) bool {
	provider, err := l.context.registry.GetLogStorage()
	// If we get an error, we can't do anything but wait until the next run through
	if err != nil {
		glog.Warningf("Failed to get storage provider for run: %v", err)
		return false
	}

	tx, err := provider.Snapshot(ctx)
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
	l.logOperation.ExecutePass(logIDs, l.context)

	// See if it's time to quit
	select {
	case <-ctx.Done():
		return true
	default:
	}
	return false
}

// OperationSingle performs a single pass of the manager.
func (l LogOperationManager) OperationSingle() {
	l.getLogsAndExecutePass(l.context.ctx)
}

// OperationLoop starts the manager working. It continues until told to exit.
// TODO(Martin2112): No mechanism for error reporting etc., this is OK for v1 but needs work
func (l LogOperationManager) OperationLoop() {
	glog.Infof("Log operation manager starting")

	// Outer loop, runs until terminated
	for {
		// TODO(alcutter): want a child context with deadline here?
		quit := l.getLogsAndExecutePass(l.context.ctx)

		glog.V(1).Infof("Log operation manager pass complete")

		// We might want to bail out early when testing
		if quit || l.context.oneShot {
			glog.Infof("Log operation manager shutting down")
			return
		}

		// Wait for the configured time before going for another pass
		time.Sleep(l.context.sleepBetweenRuns)
	}
}
