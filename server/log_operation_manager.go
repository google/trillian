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
	"reflect"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/util"
	"github.com/google/trillian/util/election"
)

const logIDLabel = "logid"

var (
	once              sync.Once
	knownLogs         monitoring.Gauge
	resignations      monitoring.Counter
	isMaster          monitoring.Gauge
	signingRuns       monitoring.Counter
	failedSigningRuns monitoring.Counter
	entriesAdded      monitoring.Counter
)

func createMetrics(mf monitoring.MetricFactory) {
	if mf == nil {
		mf = monitoring.InertMetricFactory{}
	}
	knownLogs = mf.NewGauge("known_logs", "Set to 1 for known logs (whether this instance is master or not)", logIDLabel)
	resignations = mf.NewCounter("master_resignations", "Number of mastership resignations", logIDLabel)
	isMaster = mf.NewGauge("is_master", "Whether this instance is master (0/1)", logIDLabel)
	signingRuns = mf.NewCounter("signing_runs", "Number of times a signing run has succeeded", logIDLabel)
	failedSigningRuns = mf.NewCounter("failed_signing_runs", "Number of times a signing run has failed", logIDLabel)
	entriesAdded = mf.NewCounter("entries_added", "Number of entries added to the log", logIDLabel)
}

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
	// Registry provides access to Trillian storage.
	Registry extension.Registry

	// The following parameters are passed to individual LogOperations.

	// BatchSize is the processing batch size to be passed to tasks run by this manager
	BatchSize int
	// TimeSource should be used by the LogOperation to allow mocking for tests.
	TimeSource util.TimeSource

	// The following parameters govern the overall scheduling of LogOperations
	// by a LogOperationManager.

	// Election-related configuration.
	ElectionConfig election.RunnerConfig

	// RunInterval is the time between starting batches of processing.  If a
	// batch takes longer than this interval to complete, the next batch
	// will start immediately.
	RunInterval time.Duration
	// NumWorkers is the number of worker goroutines to run in parallel.
	NumWorkers int
}

// LogOperationManager controls scheduling activities for logs.
type LogOperationManager struct {
	info LogOperationInfo

	// logOperation is the task that gets run across active logs in the scheduling loop
	logOperation LogOperation

	// electionRunner tracks the goroutines that run per-log mastership elections
	electionRunner      map[int64]*election.Runner
	pendingResignations chan election.Resignation
	runnerWG            sync.WaitGroup
	tracker             *election.MasterTracker
	heldMutex           sync.Mutex
	lastHeld            []int64
	// Cache of logID => name; assumed not to change during runtime
	logNamesMutex sync.Mutex
	logNames      map[int64]string
}

// NewLogOperationManager creates a new LogOperationManager instance.
func NewLogOperationManager(info LogOperationInfo, logOperation LogOperation) *LogOperationManager {
	once.Do(func() {
		createMetrics(info.Registry.MetricFactory)
	})
	return &LogOperationManager{
		info:                info,
		logOperation:        logOperation,
		electionRunner:      make(map[int64]*election.Runner),
		pendingResignations: make(chan election.Resignation, 100),
		logNames:            make(map[int64]string),
	}
}

// getLogIDs returns the current set of active log IDs, whether we are master for them or not.
func (l *LogOperationManager) getLogIDs(ctx context.Context) ([]int64, error) {
	tx, err := l.info.Registry.LogStorage.Snapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get tx for retrieving logIDs: %v", err)
	}
	defer tx.Close()

	logIDs, err := tx.GetActiveLogIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get active logIDs: %v", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit getting logs: %v", err)
	}
	return logIDs, nil
}

// logName maps a logID to a human-readable name, caching results along the way.
// The human-readable name may non-unique so should only be used for diagnostics.
func (l *LogOperationManager) logName(ctx context.Context, logID int64) string {
	l.logNamesMutex.Lock()
	defer l.logNamesMutex.Unlock()
	if name, ok := l.logNames[logID]; ok {
		return name
	}

	tree, err := storage.GetTree(ctx, l.info.Registry.AdminStorage, logID)
	if err != nil {
		glog.Errorf("%v: failed to get log info: %v", logID, err)
		return "<err>"
	}

	name := tree.DisplayName
	if name == "" {
		name = fmt.Sprintf("<log-%d>", logID)
	}
	l.logNames[logID] = name
	return l.logNames[logID]
}

