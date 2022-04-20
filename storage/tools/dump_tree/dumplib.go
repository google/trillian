// Copyright 2017 Google LLC. All Rights Reserved.
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

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/log"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/cache"
	"github.com/google/trillian/storage/memory"
	"github.com/google/trillian/storage/storagepb"
	"github.com/google/trillian/types"
	"github.com/google/trillian/util/clock"
	"github.com/transparency-dev/merkle"
	"github.com/transparency-dev/merkle/compact"
	"github.com/transparency-dev/merkle/rfc6962"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/types/known/durationpb"
)

type treeAndRev struct {
	fullKey  string
	subtree  *storagepb.SubtreeProto
	revision int
}

// fullProto is an output formatter function that produces a single line in proto text format.
func fullProto(s *storagepb.SubtreeProto) string {
	return fmt.Sprintf("%s\n", prototext.Format(s))
}

func sequence(tree *trillian.Tree, logStorage storage.LogStorage, count, batchSize int) {
	glog.Infof("Sequencing batch of size %d", count)
	sequenced, err := log.IntegrateBatch(context.TODO(), tree, batchSize, 0, 24*time.Hour, clock.System, logStorage, quota.Noop())
	if err != nil {
		glog.Fatalf("IntegrateBatch got: %v, want: no err", err)
	}

	if got, want := sequenced, count; got != want {
		glog.Fatalf("IntegrateBatch got: %d sequenced, want: %d", got, want)
	}
}

func createTree(as storage.AdminStorage, ls storage.LogStorage) *trillian.Tree {
	ctx := context.TODO()
	tree := &trillian.Tree{
		TreeType:        trillian.TreeType_LOG,
		TreeState:       trillian.TreeState_ACTIVE,
		MaxRootDuration: durationpb.New(0 * time.Millisecond),
	}
	createdTree, err := storage.CreateTree(ctx, as, tree)
	if err != nil {
		glog.Fatalf("Create tree: %v", err)
	}

	logRoot, err := (&types.LogRootV1{RootHash: rfc6962.DefaultHasher.EmptyRoot()}).MarshalBinary()
	if err != nil {
		glog.Fatalf("MarshalBinary: %v", err)
	}
	sthZero := &trillian.SignedLogRoot{LogRoot: logRoot}

	err = ls.ReadWriteTransaction(ctx, createdTree, func(ctx context.Context, tx storage.LogTreeTX) error {
		if err := tx.StoreSignedLogRoot(ctx, sthZero); err != nil {
			glog.Fatalf("StoreSignedLogRoot: %v", err)
		}
		return nil
	})
	if err != nil {
		glog.Fatalf("ReadWriteTransaction: %v", err)
	}

	return createdTree
}

// Options are the commandline arguments one can pass to Main
type Options struct {
	TreeSize, BatchSize           int
	LeafFormat                    string
	LatestRevision                bool
	Rebuild, Traverse, DumpLeaves bool
}

// Main runs the dump_tree tool
func Main(args Options) string {
	ctx := context.Background()

	glog.Info("Initializing memory log storage")
	ts := memory.NewTreeStorage()
	ls := memory.NewLogStorage(ts, monitoring.InertMetricFactory{})
	as := memory.NewAdminStorage(ts)
	tree := createTree(as, ls)

	log.InitMetrics(nil)

	// Create the initial tree head at size 0, which is required. And then sequence the leaves.
	sequence(tree, ls, 0, args.BatchSize)
	sequenceLeaves(ls, tree, args.TreeSize, args.BatchSize, args.LeafFormat)

	// Read the latest STH back
	var root types.LogRootV1
	err := ls.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.LogTreeTX) error {
		var err error
		sth, err := tx.LatestSignedLogRoot(ctx)
		if err != nil {
			glog.Fatalf("LatestSignedLogRoot: %v", err)
		}
		if err := root.UnmarshalBinary(sth.LogRoot); err != nil {
			return fmt.Errorf("could not parse current log root: %v", err)
		}

		glog.Infof("STH at size %d has hash %s",
			root.TreeSize,
			hex.EncodeToString(root.RootHash))
		return nil
	})
	if err != nil {
		glog.Fatalf("ReadWriteTransaction: %v", err)
	}

	// All leaves are now sequenced into the tree. The current state is what we need.
	glog.Info("Producing output")

	if args.Traverse {
		return traverseTreeStorage(ctx, ls, tree, args.TreeSize)
	}

	if args.DumpLeaves {
		return dumpLeaves(ctx, ls, tree, args.TreeSize)
	}

	formatter := fullProto

	if args.LatestRevision {
		return latestRevisions(ls, tree.TreeId, rfc6962.DefaultHasher, formatter, args.Rebuild)
	}
	return allRevisions(ls, tree.TreeId, rfc6962.DefaultHasher, formatter, args.Rebuild)
}

func allRevisions(ls storage.LogStorage, treeID int64, hasher merkle.LogHasher, of func(*storagepb.SubtreeProto) string, rebuildInternal bool) string {
	out := new(bytes.Buffer)
	memory.DumpSubtrees(ls, treeID, func(k string, v *storagepb.SubtreeProto) {
		if rebuildInternal {
			cache.PopulateLogTile(v, hasher)
		}
		fmt.Fprint(out, of(v))
	})
	return out.String()
}

