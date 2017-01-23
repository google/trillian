// Copyright 2017 Google Inc. All Rights Reserved.
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
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/log"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/util"
)

// SequencerManager provides sequencing operations for a collection of Logs.
type SequencerManager struct {
	keyManager  crypto.KeyManager
	guardWindow time.Duration
	registry    extension.Registry
}

// NewSequencerManager creates a new SequencerManager instance based on the provided KeyManager instance
// and guard window.
func NewSequencerManager(km crypto.KeyManager, registry extension.Registry, gw time.Duration) *SequencerManager {
	return &SequencerManager{
		keyManager:  km,
		guardWindow: gw,
		registry:    registry,
	}
}

// Name returns the name of the object.
func (s SequencerManager) Name() string {
	return "Sequencer"
}

// ExecutePass performs sequencing for the specified set of Logs.
func (s SequencerManager) ExecutePass(logIDs []int64, logctx LogOperationManagerContext) bool {
	glog.V(1).Infof("Beginning sequencing run for %d active log(s)", len(logIDs))

	successCount := 0
	leavesAdded := 0

	for _, logID := range logIDs {
		// See if it's time to quit
		select {
		case <-logctx.ctx.Done():
			return true
		default:
		}

		storage, err := s.registry.GetLogStorage(logID)
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

		leaves, err := sequencer.SequenceBatch(ctx, logctx.batchSize)

		if err != nil {
			glog.Warningf("%s: Error trying to sequence batch for: %v", util.LogIDPrefix(ctx), err)
			continue
		}

		successCount++
		leavesAdded += leaves
	}

	glog.V(1).Infof("Sequencing run completed %d succeeded %d failed %d leaves integrated", successCount, len(logIDs)-successCount, leavesAdded)

	return false
}
