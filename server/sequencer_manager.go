package server

import (
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/log"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/util"
)

type SequencerManager struct {
	logOperationManager
	keyManager crypto.KeyManager
}

func NewSequencerManager(km crypto.KeyManager, done chan struct{}, storageProvider LogStorageProviderFunc, batchSize int, sleepBetweenRuns time.Duration) *SequencerManager {
	return &SequencerManager{keyManager: km, logOperationManager: logOperationManager{done: done, storageProvider: storageProvider, batchSize: batchSize, sleepBetweenRuns: sleepBetweenRuns, timeSource: new(util.SystemTimeSource)}}
}

// For use by tests, arranges for the sequencer to exit after a number of passes
func newSequencerManagerForTest(km crypto.KeyManager, done chan struct{}, storageProvider LogStorageProviderFunc, batchSize int, sleepBetweenRuns time.Duration, runLimit int, timeSource util.TimeSource) *SequencerManager {
	return &SequencerManager{keyManager: km, logOperationManager: logOperationManager{done: done, storageProvider: storageProvider, batchSize: batchSize, sleepBetweenRuns: sleepBetweenRuns, timeSource: new(util.SystemTimeSource), runLimit: runLimit}}
}

func (s SequencerManager) runOperationPass(logIDs []trillian.LogID) bool {
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
			glog.Warningf("Storage provider failed for id: %v because: %v", logID, err)
			continue
		}

		// TODO(Martin2112): Allow for different tree hashers to be used by different logs
		sequencer := log.NewSequencer(merkle.NewRFC6962TreeHasher(trillian.NewSHA256()), s.timeSource, storage, s.keyManager)

		leaves, err := sequencer.SequenceBatch(s.batchSize)

		if err != nil {
			glog.Warningf("Error trying to sequence batch for: %v: %v", logID, err)
			continue
		}

		successCount++
		leavesAdded += leaves
	}

	glog.Infof("Sequencing run completed %d succeeded %d failed %d leaves integrated", successCount, len(logIDs)-successCount, leavesAdded)

	return false
}