func (l *LogOperationManager) heldInfo(ctx context.Context, logIDs []int64) string {
	names := make([]string, 0, len(logIDs))
	for _, logID := range logIDs {
		names = append(names, l.logName(ctx, logID))
	}
	sort.Strings(names)

	result := "master for:"
	for _, name := range names {
		result += " " + name
	}
	return result
}

func (l *LogOperationManager) masterFor(ctx context.Context, allIDs []int64) ([]int64, error) {
	if l.info.Registry.ElectionFactory == nil {
		return allIDs, nil
	}
	if l.tracker == nil {
		glog.Infof("creating mastership tracker for %v", allIDs)
		l.tracker = election.NewMasterTracker(allIDs, func(id int64, v bool) {
			val := 0.0
			if v {
				val = 1.0
			}
			isMaster.Set(val, strconv.FormatInt(id, 10))
		})
	}

	// Synchronize the set of configured log IDs with those we are tracking mastership for.
	for _, logID := range allIDs {
		knownLogs.Set(1.0, strconv.FormatInt(logID, 10))
		if l.electionRunner[logID] != nil {
			continue
		}
		glog.Infof("create master election goroutine for %v", logID)
		innerCtx, cancel := context.WithCancel(ctx)
		el, err := l.info.Registry.ElectionFactory.NewElection(innerCtx, logID)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to create election for %d: %v", logID, err)
		}
		l.electionRunner[logID] = election.NewRunner(logID, &l.info.ElectionConfig, l.tracker, cancel, el)
		l.runnerWG.Add(1)
		go func(r *election.Runner) {
			defer l.runnerWG.Done()
			r.Run(innerCtx, l.pendingResignations)
		}(l.electionRunner[logID])
	}

	held := l.tracker.Held()
	glog.V(1).Infof("acting as master for %d / %d: %s", len(held), len(allIDs), l.tracker)
	return held, nil
}

func (l *LogOperationManager) updateHeldIDs(ctx context.Context, logIDs, allIDs []int64) {
	l.heldMutex.Lock()
	defer l.heldMutex.Unlock()
	if !reflect.DeepEqual(logIDs, l.lastHeld) {
		l.lastHeld = make([]int64, len(logIDs))
		copy(l.lastHeld, logIDs)
		heldInfo := l.heldInfo(ctx, logIDs)
		glog.Infof("now acting as master for %d / %d, %s", len(logIDs), len(allIDs), heldInfo)
		if l.info.Registry.SetProcessStatus != nil {
			l.info.Registry.SetProcessStatus(heldInfo)
		}
	}
}

func (l *LogOperationManager) getLogsAndExecutePass(ctx context.Context) error {
	allIDs, err := l.getLogIDs(ctx)
	if err != nil {
		return fmt.Errorf("failed to retrieve full list of log IDs: %v", err)
	}
	logIDs, err := l.masterFor(ctx, allIDs)
	if err != nil {
		return fmt.Errorf("failed to determine log IDs we're master for: %v", err)
	}
	l.updateHeldIDs(ctx, logIDs, allIDs)
	glog.V(1).Infof("Beginning run for %v active log(s)", len(logIDs))

	// TODO(pavelkalinnikov): Run executor once instead of doing it on each pass.
	// This will be also needed when factoring out per-log operation loop.
	ex := newExecutor(l.logOperation, &l.info, len(logIDs))
	// Put logIDs that need to be processed to the executor's channel.
	for _, logID := range logIDs {
		ex.jobs <- logID
	}
	close(ex.jobs) // Cause executor's run to terminate when it has drained the jobs.
	ex.run(ctx)
	return nil
}

// OperationSingle performs a single pass of the manager.
func (l *LogOperationManager) OperationSingle(ctx context.Context) {
	if err := l.getLogsAndExecutePass(ctx); err != nil {
		glog.Errorf("failed to perform operation: %v", err)
	}
}

