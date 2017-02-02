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

package server

import (
	"context"
	gocrypto "crypto"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/util"
)

var (
	// Arbitrary time for use in tests
	fakeTime       = time.Date(2016, 6, 28, 13, 40, 12, 45, time.UTC)
	fakeTimeSource = util.FakeTimeSource{FakeTime: fakeTime}

	// We use a size zero tree for testing, Merkle tree state restore is tested elsewhere
	testLogID1       = int64(1)
	testLeaf0        = trillian.LogLeaf{MerkleLeafHash: testonly.Hasher.HashLeaf([]byte{}), LeafValue: nil, ExtraData: nil, LeafIndex: 0}
	testLeaf0Updated = trillian.LogLeaf{MerkleLeafHash: testonly.MustDecodeBase64("bjQLnP+zepicpUTmu3gKLHiQHT+zNzh2hRGjBhevoB0="), LeafValue: nil, ExtraData: nil, LeafIndex: 0}
	testRoot0        = trillian.SignedLogRoot{
		TreeSize:     0,
		TreeRevision: 0,
		LogId:        testLogID1,
		RootHash:     []byte{},
		Signature: &trillian.DigitallySigned{
			HashAlgorithm:      trillian.HashAlgorithm_SHA256,
			SignatureAlgorithm: trillian.SignatureAlgorithm_ECDSA,
		},
	}
	updatedNodes0 = []storage.Node{{NodeID: storage.NodeID{Path: []uint8{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, PrefixLenBits: 64, PathLenBits: 64}, Hash: testonly.MustDecodeBase64("bjQLnP+zepicpUTmu3gKLHiQHT+zNzh2hRGjBhevoB0="), NodeRevision: 1}}
	updatedRoot   = trillian.SignedLogRoot{
		LogId:          testLogID1,
		TimestampNanos: fakeTime.UnixNano(),
		RootHash:       []byte{110, 52, 11, 156, 255, 179, 122, 152, 156, 165, 68, 230, 187, 120, 10, 44, 120, 144, 29, 63, 179, 55, 56, 118, 133, 17, 163, 6, 23, 175, 160, 29},
		TreeSize:       1,
		Signature: &trillian.DigitallySigned{
			HashAlgorithm:      trillian.HashAlgorithm_SHA256,
			SignatureAlgorithm: trillian.SignatureAlgorithm_ECDSA,
			Signature:          []byte("signed"),
		},
		TreeRevision: 1,
	}

	// This is used in the signing test with no work where the treesize will be zero
	updatedRootSignOnly = trillian.SignedLogRoot{
		LogId:          testLogID1,
		TimestampNanos: fakeTime.UnixNano(),
		RootHash:       []uint8{0xe3, 0xb0, 0xc4, 0x42, 0x98, 0xfc, 0x1c, 0x14, 0x9a, 0xfb, 0xf4, 0xc8, 0x99, 0x6f, 0xb9, 0x24, 0x27, 0xae, 0x41, 0xe4, 0x64, 0x9b, 0x93, 0x4c, 0xa4, 0x95, 0x99, 0x1b, 0x78, 0x52, 0xb8, 0x55},
		TreeSize:       0,
		Signature: &trillian.DigitallySigned{
			HashAlgorithm:      trillian.HashAlgorithm_SHA256,
			SignatureAlgorithm: trillian.SignatureAlgorithm_ECDSA,
			Signature:          []byte("signed"),
		},
		TreeRevision: 1,
	}

	zeroDuration = 0 * time.Second
)

const writeRev = int64(24)

func TestSequencerManagerNothingToDo(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStorage := storage.NewMockLogStorage(mockCtrl)
	mockKeyManager := crypto.NewMockKeyManager(mockCtrl)
	mockKeyManager.EXPECT().SignatureAlgorithm().AnyTimes().Return(trillian.SignatureAlgorithm_ECDSA)
	mockKeyManager.EXPECT().HashAlgorithm().AnyTimes().Return(trillian.HashAlgorithm_SHA256)

	registry := registryForSequencer(mockStorage)
	sm := NewSequencerManager(mockKeyManager, registry, zeroDuration)

	sm.ExecutePass([]int64{}, createTestContext(registry))
}

func TestSequencerManagerSingleLogNoLeaves(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStorage := storage.NewMockLogStorage(mockCtrl)
	mockTx := storage.NewMockLogTreeTX(mockCtrl)
	var logID int64 = 1

	mockStorage.EXPECT().BeginForTree(gomock.Any(), logID).Return(mockTx, nil)
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().WriteRevision().AnyTimes().Return(writeRev)
	mockTx.EXPECT().LatestSignedLogRoot().Return(testRoot0, nil)
	mockTx.EXPECT().DequeueLeaves(50, fakeTime).Return([]trillian.LogLeaf{}, nil)
	mockKeyManager := crypto.NewMockKeyManager(mockCtrl)
	mockKeyManager.EXPECT().SignatureAlgorithm().AnyTimes().Return(trillian.SignatureAlgorithm_ECDSA)
	mockKeyManager.EXPECT().HashAlgorithm().AnyTimes().Return(trillian.HashAlgorithm_SHA256)

	registry := registryForSequencer(mockStorage)
	sm := NewSequencerManager(mockKeyManager, registry, zeroDuration)

	sm.ExecutePass([]int64{logID}, createTestContext(registry))
}

func TestSequencerManagerSingleLogOneLeaf(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStorage := storage.NewMockLogStorage(mockCtrl)
	mockTx := storage.NewMockLogTreeTX(mockCtrl)
	mockKeyManager := crypto.NewMockKeyManager(mockCtrl)
	mockKeyManager.EXPECT().SignatureAlgorithm().AnyTimes().Return(trillian.SignatureAlgorithm_ECDSA)
	mockKeyManager.EXPECT().HashAlgorithm().AnyTimes().Return(trillian.HashAlgorithm_SHA256)
	var logID int64 = 1

	// Set up enough mockery to be able to sequence. We don't test all the error paths
	// through sequencer as other tests cover this
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().Rollback().AnyTimes().Do(func() { panic(nil) })
	mockTx.EXPECT().WriteRevision().AnyTimes().Return(testRoot0.TreeRevision + 1)
	mockTx.EXPECT().DequeueLeaves(50, fakeTime).Return([]trillian.LogLeaf{testLeaf0}, nil)
	mockTx.EXPECT().LatestSignedLogRoot().Return(testRoot0, nil)
	mockTx.EXPECT().UpdateSequencedLeaves([]trillian.LogLeaf{testLeaf0Updated}).Return(nil)
	mockTx.EXPECT().SetMerkleNodes(updatedNodes0).Return(nil)
	mockTx.EXPECT().StoreSignedLogRoot(updatedRoot).Return(nil)
	mockStorage.EXPECT().BeginForTree(gomock.Any(), logID).Return(mockTx, nil)

	mockSigner := crypto.NewMockSigner(mockCtrl)
	mockSigner.EXPECT().Sign(gomock.Any(), []byte{23, 147, 61, 51, 131, 170, 136, 10, 82, 12, 93, 42, 98, 88, 131, 100, 101, 187, 124, 189, 202, 207, 66, 137, 95, 117, 205, 34, 109, 242, 103, 248}, gocrypto.SHA256).Return([]byte("signed"), nil)
	mockKeyManager.EXPECT().Signer().Return(mockSigner, nil)

	registry := registryForSequencer(mockStorage)
	sm := NewSequencerManager(mockKeyManager, registry, zeroDuration)

	sm.ExecutePass([]int64{logID}, createTestContext(registry))
}

func TestSequencerManagerGuardWindow(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStorage := storage.NewMockLogStorage(mockCtrl)
	mockTx := storage.NewMockLogTreeTX(mockCtrl)
	var logID int64 = 1

	mockStorage.EXPECT().BeginForTree(gomock.Any(), logID).Return(mockTx, nil)
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().WriteRevision().AnyTimes().Return(writeRev)
	mockTx.EXPECT().LatestSignedLogRoot().Return(testRoot0, nil)
	// Expect a 5 second guard window to be passed from manager -> sequencer -> storage
	mockTx.EXPECT().DequeueLeaves(50, fakeTime.Add(-time.Second*5)).Return([]trillian.LogLeaf{}, nil)
	mockKeyManager := crypto.NewMockKeyManager(mockCtrl)

	registry := registryForSequencer(mockStorage)
	sm := NewSequencerManager(mockKeyManager, registry, time.Second*5)

	sm.ExecutePass([]int64{logID}, createTestContext(registry))
}

func mockStorageProviderForSequencer(mockStorage storage.LogStorage) testonly.GetLogStorageFunc {
	return func() (storage.LogStorage, error) {
		return mockStorage, nil
	}
}

func registryForSequencer(mockStorage storage.LogStorage) extension.Registry {
	return testonly.NewRegistryWithLogProvider(mockStorageProviderForSequencer(mockStorage))
}

func createTestContext(registry extension.Registry) LogOperationManagerContext {
	// Set sign interval to 100 years so it won't trigger a root expiry signing unless overridden
	ctx := util.NewLogContext(context.Background(), -1)
	return LogOperationManagerContext{
		ctx:              ctx,
		registry:         registry,
		batchSize:        50,
		sleepBetweenRuns: time.Second,
		oneShot:          true,
		timeSource:       fakeTimeSource,
		signInterval:     time.Hour * 24 * 365 * 100,
	}
}
