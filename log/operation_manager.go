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

// Package log holds the code that is specific to Trillian logs core operation,
// particularly the code for sequencing.
package log

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
	"github.com/google/trillian/util/clock"
	"github.com/google/trillian/util/election"
)

var (
	// DefaultTimeout is the default timeout on a single log operation run.
	DefaultTimeout = 60 * time.Second

	once              sync.Once
	knownLogs         monitoring.Gauge
	resignations      monitoring.Counter
	isMaster          monitoring.Gauge
	signingRuns       monitoring.Counter
	failedSigningRuns monitoring.Counter
	entriesAdded      monitoring.Counter
	batchesAdded      monitoring.Counter
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
	// entriesAdded is the total number of entries that have been added to the
	// log during the lifetime of a signer. This allows an operator to determine
	// that the queue is empty for a particular log if signing runs are succeeding
	// but nothing is being processed then this counter will stop increasing.
	entriesAdded = mf.NewCounter("entries_added", "Number of entries added to the log", logIDLabel)
	// batchesAdded is the number of times a signing run caused entries to be
	// integrated into the log. The value batchesAdded / signingRuns is an
	// indication of how often the signer runs but does no work. The value of
	// entriesAdded / batchesAdded is average batch size. These can be used for
	// tuning sequencing or evaluating performance.
	batchesAdded = mf.NewCounter("batches_added", "Number of times a non zero number of entries was added", logIDLabel)
}

// Operation defines a task that operates on a log. Examples are scheduling, signing,
// consistency checking or cleanup.
type Operation interface {
	// ExecutePass performs a single pass of processing on a single log.  It returns
	// a count of items processed (for logging) and an error.
	ExecutePass(ctx context.Context, logID int64, info *OperationInfo) (int, error)
}

// OperationInfo bundles up information needed for running a set of Operations.
type OperationInfo struct {
	// Registry provides access to Trillian storage.
	Registry extension.Registry

	// The following parameters are passed to individual Operations.

	// BatchSize is the processing batch size to be passed to tasks run by this manager
	BatchSize int
	// TimeSource should be used by the Operation to allow mocking for tests.
	TimeSource clock.TimeSource

	// The following parameters govern the overall scheduling of Operations
	// by a OperationManager.

	// Election-related configuration.
	ElectionConfig election.RunnerConfig

	// RunInterval is the time between starting batches of processing.  If a
	// batch takes longer than this interval to complete, the next batch
	// will start immediately.
	RunInterval time.Duration
	// NumWorkers is the number of worker goroutines to run in parallel.
	NumWorkers int
	// Timeout sets an optional timeout on each operation run.
	// If unset, default to the value of DefaultTimeout.
	Timeout time.Duration
}

// OperationManager controls scheduling activities for logs.
type OperationManager struct {
	info OperationInfo

	// logOperation is the task that gets run across active logs in the scheduling loop
	logOperation Operation

	// electionRunner tracks the goroutines that run per-log mastership elections
	electionRunner      map[string]*election.Runner
	pendingResignations chan election.Resignation
	runnerWG            sync.WaitGroup
	tracker             *election.MasterTracker
	lastHeld            []int64
	// Cache of logID => name; assumed not to change during runtime
	logNamesMutex sync.Mutex
	logNames      map[int64]string
}

// NewOperationManager creates a new OperationManager instance.
func NewOperationManager(info OperationInfo, logOperation Operation) *OperationManager {
	once.Do(func() {
		createMetrics(info.Registry.MetricFactory)
	})
	if info.Timeout == 0 {
		info.Timeout = DefaultTimeout
	}
	return &OperationManager{
		info:                info,
		logOperation:        logOperation,
		electionRunner:      make(map[string]*election.Runner),
		pendingResignations: make(chan election.Resignation, 100),
		logNames:            make(map[int64]string),
	}
}

// getActiveLogIDs returns IDs of all currently active logs, regardless of
// mastership status.
func (o *OperationManager) getActiveLogIDs(ctx context.Context) ([]int64, error) {
	tx, err := o.info.Registry.LogStorage.Snapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction: %v", err)
	}
	defer tx.Close()

	logIDs, err := tx.GetActiveLogIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get active logIDs: %v", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit: %v", err)
	}
	return logIDs, nil
}

