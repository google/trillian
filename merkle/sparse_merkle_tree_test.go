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

package merkle

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/trillian/merkle/maphasher"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/tree"
	"github.com/google/trillian/testonly"
)

func getSparseMerkleTreeReaderWithMockTX(ctrl *gomock.Controller, rev int64) (*SparseMerkleTreeReader, *storage.MockMapTreeTX) {
	tx := storage.NewMockMapTreeTX(ctrl)
	return NewSparseMerkleTreeReader(rev, maphasher.Default, tx), tx
}

func TestInclusionProofForNullEntryInEmptyTree(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	const rev = 100
	r, tx := getSparseMerkleTreeReaderWithMockTX(mockCtrl, rev)
	tx.EXPECT().Commit(gomock.Any()).AnyTimes().Return(nil)
	tx.EXPECT().GetMerkleNodes(gomock.Any(), int64(rev), gomock.Any()).Return([]tree.Node{}, nil)
	const key = "SomeArbitraryKey"
	proof, err := r.InclusionProof(ctx, rev, testonly.HashKey(key))
	if err != nil {
		t.Fatalf("Got error while retrieving inclusion proof: %v", err)
	}

	if expected, got := 256, len(proof); expected != got {
		t.Fatalf("Expected proof of len %d, but got len %d", expected, got)
	}

	// Verify these are null hashes
	for i := len(proof) - 1; i > 0; i-- {
		if got := proof[i]; got != nil {
			t.Errorf("proof[%d] = %v, expected nil", i, got)
		}
	}
}

func TestBatchInclusionProofForNullEntriesInEmptyTrees(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	const rev = 100
	r, tx := getSparseMerkleTreeReaderWithMockTX(mockCtrl, rev)
	tx.EXPECT().Commit(gomock.Any()).AnyTimes().Return(nil)
	tx.EXPECT().GetMerkleNodes(gomock.Any(), int64(rev), gomock.Any()).Return([]tree.Node{}, nil)
	key := testonly.HashKey("SomeArbitraryKey")
	key2 := testonly.HashKey("SomeOtherArbitraryKey")
	proofs, err := r.BatchInclusionProof(ctx, rev, [][]byte{key, key2})
	if err != nil {
		t.Fatalf("Got error while retrieving inclusion proofs: %v", err)
	}

	if expected, got := 2, len(proofs); expected != got {
		t.Fatalf("Expected %d proofs but got %d", expected, got)
	}

	proof1 := proofs[string(key)]
	if expected, got := 256, len(proof1); expected != got {
		t.Fatalf("Expected proof1 of len %d, but got len %d", expected, got)
	}

	proof2 := proofs[string(key2)]
	if expected, got := 256, len(proof2); expected != got {
		t.Fatalf("Expected proof2 of len %d, but got len %d", expected, got)
	}

	// Verify these are null hashes
	for _, proof := range proofs {
		for i := len(proof) - 1; i > 0; i-- {
			if got := proof[i]; got != nil {
				t.Errorf("proof[%d] = %v, expected nil", i, got)
			}
		}
	}
}

// TODO(al): Add some more inclusion proof tests here

func TestInclusionProofPassesThroughStorageError(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	const rev = 100
	r, tx := getSparseMerkleTreeReaderWithMockTX(mockCtrl, rev)
	e := errors.New("boo")
	tx.EXPECT().GetMerkleNodes(gomock.Any(), int64(rev), gomock.Any()).Return([]tree.Node{}, e)
	_, err := r.InclusionProof(ctx, rev, testonly.HashKey("Whatever"))
	if err != e {
		t.Fatalf("InclusionProof() should've returned an error '%v', but got '%v'", e, err)
	}
}