// OperationLoop starts the manager working. It continues until told to exit.
// TODO(Martin2112): No mechanism for error reporting etc., this is OK for v1 but needs work
func (l *LogOperationManager) OperationLoop(ctx context.Context) {
	glog.Infof("Log operation manager starting")

	// Outer loop, runs until terminated
loop:
	for {
		// TODO(alcutter): want a child context with deadline here?
		start := l.info.TimeSource.Now()
		if err := l.getLogsAndExecutePass(ctx); err != nil {
			// Suppress the error if ctx is done (ok==false) as we're exiting.
			if _, ok := <-ctx.Done(); ok {
				glog.Errorf("failed to execute operation on logs: %v", err)
			}
		}
		glog.V(1).Infof("Log operation manager pass complete")

		// See if it's time to quit
		select {
		case <-ctx.Done():
			glog.Infof("Log operation manager shutting down")
			break loop
		default:
		}

		// Process any pending resignations while there's no activity.
		doneResigning := false
		for !doneResigning {
			select {
			case r := <-l.pendingResignations:
				resignations.Inc(strconv.FormatInt(r.ID, 10))
				r.Execute(ctx)
			default:
				doneResigning = true
			}
		}

		// Wait for the configured time before going for another pass
		duration := l.info.TimeSource.Now().Sub(start)
		wait := l.info.RunInterval - duration
		if wait > 0 {
			glog.V(1).Infof("Processing started at %v for %v; wait %v before next run", start, duration, wait)
			if err := util.SleepContext(ctx, wait); err != nil {
				glog.Infof("Log operation manager shutting down")
				break loop
			}
		} else {
			glog.V(1).Infof("Processing started at %v for %v; start next run immediately", start, duration)
		}

	}

	// Terminate all the election runners
	for logID, runner := range l.electionRunner {
		if runner == nil {
			continue
		}
		glog.V(1).Infof("cancel election runner for %d", logID)
		runner.Cancel()
	}
	glog.Infof("wait for termination of election runners...")
	l.runnerWG.Wait()
	glog.Infof("wait for termination of election runners...done")
}

// logOperationExecutor runs the specified LogOperation on the submitted logs
// in a set of parallel workers.
type logOperationExecutor struct {
	op   LogOperation
	info *LogOperationInfo

	// jobs holds logIDs to run log operation on.
	// TODO(pavelkalinnikov): Use mastership context for each job to make them
	// auto-cancelable when mastership is lost.
	// TODO(pavelkalinnikov): Report job completion status back.
	jobs chan int64
}

func newExecutor(op LogOperation, info *LogOperationInfo, jobs int) *logOperationExecutor {
	if jobs < 0 {
		jobs = 0
	}
	return &logOperationExecutor{op: op, info: info, jobs: make(chan int64, jobs)}
}

// run sets off a collection of transient worker goroutines which process the
// pending log operation jobs until the jobs channel is closed.
func (e *logOperationExecutor) run(ctx context.Context) {
	startBatch := e.info.TimeSource.Now()

	numWorkers := e.info.NumWorkers
	if numWorkers <= 0 {
		glog.Warning("Running executor with NumWorkers <= 0, assuming 1")
		numWorkers = 1
	}
	glog.V(1).Infof("Running executor with %d worker(s)", numWorkers)

	var wg sync.WaitGroup
	var successCount, failCount, itemCount int64

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				logID, ok := <-e.jobs
				if !ok {
					return
				}

				label := strconv.FormatInt(logID, 10)
				start := e.info.TimeSource.Now()
				count, err := e.op.ExecutePass(ctx, logID, e.info)
				if err != nil {
					glog.Errorf("ExecutePass(%v) failed: %v", logID, err)
					failedSigningRuns.Inc(label)
					atomic.AddInt64(&failCount, 1)
					continue
				}

				// This indicates signing activity is proceeding on the logID.
				signingRuns.Inc(label)
				if count > 0 {
					d := util.SecondsSince(e.info.TimeSource, start)
					glog.Infof("%v: processed %d items in %.2f seconds (%.2f qps)", logID, count, d, float64(count)/d)
					// This allows an operator to determine that the queue is empty for a
					// particular log if signing runs are succeeding but nothing is being
					// processed then this counter will stop increasing.
					entriesAdded.Add(float64(count), label)
				} else {
					glog.V(1).Infof("%v: no items to process", logID)
				}

				atomic.AddInt64(&successCount, 1)
				atomic.AddInt64(&itemCount, int64(count))
			}
		}()
	}

	// Wait for the workers to consume all of the logIDs.
	wg.Wait()
	d := util.SecondsSince(e.info.TimeSource, startBatch)
	glog.Infof("Group run completed in %.2f seconds: %v succeeded, %v failed, %v items processed", d, successCount, failCount, itemCount)
}
