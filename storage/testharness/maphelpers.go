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

package testharness

import (
	"context"
	"crypto"
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/types"

	tcrypto "github.com/google/trillian/crypto"
	storageto "github.com/google/trillian/storage/testonly"
)

func createInitializedMapForTests(ctx context.Context, t *testing.T, s storage.MapStorage, as storage.AdminStorage) *trillian.Tree {
	t.Helper()
	tree := createTree(ctx, t, as, storageto.MapTree)

	signer := tcrypto.NewSigner(tree.TreeId, testonly.NewSignerWithFixedSig(nil, []byte("sig")), crypto.SHA256)
	err := s.ReadWriteTransaction(ctx, tree, func(ctx context.Context, tx storage.MapTreeTX) error {
		initialRoot, _ := signer.SignMapRoot(&types.MapRootV1{
			RootHash: []byte("rootHash"),
			Revision: 0,
		})

		if err := tx.StoreSignedMapRoot(ctx, initialRoot); err != nil {
			t.Fatalf("Failed to StoreSignedMapRoot: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ReadWriteTransaction() = %v", err)
	}

	return tree
}

func mapTree(mapID int64) *trillian.Tree {
	return &trillian.Tree{
		TreeId:       mapID,
		TreeType:     trillian.TreeType_MAP,
		HashStrategy: trillian.HashStrategy_TEST_MAP_HASHER,
	}
}
