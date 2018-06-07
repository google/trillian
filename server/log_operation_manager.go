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
	"math/rand"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/util"
	"github.com/google/trillian/util/election"
)

const (
	minPreElectionPause    = 10 * time.Millisecond
	minMasterCheckInterval = 50 * time.Millisecond
	minMasterHoldInterval  = 10 * time.Second
	logIDLabel             = "logid"
)

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

	// RunInterval is the time between starting batches of processing.  If a
	// batch takes longer than this interval to complete, the next batch
	// will start immediately.
	RunInterval time.Duration

	// ElectionConfig configures per-log master election process.
	ElectionConfig election.Config
	// MasterHoldInterval is the minimum interval to hold mastership for.
	MasterHoldInterval time.Duration
	// ResignSpread is the max extra number of master check intervals to keep
	// mastership for. The master will resign in:
	//   MasterHoldInterval + rand(ResignSpread) * MasterCheckInverval.
	ResignSpread int

	// NumWorkers is the number of worker goroutines to run in parallel.
	NumWorkers int
}

// getResignDelay computes master resignation delay based on operation
// parameters.
func getResignDelay(info *LogOperationInfo) time.Duration {
	delay := info.MasterHoldInterval
	if info.ResignSpread <= 0 {
		return delay
	}
	mult := rand.Float64() * float64(info.ResignSpread)
	add := mult * float64(info.ElectionConfig.MasterCheckInterval)
	return delay + time.Duration(add)
}

// LogOperationManager controls scheduling activities for logs.
type LogOperationManager struct {
	info LogOperationInfo

	// logOperation is the task that gets run across active logs in the scheduling loop
	logOperation LogOperation

	// electionRunner tracks the goroutines that run per-log mastership elections
	electionRunner map[int64]*election.Runner
	resignations   chan func()
	runnerWG       sync.WaitGroup
	tracker        *election.MasterTracker
	heldMutex      sync.Mutex
	lastHeld       []int64
	// Cache of logID => name; assumed not to change during runtime
	logNamesMutex sync.Mutex
	logNames      map[int64]string
}

// fixupElectionInfo ensures election parameters have required minimum values.
func fixupElectionInfo(info LogOperationInfo) LogOperationInfo {
	cfg := &info.ElectionConfig
	if cfg.PreElectionPause < minPreElectionPause {
		cfg.PreElectionPause = minPreElectionPause
	}
	if cfg.MasterCheckInterval < minMasterCheckInterval {
		cfg.MasterCheckInterval = minMasterCheckInterval
	}
	if info.MasterHoldInterval < minMasterHoldInterval {
		info.MasterHoldInterval = minMasterHoldInterval
	}
	if info.ResignSpread <= 0 {
		info.ResignSpread = 0
	}
	return info
}

