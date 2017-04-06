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
	"time"

	"github.com/google/trillian/crypto"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/log"
	"github.com/google/trillian/merkle"
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

// ExecutePass performs sequencing for the specified Log.
func (s SequencerManager) ExecutePass(ctx context.Context, logID int64, info *LogOperationInfo) (int, error) {
	// TODO(Martin2112): Honor the sequencing enabled in log parameters, needs an API change
	// so deferring it

	// TODO(Martin2112): Allow for different tree hashers to be used by different logs
	hasher, err := merkle.Factory(merkle.RFC6962SHA256Type)
	if err != nil {
		return 0, fmt.Errorf("failed to build hasher for log %d: %v", logID, err)
	}

	signer, err := newSigner(ctx, s.registry, logID)
	if err != nil {
		return 0, fmt.Errorf("failed to get signer for log %d: %v", logID, err)
	}

	sequencer := log.NewSequencer(hasher, info.timeSource, s.registry.LogStorage, signer)
	sequencer.SetGuardWindow(s.guardWindow)

	leaves, err := sequencer.SequenceBatch(ctx, logID, info.batchSize)
	if err != nil {
		return 0, fmt.Errorf("failed to sequence batch for %d:: %v", logID, err)
	}
	return leaves, nil
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
