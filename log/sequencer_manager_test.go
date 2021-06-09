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
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/merkle/compact"
	rfc6962 "github.com/google/trillian/merkle/rfc6962/hasher"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/storage"
	stestonly "github.com/google/trillian/storage/testonly"
	"github.com/google/trillian/storage/tree"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/types"
	"github.com/google/trillian/util/clock"
	"google.golang.org/protobuf/proto"
)

// Arbitrary time for use in tests
var fakeTimeSource = clock.NewFake(fakeTime)

// We use a size zero tree for testing, Merkle tree state restore is tested elsewhere
var (
	leaf0Hash = rfc6962.DefaultHasher.HashLeaf([]byte{})
	testLeaf0 = &trillian.LogLeaf{
		MerkleLeafHash: leaf0Hash,
		LeafValue:      nil,
		ExtraData:      nil,
		LeafIndex:      0,
	}
)

var testLeaf0Updated = &trillian.LogLeaf{
	MerkleLeafHash:     testonly.MustDecodeBase64("bjQLnP+zepicpUTmu3gKLHiQHT+zNzh2hRGjBhevoB0="),
	LeafValue:          nil,
	ExtraData:          nil,
	LeafIndex:          0,
	IntegrateTimestamp: testonly.MustToTimestampProto(fakeTime),
}

var (
	testRoot0 = &types.LogRootV1{
		TreeSize: 0,
		RootHash: []byte{},
	}
	testRoot0Bytes, _ = testRoot0.MarshalBinary()
	testSignedRoot0   = &trillian.SignedLogRoot{LogRoot: testRoot0Bytes}

	updatedNodes0 = []tree.Node{{ID: compact.NewNodeID(0, 0), Hash: testonly.MustDecodeBase64("bjQLnP+zepicpUTmu3gKLHiQHT+zNzh2hRGjBhevoB0=")}}
	updatedRoot   = &types.LogRootV1{
		TimestampNanos: uint64(fakeTime.UnixNano()),
		RootHash:       []byte{110, 52, 11, 156, 255, 179, 122, 152, 156, 165, 68, 230, 187, 120, 10, 44, 120, 144, 29, 63, 179, 55, 56, 118, 133, 17, 163, 6, 23, 175, 160, 29},
		TreeSize:       1,
	}
	updatedRootBytes, _ = updatedRoot.MarshalBinary()
	updatedSignedRoot   = &trillian.SignedLogRoot{LogRoot: updatedRootBytes}
)

var zeroDuration = 0 * time.Second

func TestSequencerManagerSingleLogNoLeaves(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	logID := stestonly.LogTree.GetTreeId()
	mockAdminTx := storage.NewMockReadOnlyAdminTX(mockCtrl)
	mockAdmin := &stestonly.FakeAdminStorage{ReadOnlyTX: []storage.ReadOnlyAdminTX{mockAdminTx}}
	mockTx := storage.NewMockLogTreeTX(mockCtrl)
	fakeStorage := &stestonly.FakeLogStorage{TX: mockTx}

	mockTx.EXPECT().Commit(gomock.Any()).Return(nil)
	mockTx.EXPECT().Close().Return(nil)
	mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(testSignedRoot0, nil)
	mockTx.EXPECT().DequeueLeaves(gomock.Any(), 50, fakeTime).Return([]*trillian.LogLeaf{}, nil)

	mockAdminTx.EXPECT().GetTree(gomock.Any(), logID).Return(stestonly.LogTree, nil)
	mockAdminTx.EXPECT().Commit().Return(nil)
	mockAdminTx.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: mockAdmin,
		LogStorage:   fakeStorage,
		QuotaManager: quota.Noop(),
	}

	sm := NewSequencerManager(registry, zeroDuration)
	sm.ExecutePass(ctx, logID, createTestInfo(registry))
}

