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
	"sync"
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
	guardWindow time.Duration
	registry    extension.Registry
}

// NewSequencerManager creates a new SequencerManager instance based on the provided KeyManager instance
// and guard window.
func NewSequencerManager(registry extension.Registry, gw time.Duration) *SequencerManager {
	return &SequencerManager{
		guardWindow: gw,
		registry:    registry,
	}
}

// Name returns the name of the object.
func (s SequencerManager) Name() string {
	return "Sequencer"
}

// ExecutePass performs sequencing for the specified set of Logs.
func (s SequencerManager) ExecutePass(ctx context.Context, logIDs []int64, info *LogOperationInfo) {
	if info.numSequencers == 0 {
		glog.Warning("Called ExecutePass with numSequencers == 0, assuming 1")
		info.numSequencers = 1
	}
	glog.V(1).Infof("Beginning sequencing run for %v active log(s) using %d sequencers", len(logIDs), info.numSequencers)

	startBatch := time.Now()

	var mu sync.Mutex
	successCount := 0
	leavesAdded := 0

	var wg sync.WaitGroup
	toSeq := make(chan int64, len(logIDs))

	for _, logID := range logIDs {
		toSeq <- logID
	}
	close(toSeq)

	for i := 0; i < info.numSequencers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				logID, more := <-toSeq
				if !more {
					return
				}

				start := time.Now()

				// TODO(Martin2112): Honor the sequencing enabled in log parameters, needs an API change
				// so deferring it
				ctx := util.NewLogContext(ctx, logID)

				// TODO(Martin2112): Allow for different tree hashers to be used by different logs
				hasher, err := merkle.Factory(merkle.RFC6962SHA256Type)
				if err != nil {
					glog.Errorf("Unknown hash strategy for log %d: %v", logID, err)
					continue
				}

				signer, err := newSigner(ctx, s.registry, logID)
				if err != nil {
					glog.Errorf("Could not get signer for log %d: %v", logID, err)
					continue
				}

				sequencer := log.NewSequencer(hasher, info.timeSource, s.registry.LogStorage, signer)
				sequencer.SetGuardWindow(s.guardWindow)

				leaves, err := sequencer.SequenceBatch(ctx, logID, info.batchSize)
				if err != nil {
					glog.Warningf("%v: Error trying to sequence batch for: %v", logID, err)
					continue
				}
				if leaves > 0 {
					d := time.Now().Sub(start).Seconds()
					glog.Infof("%v: sequenced %d leaves in %.2f seconds (%.2f qps)", logID, leaves, d, float64(leaves)/d)
				} else {
					glog.V(1).Infof("%v: no leaves to sequence", logID)
				}

				mu.Lock()
				successCount++
				leavesAdded += leaves
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	d := time.Now().Sub(startBatch).Seconds()

	mu.Lock()
	defer mu.Unlock()
	glog.V(1).Infof("Sequencing group run completed in %.2f seconds: %v succeeded, %v failed, %v leaves integrated", d, successCount, len(logIDs)-successCount, leavesAdded)
}

func newSigner(ctx context.Context, registry extension.Registry, logID int64) (*crypto.Signer, error) {
	if registry.AdminStorage == nil {
		return nil, fmt.Errorf("no AdminStorage provided by registry")
	}
	if registry.SignerFactory == nil {
		return nil, fmt.Errorf("no SignerFactory provided by registry")
	}

	snapshot, err := registry.AdminStorage.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	defer snapshot.Close()

	tree, err := snapshot.GetTree(ctx, logID)
	if err != nil {
		return nil, err
	}

	if err := snapshot.Commit(); err != nil {
		return nil, err
	}

	signer, err := registry.SignerFactory.NewSigner(ctx, tree)
	if err != nil {
		return nil, err
	}

	return crypto.NewSigner(signer), nil
}
