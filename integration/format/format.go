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

// Package format contains an integration test which builds a log using an
// in-memory storage end-to-end, and makes sure the SubtreeProto storage format
// has no regressions.
package format

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

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

func run(treeSize, batchSize int, leafFormat string) (string, error) {
	ctx := context.Background()

	ts := memory.NewTreeStorage()
	ls := memory.NewLogStorage(ts, monitoring.InertMetricFactory{})
	as := memory.NewAdminStorage(ts)
	tree, err := createTree(ctx, as, ls)
	if err != nil {
		return "", err
	}
	log.InitMetrics(nil)

	leaves := generateLeaves(treeSize, leafFormat)
	if err := sequenceLeaves(ctx, ls, tree, leaves, batchSize); err != nil {
		return "", err
	}

	// Read the latest LogRoot back.
	var root types.LogRootV1
	if err := ls.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.LogTreeTX) error {
		latest, err := tx.LatestSignedLogRoot(ctx)
		if err != nil {
			return err
		}
		return root.UnmarshalBinary(latest.LogRoot)
	}); err != nil {
		return "", fmt.Errorf("ReadWriteTransaction: %v", err)
	}

	return latestRevisions(ls, tree.TreeId, rfc6962.DefaultHasher)
}

func createTree(ctx context.Context, as storage.AdminStorage, ls storage.LogStorage) (*trillian.Tree, error) {
	tree, err := storage.CreateTree(ctx, as, &trillian.Tree{
		TreeType:        trillian.TreeType_LOG,
		TreeState:       trillian.TreeState_ACTIVE,
		MaxRootDuration: durationpb.New(0 * time.Millisecond),
	})
	if err != nil {
		return nil, fmt.Errorf("CreateTree: %v", err)
	}

	logRoot, err := (&types.LogRootV1{RootHash: rfc6962.DefaultHasher.EmptyRoot()}).MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("MarshalBinary: %v", err)
	}

	if err = ls.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.LogTreeTX) error {
		return tx.StoreSignedLogRoot(ctx, &trillian.SignedLogRoot{LogRoot: logRoot})
	}); err != nil {
		return nil, fmt.Errorf("ReadWriteTransaction: %v", err)
	}

	return tree, nil
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

func sequenceLeaves(ctx context.Context, ls storage.LogStorage, tree *trillian.Tree, leaves []*trillian.LogLeaf, batchSize int) error {
	for i, size := 0, len(leaves); i < size; i += batchSize {
		if left := size - i; left < batchSize {
			batchSize = left
		}
		if _, err := ls.QueueLeaves(ctx, tree, leaves[i:i+batchSize], time.Now()); err != nil {
			return fmt.Errorf("QueueLeaves: %v", err)
		}

		sequenced, err := log.IntegrateBatch(ctx, tree, batchSize, 0, 24*time.Hour, clock.System, ls, quota.Noop())
		if err != nil {
			return fmt.Errorf("IntegrateBatch: %v", err)
		}
		if got, want := sequenced, batchSize; got != want {
			return fmt.Errorf("IntegrateBatch: got %d, want %d", got, want)
		}
	}
	return nil
}

type treeAndRev struct {
	subtree  *storagepb.SubtreeProto
	revision int
}

func latestRevisions(ls storage.LogStorage, treeID int64, hasher merkle.LogHasher) (string, error) {
	// vMap maps subtree prefixes (as strings) to the corresponding subtree proto and its revision
	vMap := make(map[string]treeAndRev)
	memory.DumpSubtrees(ls, treeID, func(k string, v *storagepb.SubtreeProto) {
		// Relies on the btree key space for subtrees being /tree_id/subtree/id/revision.
		pieces := strings.Split(k, "/")
		if got, want := len(pieces), 5; got != want {
			panic(fmt.Sprintf("Btree subtree key segments: got %d, want %d", got, want))
		}

		subID := pieces[3]
		rev, err := strconv.Atoi(pieces[4])
		if err != nil {
			panic(fmt.Sprintf("Bad subtree key: %v: %v", k, err))
		}

		if rev > vMap[subID].revision {
			vMap[subID] = treeAndRev{
				subtree:  v,
				revision: rev,
			}
		}
	})

	// Store the keys in sorted order.
	keys := make([]string, 0, len(vMap))
	for k := range vMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// The map should now contain the latest revisions per subtree.
	out := new(bytes.Buffer)
	for _, k := range keys {
		subtree := vMap[k].subtree
		cache.PopulateLogTile(subtree, hasher)
		fmt.Fprintf(out, "%s\n", prototext.Format(subtree))
	}
	return out.String(), nil
}
