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

// SequencerManager provides sequencing operations for a collection of Logs.
type SequencerManager struct {
	keyManager     crypto.KeyManager
	guardWindow    time.Duration
	cachedProvider *cachedLogStorageProvider
}

func isRootTooOld(ts util.TimeSource, maxAge time.Duration) log.CurrentRootExpiredFunc {
	return func(root trillian.SignedLogRoot) bool {
		rootTime := time.Unix(0, root.TimestampNanos)
		rootAge := ts.Now().Sub(rootTime)

		return rootAge > maxAge
	}
}

// NewSequencerManager creates a new SequencerManager instance based on the provided KeyManager instance
// and guard window.
func NewSequencerManager(km crypto.KeyManager, p LogStorageProviderFunc, gw time.Duration) *SequencerManager {
	return &SequencerManager{
		keyManager:     km,
		guardWindow:    gw,
		cachedProvider: newCachedLogStorageProvider(p),
	}
}

// Name returns the name of the object.
func (s SequencerManager) Name() string {
	return "Sequencer"
}

// ExecutePass performs sequencing for the specified set of Logs.
func (s SequencerManager) ExecutePass(logIDs []int64, logctx LogOperationManagerContext) bool {
	// TODO(Martin2112): Demote logging to verbose level
	glog.Infof("Beginning sequencing run for %d active log(s)", len(logIDs))

	successCount := 0
	leavesAdded := 0

	for _, logID := range logIDs {
		// See if it's time to quit
		select {
		case <-logctx.ctx.Done():
			return true
		default:
		}

		storage, err := s.cachedProvider.storageForLog(logID)
		ctx := util.NewLogContext(logctx.ctx, logID)

		// TODO(Martin2112): Honour the sequencing enabled in log parameters, needs an API change
		// so deferring it
		if err != nil {
			glog.Warningf("%s: Storage provider failed for id because: %v", util.LogIDPrefix(ctx), err)
			continue
		}

		// TODO(Martin2112): Allow for different tree hashers to be used by different logs
		sequencer := log.NewSequencer(merkle.NewRFC6962TreeHasher(crypto.NewSHA256()), logctx.timeSource, storage, s.keyManager)
		sequencer.SetGuardWindow(s.guardWindow)

		leaves, err := sequencer.SequenceBatch(ctx, logctx.batchSize, isRootTooOld(logctx.timeSource, logctx.signInterval))

		if err != nil {
			glog.Warningf("%s: Error trying to sequence batch for: %v", util.LogIDPrefix(ctx), err)
			continue
		}

		successCount++
		leavesAdded += leaves
	}

	glog.Infof("Sequencing run completed %d succeeded %d failed %d leaves integrated", successCount, len(logIDs)-successCount, leavesAdded)

	return false
}
