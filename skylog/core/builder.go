// Copyright 2019 Google Inc. All Rights Reserved.
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

// Package core contains code for a scalable Merkle tree construction.
package core

import (
	"context"
	"fmt"

	"github.com/google/trillian/merkle/compact"
	"github.com/google/trillian/skylog/storage"
)

// BuildJob is a slice of leaf data hashes spanning a range of a Merkle tree.
type BuildJob struct {
	RangeStart uint64   // The beginning of the range.
	Hashes     [][]byte // The leaf hashes.
}

// BuildWorker processes tree building jobs.
type BuildWorker struct {
	tw storage.TreeWriter
	rf *compact.RangeFactory
}

// NewBuildWorker returns a new BuildWorker for the specified tree.
func NewBuildWorker(tw storage.TreeWriter, rf *compact.RangeFactory) *BuildWorker {
	return &BuildWorker{tw: tw, rf: rf}
}

// Process handles a single build job. It populates the tree storage with the
// leaf hashes from the slice in the job, and all internal nodes of the tree
// that can be inferred from them. Returns the compact range of the resulting
// tree segment.
func (b *BuildWorker) Process(ctx context.Context, job BuildJob) (*compact.Range, error) {
	rng := b.rf.NewEmptyRange(job.RangeStart)
	if len(job.Hashes) == 0 {
		return rng, nil
	}
	nodes := make([]storage.Node, 0, len(job.Hashes)*2-1)
	visit := func(id compact.NodeID, hash []byte) {
		nodes = append(nodes, storage.Node{ID: id, Hash: hash})
	}
	for i, hash := range job.Hashes {
		// Add the leaf node.
		visit(compact.NewNodeID(0, job.RangeStart+uint64(i)), hash)
		// Add the internal tree nodes.
		if err := rng.Append(hash, visit); err != nil {
			return nil, fmt.Errorf("appending hash: %v", err)
		}
	}
	if err := b.tw.Write(ctx, nodes); err != nil {
		return nil, fmt.Errorf("writing tree nodes: %v", err)
	}
	return rng, nil
}
