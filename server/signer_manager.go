package server

import (
	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/log"
	"github.com/google/trillian/merkle"
)

type SignerManager struct {
	keyManager crypto.KeyManager
}

func NewSignerManager(km crypto.KeyManager) *SignerManager {
	return &SignerManager{keyManager: km}
}

func (s SignerManager) Name() string {
	return "Signer"
}

func (s SignerManager) ExecutePass(logIDs []trillian.LogID, context LogOperationManagerContext) bool {
	// TODO(Martin2112): Demote logging to verbose level
	glog.Infof("Beginning signing run for %d active log(s)", len(logIDs))

	successCount := 0

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

		sequencer := log.NewSequencer(merkle.NewRFC6962TreeHasher(trillian.NewSHA256()), context.timeSource, storage, s.keyManager)
		err = sequencer.SignRoot()

		if err != nil {
			glog.Warningf("Error trying to sign for: %v: %v", logID, err)
			continue
		}

		successCount++
	}

	glog.Infof("Sigining run completed %d succeeded %d failed", successCount, len(logIDs)-successCount)

	return false
}
