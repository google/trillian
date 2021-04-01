// Copyright 2020 Google LLC. All Rights Reserved.
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

package storagetest

import (
	"context"
	"crypto"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/types"

	tcrypto "github.com/google/trillian/crypto"
)

// runLogTX is a helps avoid copying out "if err != nil { blah }" all over the place
func runLogTX(s storage.LogStorage, tree *trillian.Tree, t *testing.T, f storage.LogTXFunc) {
	t.Helper()
	if err := s.ReadWriteTransaction(context.Background(), tree, f); err != nil {
		t.Fatalf("Failed to run log tx: %v", err)
	}
}

// createTestLeaves creates some test leaves with predictable data
func createTestLeaves(n, startSeq int64) []*trillian.LogLeaf {
	var leaves []*trillian.LogLeaf
	for l := int64(0); l < n; l++ {
		lv := fmt.Sprintf("Leaf %d", l+startSeq)
		leafHash := sha256.Sum256([]byte(lv))
		leaf := &trillian.LogLeaf{
			LeafIdentityHash: leafHash[:],
			MerkleLeafHash:   leafHash[:],
			LeafValue:        []byte(lv),
			ExtraData:        []byte(fmt.Sprintf("Extra %d", l)),
			LeafIndex:        int64(startSeq + l),
		}
		leaves = append(leaves, leaf)
	}

	return leaves
}

func mustSignAndStoreLogRoot(ctx context.Context, t *testing.T, l storage.LogStorage, tree *trillian.Tree, r *types.LogRootV1) {
	t.Helper()
	signer := tcrypto.NewSigner(testonly.NewSignerWithFixedSig(nil, []byte("notnil")), crypto.SHA256)
	root, err := signer.SignLogRoot(r)
	if err != nil {
		t.Fatalf("error creating new SignedLogRoot: %v", err)
	}

	if err := l.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.LogTreeTX) error {
		return tx.StoreSignedLogRoot(ctx, root)
	}); err != nil {
		t.Fatalf("ReadWriteTransaction() = %v", err)
	}
}

func dequeueLeavesInTx(ctx context.Context, ls storage.LogStorage, tree *trillian.Tree, t time.Time, limit int) ([]*trillian.LogLeaf, error) {
	var ret []*trillian.LogLeaf
	if err := ls.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.LogTreeTX) error {
		var err error
		ret, err = tx.DequeueLeaves(ctx, limit, t)
		return err
	}); err != nil {
		return nil, err
	}
	return ret, nil
}
