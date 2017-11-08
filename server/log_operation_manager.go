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
	"github.com/google/trillian/util"
)

const (
	minPreElectionPause    = 10 * time.Millisecond
	minMasterCheckInterval = 50 * time.Millisecond
	minMasterHoldInterval  = 10 * time.Second
	logIDLabel             = "logid"
)

var (
	once         sync.Once
	knownLogs    monitoring.Gauge
	resignations monitoring.Counter
	isMaster     monitoring.Gauge
)

func createMetrics(mf monitoring.MetricFactory) {
	if mf == nil {
		mf = monitoring.InertMetricFactory{}
	}
	knownLogs = mf.NewGauge("known_logs", "Set to 1 for known logs (whether this instance is master or not)", logIDLabel)
	resignations = mf.NewCounter("master_resignations", "Number of mastership resignations", logIDLabel)
	isMaster = mf.NewGauge("is_master", "Whether this instance is master (0/1)", logIDLabel)
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
	// PreElectionPause is the maximum interval to wait before starting a
	// mastership election for a particular log.
	PreElectionPause time.Duration
	// MasterCheckInterval is the interval between checks that we still
	// hold mastership for a log.
	MasterCheckInterval time.Duration
	// MasterHoldInterval is the minimum interval to hold mastership for.
	MasterHoldInterval time.Duration
	// ResignOdds gives the chance of resigning mastership after each
	// check interval, as the N for 1-in-N.
	ResignOdds int
	// NumWorkers is the number of worker goroutines to run in parallel.
	NumWorkers int
}

type electionRunner struct {
	logID    int64
	info     *LogOperationInfo
	tracker  *util.MasterTracker
	cancel   context.CancelFunc
	wg       *sync.WaitGroup
	election util.MasterElection
}

func (er *electionRunner) Run(ctx context.Context) {
	defer er.wg.Done()
	label := strconv.FormatInt(er.logID, 10)

	// Pause for a random interval so that if multiple instances start at the same
	// time there is less of a thundering herd.
	pause := rand.Int63n(er.info.PreElectionPause.Nanoseconds())
	time.Sleep(time.Duration(pause))

	glog.V(1).Infof("%d: start election-monitoring loop ", er.logID)
	if err := er.election.Start(ctx); err != nil {
		glog.Errorf("%d: election.Start() failed: %v", er.logID, err)
		return
	}
	defer func(ctx context.Context, er *electionRunner) {
		glog.Infof("%d: shutdown election-monitoring loop", er.logID)
		er.election.Close(ctx)
	}(ctx, er)

	for {
		glog.V(1).Infof("%d: When I left you, I was but the learner", er.logID)
		if err := er.election.WaitForMastership(ctx); err != nil {
			glog.Errorf("%d: er.election.WaitForMastership() failed: %v", er.logID, err)
			return
		}
		glog.V(1).Infof("%d: Now, I am the master", er.logID)
		er.tracker.Set(er.logID, true)
		isMaster.Set(1.0, label)
		masterSince := time.Now()

		// While-master loop
		for {
			time.Sleep(er.info.MasterCheckInterval)
			select {
			case <-ctx.Done():
				glog.Infof("%d: termination requested", er.logID)
				return
			default:
			}
			master, err := er.election.IsMaster(ctx)
			if err != nil {
				glog.Errorf("%d: failed to check mastership status", er.logID)
				break
			}
			if !master {
				glog.Errorf("%d: no longer the master!", er.logID)
				er.tracker.Set(er.logID, false)
				isMaster.Set(0.0, label)
				break
			}
			if er.shouldResign(masterSince) {
				glog.Infof("%d: deliberately resigning mastership", er.logID)
				resignations.Inc(label)
				if err := er.election.ResignAndRestart(ctx); err == nil {
					er.tracker.Set(er.logID, false)
					isMaster.Set(0.0, label)
					break
				}
				glog.Errorf("%d: failed to resign mastership", er.logID)
			}
		}
	}
}