// logName maps a logID to a human-readable name, caching results along the way.
// The human-readable name may non-unique so should only be used for diagnostics.
func (o *OperationManager) logName(ctx context.Context, logID int64) string {
	o.logNamesMutex.Lock()
	defer o.logNamesMutex.Unlock()
	if name, ok := o.logNames[logID]; ok {
		return name
	}

	tree, err := storage.GetTree(ctx, o.info.Registry.AdminStorage, logID)
	if err != nil {
		glog.Errorf("%v: failed to get log info: %v", logID, err)
		return "<err>"
	}

	name := tree.DisplayName
	if name == "" {
		name = fmt.Sprintf("<log-%d>", logID)
	}
	o.logNames[logID] = name
	return o.logNames[logID]
}

func (o *OperationManager) heldInfo(ctx context.Context, logIDs []int64) string {
	names := make([]string, 0, len(logIDs))
	for _, logID := range logIDs {
		names = append(names, o.logName(ctx, logID))
	}
	sort.Strings(names)

	result := "master for:"
	for _, name := range names {
		result += " " + name
	}
	return result
}

// masterFor returns the list of log IDs among allIDs that this instance is
// master for. Note that the instance may hold mastership for logs that are not
// listed in allIDs, but such logs are skipped.
func (o *OperationManager) masterFor(ctx context.Context, allIDs []int64) ([]int64, error) {
	if o.info.Registry.ElectionFactory == nil {
		return allIDs, nil
	}
	allStringIDs := make([]string, 0, len(allIDs))
	for _, id := range allIDs {
		s := strconv.FormatInt(id, 10)
		allStringIDs = append(allStringIDs, s)
	}
	if o.tracker == nil {
		glog.Infof("creating mastership tracker for %v", allIDs)
		o.tracker = election.NewMasterTracker(allStringIDs, func(id string, v bool) {
			val := 0.0
			if v {
				val = 1.0
			}
			isMaster.Set(val, id)
		})
	}

	// Synchronize the set of log IDs with those we are tracking mastership for.
	for _, logID := range allStringIDs {
		knownLogs.Set(1, logID)
		if o.electionRunner[logID] != nil {
			continue
		}
		glog.Infof("create master election goroutine for %v", logID)
		innerCtx, cancel := context.WithCancel(ctx)
		el, err := o.info.Registry.ElectionFactory.NewElection(innerCtx, logID)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to create election for %v: %v", logID, err)
		}
		o.electionRunner[logID] = election.NewRunner(logID, &o.info.ElectionConfig, o.tracker, cancel, el)
		o.runnerWG.Add(1)
		go func(r *election.Runner) {
			defer o.runnerWG.Done()
			r.Run(innerCtx, o.pendingResignations)
		}(o.electionRunner[logID])
	}

	held := o.tracker.Held()
	heldIDs := make([]int64, 0, len(allIDs))
	sort.Strings(allStringIDs)
	for _, s := range held {
		// Skip the log if it is not present in allIDs.
		if i := sort.SearchStrings(allStringIDs, s); i >= len(allStringIDs) || allStringIDs[i] != s {
			continue
		}
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse logID %v as int64", s)
		}
		heldIDs = append(heldIDs, id)
	}

	return heldIDs, nil
}

// updateHeldIDs updates the process status with the number/list of logs that
// the instance holds mastership for.
func (o *OperationManager) updateHeldIDs(ctx context.Context, logIDs, activeIDs []int64) {
	heldInfo := o.heldInfo(ctx, logIDs)
	msg := fmt.Sprintf("Acting as master for %d / %d active logs: %s", len(logIDs), len(activeIDs), heldInfo)
	if !reflect.DeepEqual(logIDs, o.lastHeld) {
		o.lastHeld = make([]int64, len(logIDs))
		copy(o.lastHeld, logIDs)
		glog.Info(msg)
		if o.info.Registry.SetProcessStatus != nil {
			o.info.Registry.SetProcessStatus(heldInfo)
		}
	} else {
		glog.V(1).Info(msg)
	}
}

