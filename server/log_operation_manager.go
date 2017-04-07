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
	"fmt"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/util"
)

// LogOperation defines a task that operates on a log. Examples are scheduling, signing,
// consistency checking or cleanup.
type LogOperation interface {
	// Name returns the name of the task.
	Name() string
	// ExecutePass performs a single pass of processing on a single log.  It returns
	// a count of items processed (for logging) and an error.
	ExecutePass(ctx context.Context, logID int64, info *LogOperationInfo) (int, error)
}

// LogOperationInfo bundles up information needed for running a set of LogOperations.
type LogOperationInfo struct {
	// registry provides access to Trillian storage
	registry extension.Registry
	// batchSize is the batch size to be passed to tasks run by this manager
	batchSize int
	// runInterval is the time between starting batches of processing.  If a
	// batch takes longer than this interval to complete, the next batch
	// will start immediately.
	runInterval time.Duration
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
func NewLogOperationManager(registry extension.Registry, batchSize, numWorkers int, runInterval time.Duration, timeSource util.TimeSource, logOperation LogOperation) *LogOperationManager {
	return &LogOperationManager{
		info: LogOperationInfo{
			registry:    registry,
			batchSize:   batchSize,
			runInterval: runInterval,
			timeSource:  timeSource,
			numWorkers:  numWorkers,
		},
		logOperation: logOperation,
	}
}

// getLogIDs returns the current set of active log IDs.
func (l LogOperationManager) getLogIDs(ctx context.Context) ([]int64, error) {
	tx, err := l.info.registry.LogStorage.Snapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get tx for retrieving logIDs: %v", err)
	}
	defer tx.Close()

	logIDs, err := tx.GetActiveLogIDs()
	if err != nil {
		return nil, fmt.Errorf("failed to get active logIDs: %v", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit getting logs: %v", err)
	}
	return logIDs, nil
}

func (l LogOperationManager) getLogsAndExecutePass(ctx context.Context) error {
	logIDs, err := l.getLogIDs(ctx)
	if err != nil {
		return err
	}

	numWorkers := l.info.numWorkers
	if numWorkers == 0 {
		glog.Warning("Executing a LogOperation pass with numWorkers == 0, assuming 1")
		numWorkers = 1
	}
	glog.V(1).Infof("Beginning run for %v active log(s) using %d workers", len(logIDs), numWorkers)

	var mu sync.Mutex
	successCount := 0
	itemCount := 0

	// Build a channel of the logIDs that need to be processed.
	toProcess := make(chan int64, len(logIDs))
	for _, logID := range logIDs {
		toProcess <- logID
	}
	close(toProcess)

	// Set off a collection of worker goroutines to process the pending logIDs.
	startBatch := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				logID, more := <-toProcess
				if !more {
					return
				}

				start := time.Now()
				count, err := l.logOperation.ExecutePass(ctx, logID, &l.info)

				if err == nil {
					if count > 0 {
						d := time.Now().Sub(start).Seconds()
						glog.Infof("%v: processed %d items in %.2f seconds (%.2f qps)", logID, count, d, float64(count)/d)
					} else {
						glog.V(1).Infof("%v: no items to process", logID)
					}
					mu.Lock()
					successCount++
					itemCount += count
					mu.Unlock()
				}
			}
		}()
	}

	// Wait for the workers to consume all of the logIDs
	wg.Wait()
	d := time.Now().Sub(startBatch).Seconds()
	glog.V(1).Infof("Group run completed in %.2f seconds: %v succeeded, %v failed, %v items processed", d, successCount, len(logIDs)-successCount, itemCount)

	return nil

}

// OperationSingle performs a single pass of the manager.
func (l LogOperationManager) OperationSingle(ctx context.Context) {
	if err := l.getLogsAndExecutePass(ctx); err != nil {
		glog.Errorf("failed to perform operation: %v", err)
	}
}

// OperationLoop starts the manager working. It continues until told to exit.
// TODO(Martin2112): No mechanism for error reporting etc., this is OK for v1 but needs work
func (l LogOperationManager) OperationLoop(ctx context.Context) {
	glog.Infof("Log operation manager starting")

	// Outer loop, runs until terminated
	for {
		// TODO(alcutter): want a child context with deadline here?
		start := time.Now()
		if err := l.getLogsAndExecutePass(ctx); err != nil {
			glog.Errorf("failed to execute operation on logs: %v", err)
		}

		glog.V(1).Infof("Log operation manager pass complete")

		// See if it's time to quit
		select {
		case <-ctx.Done():
			glog.Infof("Log operation manager shutting down")
			return
		default:
		}

		// Wait for the configured time before going for another pass
		duration := time.Now().Sub(start)
		wait := l.info.runInterval - duration
		if wait > 0 {
			glog.V(1).Infof("Processing started at %v for %v; wait %v before next run", start, duration, wait)
			time.Sleep(wait)
		} else {
			glog.V(1).Infof("Processing started at %v for %v; start next run immediately", start, duration)
		}
	}
}
