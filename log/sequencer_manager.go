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
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/extension"
	rfc6962 "github.com/google/trillian/merkle/rfc6962/hasher"
	"github.com/google/trillian/trees"
)

// SequencerManager provides sequencing operations for a collection of Logs.
type SequencerManager struct {
	guardWindow time.Duration
	registry    extension.Registry
}

var seqOpts = trees.NewGetOpts(trees.SequenceLog, trillian.TreeType_LOG, trillian.TreeType_PREORDERED_LOG)

// NewSequencerManager creates a new SequencerManager instance based on the provided KeyManager instance
// and guard window.
func NewSequencerManager(registry extension.Registry, gw time.Duration) *SequencerManager {
	return &SequencerManager{
		guardWindow: gw,
		registry:    registry,
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

	sequencer := NewSequencer(rfc6962.DefaultHasher, info.TimeSource, s.registry.LogStorage, s.registry.MetricFactory, s.registry.QuotaManager)

	maxRootDuration := tree.MaxRootDuration.AsDuration()
	if !tree.MaxRootDuration.IsValid() {
		glog.Warning("failed to parse tree.MaxRootDuration, using zero")
		maxRootDuration = 0
	}
	leaves, err := sequencer.IntegrateBatch(ctx, tree, info.BatchSize, s.guardWindow, maxRootDuration)
	if err != nil {
		return 0, fmt.Errorf("failed to integrate batch for %v: %v", logID, err)
	}
	return leaves, nil
}