func (er *electionRunner) shouldResign(masterSince time.Time) bool {
	now := er.info.TimeSource.Now()
	duration := now.Sub(masterSince)
	if duration < er.info.MasterHoldInterval {
		// Always hold onto mastership for a minimum interval to prevent churn.
		return false
	}
	// Roll the bones.
	odds := er.info.ResignOdds
	if odds <= 0 {
		return true
	}
	return rand.Intn(er.info.ResignOdds) == 0
}

// LogOperationManager controls scheduling activities for logs.
type LogOperationManager struct {
	info LogOperationInfo

	// logOperation is the task that gets run across active logs in the scheduling loop
	logOperation LogOperation

	// electionRunner tracks the goroutines that run per-log mastership elections
	electionRunner map[int64]*electionRunner
	runnerWG       sync.WaitGroup
	tracker        *util.MasterTracker
	heldMutex      sync.Mutex
	lastHeld       []int64
	// Cache of logID => name; assumed not to change during runtime
	logNamesMutex sync.Mutex
	logNames      map[int64]string
}

// fixupElectionInfo ensures operation parameters have required minimum values.
func fixupElectionInfo(info LogOperationInfo) LogOperationInfo {
	if info.PreElectionPause < minPreElectionPause {
		info.PreElectionPause = minPreElectionPause
	}
	if info.MasterCheckInterval < minMasterCheckInterval {
		info.MasterCheckInterval = minMasterCheckInterval
	}
	if info.MasterHoldInterval < minMasterHoldInterval {
		info.MasterHoldInterval = minMasterHoldInterval
	}
	if info.ResignOdds < 1 {
		info.ResignOdds = 1
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
		electionRunner: make(map[int64]*electionRunner),
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
	tx, err := l.info.Registry.AdminStorage.Snapshot(ctx)
	if err != nil {
		glog.Errorf("%v: failed to start transaction: %v", logID, err)
		return "<err>"
	}
	defer tx.Close()
	tree, err := tx.GetTree(ctx, logID)
	if err != nil {
		glog.Errorf("%v: failed to get log info: %v", logID, err)
		return "<err>"
	}
	if err := tx.Commit(); err != nil {
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
		l.tracker = util.NewMasterTracker(allIDs)
	}

	// Synchronize the set of configured log IDs with those we are tracking mastership for.
	for _, logID := range allIDs {
		knownLogs.Set(1.0, strconv.FormatInt(logID, 10))
		if l.electionRunner[logID] != nil {
			continue
		}
		glog.Infof("create master election goroutine for %v", logID)
		innerCtx, cancel := context.WithCancel(ctx)
		election, err := l.info.Registry.ElectionFactory.NewElection(innerCtx, logID)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to create election for %d: %v", logID, err)
		}
		l.electionRunner[logID] = &electionRunner{
			logID:    logID,
			info:     &l.info,
			tracker:  l.tracker,
			cancel:   cancel,
			wg:       &l.runnerWG,
			election: election,
		}
		l.runnerWG.Add(1)
		go l.electionRunner[logID].Run(innerCtx)
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
				if err != nil {
					glog.Errorf("ExecutePass(%v) failed: %v", logID, err)
					continue
				}

				if count > 0 {
					d := time.Since(start).Seconds()
					glog.Infof("%v: processed %d items in %.2f seconds (%.2f qps)", logID, count, d, float64(count)/d)
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
	d := time.Since(startBatch).Seconds()
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
		start := time.Now()
		if err := l.getLogsAndExecutePass(ctx); err != nil {
			glog.Errorf("failed to execute operation on logs: %v", err)
		}

		glog.V(1).Infof("Log operation manager pass complete")

		// See if it's time to quit
		select {
		case <-ctx.Done():
			glog.Infof("Log operation manager shutting down")
			break loop
		default:
		}

		// Wait for the configured time before going for another pass
		duration := time.Since(start)
		wait := l.info.RunInterval - duration
		if wait > 0 {
			glog.V(1).Infof("Processing started at %v for %v; wait %v before next run", start, duration, wait)
			time.Sleep(wait)
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
		runner.cancel()
	}
	glog.Infof("wait for termination of election runners...")
	l.runnerWG.Wait()
	glog.Infof("wait for termination of election runners...done")
}