func TestSequencerManagerCachesSigners(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	logID := stestonly.LogTree.GetTreeId()
	mockAdminTx := storage.NewMockReadOnlyAdminTX(mockCtrl)
	mockAdmin := &stestonly.FakeAdminStorage{}
	mockTx := storage.NewMockLogTreeTX(mockCtrl)
	fakeStorage := &stestonly.FakeLogStorage{}

	registry := extension.Registry{
		AdminStorage: mockAdmin,
		LogStorage:   fakeStorage,
		QuotaManager: quota.Noop(),
	}
	sm := NewSequencerManager(registry, zeroDuration)

	// Expect two sequencing passes.
	for i := 0; i < 2; i++ {
		mockAdmin.ReadOnlyTX = []storage.ReadOnlyAdminTX{mockAdminTx}
		gomock.InOrder(
			mockAdminTx.EXPECT().GetTree(gomock.Any(), logID).Return(stestonly.LogTree, nil),
			mockAdminTx.EXPECT().Commit().Return(nil),
			mockAdminTx.EXPECT().Close().Return(nil),
		)

		fakeStorage.TX = mockTx
		gomock.InOrder(
			mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(testSignedRoot0, nil),
			mockTx.EXPECT().DequeueLeaves(gomock.Any(), 50, fakeTime).Return([]*trillian.LogLeaf{}, nil),
			mockTx.EXPECT().Commit(gomock.Any()).Return(nil),
			mockTx.EXPECT().Close().Return(nil),
		)

		if _, err := sm.ExecutePass(ctx, logID, createTestInfo(registry)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSequencerManagerSingleLogOneLeaf(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	logID := stestonly.LogTree.GetTreeId()
	mockAdminTx := storage.NewMockReadOnlyAdminTX(mockCtrl)
	mockAdmin := &stestonly.FakeAdminStorage{ReadOnlyTX: []storage.ReadOnlyAdminTX{mockAdminTx}}
	mockTx := storage.NewMockLogTreeTX(mockCtrl)
	fakeStorage := &stestonly.FakeLogStorage{TX: mockTx}

	// Set up enough mockery to be able to sequence. We don't test all the error paths
	// through sequencer as other tests cover this
	mockTx.EXPECT().Commit(gomock.Any()).Return(nil)
	mockTx.EXPECT().Close().Return(nil)
	mockTx.EXPECT().DequeueLeaves(gomock.Any(), 50, fakeTime).Return([]*trillian.LogLeaf{testLeaf0}, nil)
	mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(testSignedRoot0, nil)
	mockTx.EXPECT().UpdateSequencedLeaves(gomock.Any(), cmpMatcher{[]*trillian.LogLeaf{testLeaf0Updated}}).Return(nil)
	mockTx.EXPECT().SetMerkleNodes(gomock.Any(), updatedNodes0).Return(nil)
	mockTx.EXPECT().StoreSignedLogRoot(gomock.Any(), cmpMatcher{updatedSignedRoot}).Return(nil)

	mockAdminTx.EXPECT().GetTree(gomock.Any(), logID).Return(stestonly.LogTree, nil)
	mockAdminTx.EXPECT().Commit().Return(nil)
	mockAdminTx.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: mockAdmin,
		LogStorage:   fakeStorage,
		QuotaManager: quota.Noop(),
	}

	sm := NewSequencerManager(registry, zeroDuration)
	sm.ExecutePass(ctx, logID, createTestInfo(registry))
}

// cmpMatcher is a custom gomock.Matcher that uses cmp.Equal combined with a
// cmp.Comparer that knows how to properly compare proto.Message types.
type cmpMatcher struct{ want interface{} }

func (m cmpMatcher) Matches(got interface{}) bool {
	return cmp.Equal(got, m.want, cmp.Comparer(proto.Equal))
}

func (m cmpMatcher) String() string {
	return fmt.Sprintf("equals %v", m.want)
}

func TestSequencerManagerGuardWindow(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	logID := stestonly.LogTree.GetTreeId()
	mockAdminTx := storage.NewMockReadOnlyAdminTX(mockCtrl)
	mockAdmin := &stestonly.FakeAdminStorage{ReadOnlyTX: []storage.ReadOnlyAdminTX{mockAdminTx}}
	mockTx := storage.NewMockLogTreeTX(mockCtrl)
	fakeStorage := &stestonly.FakeLogStorage{TX: mockTx}

	mockTx.EXPECT().Commit(gomock.Any()).Return(nil)
	mockTx.EXPECT().Close().Return(nil)
	mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(testSignedRoot0, nil)
	// Expect a 5 second guard window to be passed from manager -> sequencer -> storage
	mockTx.EXPECT().DequeueLeaves(gomock.Any(), 50, fakeTime.Add(-time.Second*5)).Return([]*trillian.LogLeaf{}, nil)

	mockAdminTx.EXPECT().GetTree(gomock.Any(), logID).Return(stestonly.LogTree, nil)
	mockAdminTx.EXPECT().Commit().Return(nil)
	mockAdminTx.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: mockAdmin,
		LogStorage:   fakeStorage,
		QuotaManager: quota.Noop(),
	}

	sm := NewSequencerManager(registry, time.Second*5)
	sm.ExecutePass(ctx, logID, createTestInfo(registry))
}

func createTestInfo(registry extension.Registry) *OperationInfo {
	// Set sign interval to 100 years so it won't trigger a root expiry signing unless overridden
	return &OperationInfo{
		Registry:    registry,
		BatchSize:   50,
		RunInterval: time.Second,
		TimeSource:  fakeTimeSource,
	}
}