func (o *OperationManager) getLogsAndExecutePass(ctx context.Context) error {
	runCtx, cancel := context.WithTimeout(ctx, o.info.Timeout)
	defer cancel()

	activeIDs, err := o.getActiveLogIDs(runCtx)
	if err != nil {
		return fmt.Errorf("failed to list active log IDs: %v", err)
	}
	// Find the logs we are master for, skipping those logs that are not active,
	// e.g. deleted or FROZEN ones.
	// TODO(pavelkalinnikov): Resign mastership for the inactive logs.
	logIDs, err := o.masterFor(ctx, activeIDs)
	if err != nil {
		return fmt.Errorf("failed to determine log IDs we're master for: %v", err)
	}
	o.updateHeldIDs(ctx, logIDs, activeIDs)

	// TODO(pavelkalinnikov): Run executor once instead of doing it on each pass.
	// This will be also needed when factoring out per-log operation loop.
	ex := newExecutor(o.logOperation, &o.info, len(logIDs))
	// Put logIDs that need to be processed to the executor's channel.
	for _, logID := range logIDs {
		ex.jobs <- logID
	}
	close(ex.jobs) // Cause executor's run to terminate when it has drained the jobs.
	ex.run(runCtx)
	return nil
}

// OperationSingle performs a single pass of the manager.
func (o *OperationManager) OperationSingle(ctx context.Context) {
	if err := o.getLogsAndExecutePass(ctx); err != nil {
		glog.Errorf("failed to perform operation: %v", err)
	}
}

// OperationLoop starts the manager working. It continues until told to exit.
// TODO(Martin2112): No mechanism for error reporting etc., this is OK for v1 but needs work
func (o *OperationManager) OperationLoop(ctx context.Context) {
	glog.Infof("Log operation manager starting")

	// Outer loop, runs until terminated
loop:
	for {
		// TODO(alcutter): want a child context with deadline here?
		start := o.info.TimeSource.Now()
		if err := o.getLogsAndExecutePass(ctx); err != nil {
			// Suppress the error if ctx is done (ctx.Err != nil) as we're exiting.
			if ctx.Err() != nil {
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
			case r := <-o.pendingResignations:
				resignations.Inc(r.ID)
				r.Execute(ctx)
			default:
				doneResigning = true
			}
		}

		// Wait for the configured time before going for another pass
		duration := o.info.TimeSource.Now().Sub(start)
		wait := o.info.RunInterval - duration
		if wait > 0 {
			glog.V(1).Infof("Processing started at %v for %v; wait %v before next run", start, duration, wait)
			if err := clock.SleepContext(ctx, wait); err != nil {
				glog.Infof("Log operation manager shutting down")
				break loop
			}
		} else {
			glog.V(1).Infof("Processing started at %v for %v; start next run immediately", start, duration)
		}

	}

	// Terminate all the election runners
	for logID, runner := range o.electionRunner {
		if runner == nil {
			continue
		}
		glog.V(1).Infof("cancel election runner for %s", logID)
		runner.Cancel()
	}
	glog.Infof("wait for termination of election runners...")
	o.runnerWG.Wait()
	glog.Infof("wait for termination of election runners...done")
}

// logOperationExecutor runs the specified Operation on the submitted logs
// in a set of parallel workers.
type logOperationExecutor struct {
	op   Operation
	info *OperationInfo

	// jobs holds logIDs to run log operation on.
	// TODO(pavelkalinnikov): Use mastership context for each job to make them
	// auto-cancelable when mastership is lost.
	// TODO(pavelkalinnikov): Report job completion status back.
	jobs chan int64
}

func newExecutor(op Operation, info *OperationInfo, jobs int) *logOperationExecutor {
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
					d := clock.SecondsSince(e.info.TimeSource, start)
					glog.Infof("%v: processed %d items in %.2f seconds (%.2f qps)", logID, count, d, float64(count)/d)
					entriesAdded.Add(float64(count), label)
					batchesAdded.Inc(label)
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
	d := clock.SecondsSince(e.info.TimeSource, startBatch)
	if itemCount > 0 {
		glog.Infof("Group run completed in %.2f seconds: %v succeeded, %v failed, %v items processed", d, successCount, failCount, itemCount)
	} else {
		glog.V(1).Infof("Group run completed in %.2f seconds: no items to process", d)
	}
}
