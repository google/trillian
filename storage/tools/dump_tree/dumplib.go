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

// Options are the commandline arguments one can pass to Main
type Options struct {
	TreeSize   int
	BatchSize  int
	LeafFormat string
}

// Main runs the dump_tree tool
func Main(args Options) string {
	ctx := context.Background()

	ts := memory.NewTreeStorage()
	ls := memory.NewLogStorage(ts, monitoring.InertMetricFactory{})
	as := memory.NewAdminStorage(ts)
	tree := createTree(ctx, as, ls)

	log.InitMetrics(nil)

	leaves := generateLeaves(args.TreeSize, args.LeafFormat)
	sequenceLeaves(ls, tree, leaves, args.BatchSize)

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

	formatter := fullProto

	return latestRevisions(ls, tree.TreeId, rfc6962.DefaultHasher, formatter)
}

func createTree(ctx context.Context, as storage.AdminStorage, ls storage.LogStorage) *trillian.Tree {
	tree, err := storage.CreateTree(ctx, as, &trillian.Tree{
		TreeType:        trillian.TreeType_LOG,
		TreeState:       trillian.TreeState_ACTIVE,
		MaxRootDuration: durationpb.New(0 * time.Millisecond),
	})
	if err != nil {
		glog.Fatalf("CreateTree: %v", err)
	}

	logRoot, err := (&types.LogRootV1{RootHash: rfc6962.DefaultHasher.EmptyRoot()}).MarshalBinary()
	if err != nil {
		glog.Fatalf("MarshalBinary: %v", err)
	}

	if err = ls.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.LogTreeTX) error {
		return tx.StoreSignedLogRoot(ctx, &trillian.SignedLogRoot{LogRoot: logRoot})
	}); err != nil {
		glog.Fatalf("ReadWriteTransaction: %v", err)
	}

	return tree
}

func latestRevisions(ls storage.LogStorage, treeID int64, hasher merkle.LogHasher, of func(*storagepb.SubtreeProto) string) string {
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
		cache.PopulateLogTile(v.subtree, hasher)
		fmt.Fprint(out, of(v.subtree))
	}
	return out.String()
}

func sequenceLeaves(ls storage.LogStorage, tree *trillian.Tree, leaves []*trillian.LogLeaf, batchSize int) {
	for i, size := 0, len(leaves); i < size; i += batchSize {
		if left := size - i; left < batchSize {
			batchSize = left
		}
		if _, err := ls.QueueLeaves(context.TODO(), tree, leaves[i:i+batchSize], time.Now()); err != nil {
			glog.Fatalf("QueueLeaves: %v", err)
		}

		sequenced, err := log.IntegrateBatch(context.TODO(), tree, batchSize, 0, 24*time.Hour, clock.System, ls, quota.Noop())
		if err != nil {
			glog.Fatalf("IntegrateBatch: %v", err)
		}
		if got, want := sequenced, batchSize; got != want {
			glog.Fatalf("IntegrateBatch: got %d, want %d", got, want)
		}
	}
}

func generateLeaves(count int, format string) []*trillian.LogLeaf {
	leaves := make([]*trillian.LogLeaf, 0, count)
	for i := 0; i < count; i++ {
		data := []byte(fmt.Sprintf(format, i))
		hash := sha256.Sum256(data)
		leaves = append(leaves, &trillian.LogLeaf{
			LeafValue: data, LeafIdentityHash: hash[:], MerkleLeafHash: hash[:],
		})
	}
	return leaves
}
