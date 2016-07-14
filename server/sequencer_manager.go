package server

import (
	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/log"
	"github.com/google/trillian/merkle"
)

type SequencerManager struct {
	keyManager crypto.KeyManager
}

func NewSequencerManager(km crypto.KeyManager) *SequencerManager {
	return &SequencerManager{keyManager: km}
}

func (s SequencerManager) Name() string {
	return "Sequencer"
}

func (s SequencerManager) ExecutePass(logIDs []trillian.LogID, context LogOperationManagerContext) bool {
	// TODO(Martin2112): Demote logging to verbose level
	glog.Infof("Beginning sequencing run for %d active log(s)", len(logIDs))

	successCount := 0
	leavesAdded := 0

	for _, logID := range logIDs {
		// See if it's time to quit
		select {
		case <-context.done:
			return true
		default:
		}

		// TODO(Martin2112): Probably want to make the sequencer objects longer lived to
		// avoid the cost of initializing their state each time but this works for now
		storage, err := context.storageProvider(logID.TreeID)

		// TODO(Martin2112): Honour the sequencing enabled in log parameters, needs an API change
		// so deferring it
		if err != nil {
			glog.Warningf("Storage provider failed for id: %v because: %v", logID, err)
			continue
		}

		// TODO(Martin2112): Allow for different tree hashers to be used by different logs
		sequencer := log.NewSequencer(merkle.NewRFC6962TreeHasher(trillian.NewSHA256()), context.timeSource, storage, s.keyManager)

		leaves, err := sequencer.SequenceBatch(context.batchSize)

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
