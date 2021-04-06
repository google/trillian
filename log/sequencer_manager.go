// Copyright 2016 Google LLC. All Rights Reserved.
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

package log

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/extension"
	rfc6962 "github.com/google/trillian/merkle/rfc6962/hasher"
	"github.com/google/trillian/trees"

	tcrypto "github.com/google/trillian/crypto"
)

// SequencerManager provides sequencing operations for a collection of Logs.
type SequencerManager struct {
	guardWindow  time.Duration
	registry     extension.Registry
	signers      map[int64]*tcrypto.Signer
	signersMutex sync.Mutex
}

var seqOpts = trees.NewGetOpts(trees.SequenceLog, trillian.TreeType_LOG, trillian.TreeType_PREORDERED_LOG)

// NewSequencerManager creates a new SequencerManager instance based on the provided KeyManager instance
// and guard window.
func NewSequencerManager(registry extension.Registry, gw time.Duration) *SequencerManager {
	return &SequencerManager{
		guardWindow: gw,
		registry:    registry,
		signers:     make(map[int64]*tcrypto.Signer),
	}
}

// ExecutePass performs sequencing for the specified Log.
func (s *SequencerManager) ExecutePass(ctx context.Context, logID int64, info *OperationInfo) (int, error) {
	// TODO(Martin2112): Honor the sequencing enabled in log parameters, needs an API change
	// so deferring it

	tree, err := trees.GetTree(ctx, s.registry.AdminStorage, logID, seqOpts)
	if err != nil {
		return 0, fmt.Errorf("error retrieving log %v: %v", logID, err)
	}
	ctx = trees.NewContext(ctx, tree)

	if s := tree.HashStrategy; s != trillian.HashStrategy_RFC6962_SHA256 {
		return 0, fmt.Errorf("unknown hash strategy for log %v: %s", logID, s)
	}

	signer, err := s.getSigner(ctx, tree)
	if err != nil {
		return 0, fmt.Errorf("error getting signer for log %v: %v", logID, err)
	}

	sequencer := NewSequencer(rfc6962.DefaultHasher, info.TimeSource, s.registry.LogStorage, signer, s.registry.MetricFactory, s.registry.QuotaManager)

	maxRootDuration := tree.MaxRootDuration.AsDuration()
	leaves, err := sequencer.IntegrateBatch(ctx, tree, info.BatchSize, s.guardWindow, maxRootDuration)
	if err != nil {
		return 0, fmt.Errorf("failed to integrate batch for %v: %v", logID, err)
	}
	return leaves, nil
}

// getSigner returns a signer for the given tree.
// Signers are cached, so only one will be created per tree.
func (s *SequencerManager) getSigner(ctx context.Context, tree *trillian.Tree) (*tcrypto.Signer, error) {
	s.signersMutex.Lock()
	defer s.signersMutex.Unlock()

	if signer, ok := s.signers[tree.GetTreeId()]; ok {
		return signer, nil
	}

	signer, err := trees.Signer(ctx, tree)
	if err != nil {
		return nil, err
	}

	s.signers[tree.GetTreeId()] = signer
	return signer, nil
}