func latestRevisions(ls storage.LogStorage, treeID int64, hasher merkle.LogHasher, of func(*storagepb.SubtreeProto) string, rebuildInternal bool) string {
	out := new(bytes.Buffer)
	// vMap maps subtree prefixes (as strings) to the corresponding subtree proto and its revision
	vMap := make(map[string]treeAndRev)
	memory.DumpSubtrees(ls, treeID, func(k string, v *storagepb.SubtreeProto) {
		// Relies on the btree key space for subtrees being /tree_id/subtree/<id>/<revision>
		pieces := strings.Split(k, "/")
		if got, want := len(pieces), 5; got != want {
			glog.Fatalf("Wrong no of Btree subtree key segments. Got: %d, want: %d", got, want)
		}

		subID := pieces[3]
		subtree := vMap[subID]
		rev, err := strconv.Atoi(pieces[4])
		if err != nil {
			glog.Fatalf("Bad subtree key: %v", k)
		}

		if rev > subtree.revision {
			vMap[subID] = treeAndRev{
				fullKey:  k,
				subtree:  v,
				revision: rev,
			}
		}
	})

	// Store the keys in sorted order
	var sKeys []string
	for k := range vMap {
		sKeys = append(sKeys, k)
	}
	sort.Strings(sKeys)

	// The map should now contain the latest revisions per subtree
	for _, k := range sKeys {
		v := vMap[k]
		if rebuildInternal {
			cache.PopulateLogTile(v.subtree, hasher)
		}
		fmt.Fprint(out, of(v.subtree))
	}
	return out.String()
}

func sequenceLeaves(ls storage.LogStorage, tree *trillian.Tree, treeSize, batchSize int, leafDataFormat string) {
	glog.Info("Queuing work")
	for l := 0; l < treeSize; l++ {
		glog.V(1).Infof("Queuing leaf %d", l)

		leafData := []byte(fmt.Sprintf(leafDataFormat, l))
		hash := sha256.Sum256(leafData)
		lh := hash[:]
		leaf := trillian.LogLeaf{LeafValue: leafData, LeafIdentityHash: lh, MerkleLeafHash: lh}
		leaves := []*trillian.LogLeaf{&leaf}

		if _, err := ls.QueueLeaves(context.TODO(), tree, leaves, time.Now()); err != nil {
			glog.Fatalf("QueueLeaves got: %v, want: no err", err)
		}

		if l > 0 && l%batchSize == 0 {
			sequence(tree, ls, batchSize, batchSize)
		}
	}
	glog.Info("Finished queueing")
	// Handle anything left over
	left := treeSize % batchSize
	if left == 0 {
		left = batchSize
	}
	sequence(tree, ls, left, batchSize)
	glog.Info("Finished sequencing")
}

func traverseTreeStorage(ctx context.Context, ls storage.LogStorage, tt *trillian.Tree, ts int) string {
	out := new(bytes.Buffer)
	nodesAtLevel := uint64(ts)

	tx, err := ls.SnapshotForTree(context.TODO(), tt)
	if err != nil {
		glog.Fatalf("SnapshotForTree: %v", err)
	}
	defer func() {
		if err := tx.Commit(ctx); err != nil {
			glog.Fatalf("TX Commit(): %v", err)
		}
	}()

	levels := uint(0)
	n := nodesAtLevel
	for n > 0 {
		levels++
		n = n >> 1
	}

	// Because of the way we store subtrees omitting internal RHS nodes with one sibling there
	// is an extra level stored for trees that don't have a number of leaves that is a power
	// of 2. We account for this here and in the loop below.
	if !isPerfectTree(int64(ts)) {
		levels++
	}

	for level := uint(0); level < levels; level++ {
		for node := uint64(0); node < nodesAtLevel; node++ {
			// We're going to request one node at a time, which would normally be slow but we have
			// the tree in RAM so it's not a real problem.
			nodeID := compact.NewNodeID(level, node)
			nodes, err := tx.GetMerkleNodes(context.TODO(), []compact.NodeID{nodeID})
			if err != nil {
				glog.Fatalf("GetMerkleNodes: %+v: %v", nodeID, err)
			}
			if len(nodes) != 1 {
				glog.Fatalf("GetMerkleNodes: %+v: want 1 node got: %v", nodeID, nodes)
			}

			fmt.Fprintf(out, "%6d %6d -> %s\n", level, node, hex.EncodeToString(nodes[0].Hash))
		}

		nodesAtLevel = nodesAtLevel >> 1
		fmt.Println()
		// This handles the extra level in non-perfect trees
		if nodesAtLevel == 0 {
			nodesAtLevel = 1
		}
	}
	return out.String()
}

func dumpLeaves(ctx context.Context, ls storage.LogStorage, tree *trillian.Tree, ts int) string {
	out := new(bytes.Buffer)
	tx, err := ls.SnapshotForTree(ctx, tree)
	if err != nil {
		glog.Fatalf("SnapshotForTree: %v", err)
	}
	defer func() {
		if err := tx.Commit(ctx); err != nil {
			glog.Fatalf("TX Commit(): got: %v", err)
		}
	}()

	for l := int64(0); l < int64(ts); l++ {
		leaves, err := tx.GetLeavesByRange(ctx, l, 1)
		if err != nil {
			glog.Fatalf("GetLeavesByRange for index %d got: %v", l, err)
		}
		fmt.Fprintf(out, "%6d:%s\n", l, leaves[0].LeafValue)
	}
	return out.String()
}

func isPerfectTree(x int64) bool {
	return x != 0 && (x&(x-1) == 0)
}
