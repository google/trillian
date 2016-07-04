package server

import (
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/log"
	"github.com/google/trillian/util"
	"github.com/google/trillian/merkle"
)

// SequencerManager controls sequencing activities for logs. At the moment it's very simple
// with a single task sequencing active logs one at a time. This will be expanded later.

type SequencerManager struct {
	// done is a channel that provides an exit signal
	done chan struct{}
	// storageProvider is the log storage provider used to build sequencers
	storageProvider LogStorageProviderFunc
	// batchSize is the batch size to be passed to sequencers run by this manager
	batchSize int
	// sleepBetweenLogs is the time to pause after each batch
	sleepBetweenLogs time.Duration
	// sleepBetweenRuns is the time to pause after all active logs have processed a batch
	sleepBetweenRuns time.Duration
	// runLimit is a limit on the number of sequencing passes. It can only be set for tests
	runLimit int
	// timeSource allows us to mock this in tests
	timeSource util.TimeSource
}

func NewSequencerManager(done chan struct{}, storageProvider LogStorageProviderFunc, batchSize int, sleepBetweenLogs, sleepBetweenRuns time.Duration) *SequencerManager {
	return &SequencerManager{done: done, storageProvider: storageProvider, batchSize: batchSize, sleepBetweenLogs: sleepBetweenLogs, sleepBetweenRuns: sleepBetweenRuns, timeSource: new(util.SystemTimeSource)}
}

// For use by tests, arranges for the sequencer to exit after a number of passes
func newSequencerManagerForTest(done chan struct{}, storageProvider LogStorageProviderFunc, batchSize int, sleepBetweenLogs, sleepBetweenRuns time.Duration, runLimit int, timeSource util.TimeSource) *SequencerManager {
	return &SequencerManager{done: done, storageProvider: storageProvider, batchSize: batchSize, sleepBetweenLogs: sleepBetweenLogs, sleepBetweenRuns: sleepBetweenRuns, runLimit: runLimit, timeSource: timeSource}
}

func (s SequencerManager) sequenceActiveLogs(logIDs []trillian.LogID) bool {
	// TODO(Martin2112): Demote logging to verbose level
	glog.Infof("Beginning sequencing run for %d active log(s)", len(logIDs))

	successCount := 0
	leavesAdded := 0

	for _, logID := range logIDs {
		// See if it's time to quit
		select {
		case <-s.done:
			return true
		default:
		}

		// Now wait for the configured time before going on to the next one
		time.Sleep(s.sleepBetweenLogs)

		// TODO(Martin2112): Probably want to make the sequencer objects longer lived to
		// avoid the cost of initializing their state each time but this works for now
		storage, err := s.storageProvider(logID.TreeID)

		// TODO(Martin2112): Honour the sequencing enabled in log parameters, needs an API change
		// so deferring it
		if err != nil {
			glog.Warningf("Storage provider failed for id %v because: %v", logID, err)
			continue
		}

		// TODO(Martin2112): Allow for different tree hashers to be used by different logs
		sequencer := log.NewSequencer(merkle.NewRFC6962TreeHasher(trillian.NewSHA256()), s.timeSource, storage)

		leaves, err := sequencer.SequenceBatch(s.batchSize)

		if err != nil {
			glog.Warningf("Error trying to sequence batch for %v: %v", logID, err)
			continue
		}

		successCount++
		leavesAdded += leaves
	}

	glog.Infof("Sequencing run completed %d succeeded %d failed %d leaves integrated", successCount, len(logIDs)-successCount, leavesAdded)

	return false
}

// SequencerLoop starts the manager working. It continues until told to exit.
// TODO(Martin2112): No mechanism for error reporting etc., this is OK for v1 but needs work
func (s SequencerManager) SequencerLoop() {
	glog.Infof("Log sequencer starting")

	// Outer sequencing loop, runs until terminated
	for {
		// We might want to bail out early when testing
		if s.runLimit >= 0 {
			s.runLimit--
			if s.runLimit < 0 {
				return
			}
		}

		// Wait for the configured time before going for another set of sequencing runs
		time.Sleep(s.sleepBetweenRuns)

		// TODO(Martin2112) using log ID zero because we don't have an id for metadata ops
		// this API could improved
		provider, err := s.storageProvider(0)

		// If we get an error, we can't do anything but wait until the next run through
		if err != nil {
			glog.Warningf("Failed to get storage provider for sequencing run: %v", err)
			continue
		}

		tx, err := provider.Begin()

		if err != nil {
			glog.Warningf("Failed to get tx for sequencing run: %v", err)
			continue
		}

		// Inner sequencing loop is across all active logs, currently one at a time
		logIDs, err := tx.GetActiveLogIDs()

		if err != nil {
			glog.Warningf("Failed to get log list for sequencing run: %v", err)
			tx.Rollback()
			continue
		}

		if err := tx.Commit(); err != nil {
			glog.Warningf("Failed to commit getting logs to sequence, continuing anyway: %v", err)
			continue
		}

		// Sequence each active log once, exit if we've seen a quit signal
		quit := s.sequenceActiveLogs(logIDs)
		if quit {
			glog.Infof("Log sequencer shutting down")
			return
		}

		glog.Infof("Log sequencing pass complete")
	}
}
