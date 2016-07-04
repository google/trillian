package server

import (
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/util"
)

// logOperationManager controls scheduling activities for logs. At the moment it's very simple
// with a single task running over active logs one at a time. This will be expanded later.
// This is meant for embedding into the actual operation implementations and should not
// be created separately.
type logOperationManager struct {
	// done is a channel that provides an exit signal
	done chan struct{}
	// storageProvider is the log storage provider used to get active logs
	storageProvider LogStorageProviderFunc
	// batchSize is the batch size to be passed to tasks run by this manager
	batchSize int
	// sleepBetweenLogs is the time to pause after each batch
	sleepBetweenLogs time.Duration
	// sleepBetweenRuns is the time to pause after all active logs have processed a batch
	sleepBetweenRuns time.Duration
	// runLimit is a limit on the number of passes. It can only be set for tests
	runLimit int
	// timeSource allows us to mock this in tests
	timeSource util.TimeSource
}

func (l logOperationManager) runOperationPass([]trillian.LogID) bool {
	return false
}

// OperationLoop starts the manager working. It continues until told to exit.
// TODO(Martin2112): No mechanism for error reporting etc., this is OK for v1 but needs work
func (l logOperationManager) OperationLoop() {
	glog.Infof("Log operation manager starting")

	// Outer loop, runs until terminated
	for {
		// We might want to bail out early when testing
		if l.runLimit >= 0 {
			l.runLimit--
			if l.runLimit < 0 {
				return
			}
		}

		// Wait for the configured time before going for another pass
		time.Sleep(l.sleepBetweenRuns)

		// TODO(Martin2112) using log ID zero because we don't have an id for metadata ops
		// this API could improved
		provider, err := l.storageProvider(0)

		// If we get an error, we can't do anything but wait until the next run through
		if err != nil {
			glog.Warningf("Failed to get storage provider for run: %v", err)
			continue
		}

		tx, err := provider.Begin()

		if err != nil {
			glog.Warningf("Failed to get tx for run: %v", err)
			continue
		}

		// Inner loop is across all active logs, currently one at a time
		logIDs, err := tx.GetActiveLogIDs()

		if err != nil {
			glog.Warningf("Failed to get log list for run: %v", err)
			tx.Rollback()
			continue
		}

		if err := tx.Commit(); err != nil {
			glog.Warningf("Failed to commit getting logs, continuing anyway: %v", err)
			continue
		}

		// Process each active log once, exit if we've seen a quit signal
		quit := l.runOperationPass(logIDs)
		if quit {
			glog.Infof("Log operation manager shutting down")
			return
		}

		glog.Infof("Log operation manager pass complete")
	}
}