// NewLogOperationManager creates a new LogOperationManager instance.
func NewLogOperationManager(info LogOperationInfo, logOperation LogOperation) *LogOperationManager {
	once.Do(func() {
		createMetrics(info.Registry.MetricFactory)
	})
	return &LogOperationManager{
		info:           fixupElectionInfo(info),
		logOperation:   logOperation,
		electionRunner: make(map[int64]*election.Runner),
		resignations:   make(chan func(), 100),
		logNames:       make(map[int64]string),
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

// TODO(pavelkalinnikov): Make each log track their own mastership status
// directly during their operation loop and take actions on status updates
// without having to distribute them to the common bottleneck.
func (l *LogOperationManager) masterFor(ctx context.Context, allIDs []int64) ([]int64, error) {
	if l.info.Registry.ElectionFactory == nil {
		return allIDs, nil
	}
	if l.tracker == nil {
		glog.Infof("creating mastership tracker for %v", allIDs)
		l.tracker = election.NewMasterTracker(allIDs)
	}

	// Synchronize the set of configured log IDs with those we are tracking
	// mastership for.
	for _, logID := range allIDs {
		label := strconv.FormatInt(logID, 10)
		knownLogs.Set(1.0, label)
		if l.electionRunner[logID] != nil {
			continue
		}
		glog.Infof("create master election goroutine for %v", logID)
		el, err := l.info.Registry.ElectionFactory.NewElection(ctx, logID)
		if err != nil {
			return nil, fmt.Errorf("failed to create election for %d: %v", logID, err)
		}

		runner := election.NewRunner(&l.info.ElectionConfig, el, l.info.TimeSource, label+": ")
		l.electionRunner[logID] = runner

		//   beMaster := func(ctx context.Context) error {
		//     if err := runner.Start(ctx); err != nil {
		//       return err
		//     }
		//     mctx = runner.MasterContext()
		//     defer runner.Stop()
		//
		//     for { // While we are the master.
		//       // Note: The work will be canceled if mastership is suddenly over.
		//       if err := DoSomeWorkAsMaster(mctx); err != nil {
		//         // Return if mctx is done, or report err and retry if possible.
		//       }
		//       if ShouldResign() {
		//         return runner.Resign(ctx)
		//       }
		//     }
		//   }

		l.runnerWG.Add(1)
		go func(logID int64) {
			defer l.runnerWG.Done()
			defer runner.Close(ctx)

			for {
				if err := runner.Start(ctx); err != nil {
					glog.Errorf("%d: failed to become the master: %v", logID, err)
					// TODO(pavelkalinnikov): Retry while it's not context cancelation.
					return
				}
				start := l.info.TimeSource.Now()

				glog.Infof("%d: holding mastership of the log", logID)
				l.tracker.Set(logID, true)
				isMaster.Set(1.0, label)

				resign := true
				delay := getResignDelay(&l.info)
				// Note: The loop is to allow blocking by a mocked TimeSource.
				for until := start.Add(delay); l.info.TimeSource.Now().Before(until); {
					if util.SleepContext(runner.MasterContext(), delay) != nil {
						// Lost mastership or canceled.
						resign = false
						break
					}
				}

				glog.Warningf("%d: no longer the master", logID)
				l.tracker.Set(logID, false)
				isMaster.Set(0.0, label)
				if resign {
					resignations.Inc(label)
					l.resignations <- func() { runner.Resign(ctx) }
					runner.Wait()
				}
			}
		}(logID)
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

// TODO(pavelkalinnikov): Consider operating each log individually. Currently,
// if a more heavily updated log takes significant time to complete a pass, all
// lightweight logs will have to wait for it.
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

	numWorkers := l.info.NumWorkers
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

	// Set off a collection of transient worker goroutines to process the pending logIDs.
	startBatch := l.info.TimeSource.Now()
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

				label := strconv.FormatInt(logID, 10)
				start := l.info.TimeSource.Now()
				count, err := l.logOperation.ExecutePass(ctx, logID, &l.info)
				if err != nil {
					glog.Errorf("ExecutePass(%v) failed: %v", logID, err)
					failedSigningRuns.Inc(label)
					continue
				}

				// This indicates signing activity is proceeding on the logID.
				signingRuns.Inc(label)
				if count > 0 {
					d := util.SecondsSince(l.info.TimeSource, start)
					glog.Infof("%v: processed %d items in %.2f seconds (%.2f qps)", logID, count, d, float64(count)/d)
					// This allows an operator to determine that the queue is empty
					// for a particular log if signing runs are succeeding but nothing
					// is being processed then this counter will stop increasing.
					entriesAdded.Add(float64(count), label)
				} else {
					glog.V(1).Infof("%v: no items to process", logID)
				}
				mu.Lock()
				successCount++
				itemCount += count
				mu.Unlock()
			}
		}()
	}

	// Wait for the workers to consume all of the logIDs
	wg.Wait()
	d := util.SecondsSince(l.info.TimeSource, startBatch)
	glog.Infof("Group run completed in %.2f seconds: %v succeeded, %v failed, %v items processed", d, successCount, len(logIDs)-successCount, itemCount)

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
		for done := false; !done; {
			select {
			case cancel := <-l.resignations:
				cancel()
			default:
				done = true
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

	// Election runners will terminate due to ctx canceling.
	glog.Infof("wait for termination of election runners...")
	l.runnerWG.Wait()
	glog.Infof("wait for termination of election runners...done")
}
