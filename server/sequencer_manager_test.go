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
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/util"
)

// Arbitrary time for use in tests
var fakeTime = time.Date(2016, 6, 28, 13, 40, 12, 45, time.UTC)
var fakeTimeSource = util.FakeTimeSource{FakeTime: fakeTime}

// We use a size zero tree for testing, Merkle tree state restore is tested elsewhere
var testLogID1 = int64(1)
var testLeaf0 = &trillian.LogLeaf{
	MerkleLeafHash: testonly.Hasher.HashLeaf([]byte{}),
	LeafValue:      nil,
	ExtraData:      nil,
	LeafIndex:      0,
}
var testLeaf0Updated = &trillian.LogLeaf{
	MerkleLeafHash: testonly.MustDecodeBase64("bjQLnP+zepicpUTmu3gKLHiQHT+zNzh2hRGjBhevoB0="),
	LeafValue:      nil,
	ExtraData:      nil,
	LeafIndex:      0,
}
var testRoot0 = trillian.SignedLogRoot{
	TreeSize:     0,
	TreeRevision: 0,
	LogId:        testLogID1,
	RootHash:     []byte{},
	Signature: &sigpb.DigitallySigned{
		HashAlgorithm:      sigpb.DigitallySigned_SHA256,
		SignatureAlgorithm: sigpb.DigitallySigned_ECDSA,
	},
}
var updatedNodes0 = []storage.Node{{NodeID: storage.NodeID{Path: []uint8{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, PrefixLenBits: 64, PathLenBits: 64}, Hash: testonly.MustDecodeBase64("bjQLnP+zepicpUTmu3gKLHiQHT+zNzh2hRGjBhevoB0="), NodeRevision: 1}}
var updatedRoot = trillian.SignedLogRoot{
	LogId:          testLogID1,
	TimestampNanos: fakeTime.UnixNano(),
	RootHash:       []byte{110, 52, 11, 156, 255, 179, 122, 152, 156, 165, 68, 230, 187, 120, 10, 44, 120, 144, 29, 63, 179, 55, 56, 118, 133, 17, 163, 6, 23, 175, 160, 29},
	TreeSize:       1,
	Signature: &sigpb.DigitallySigned{
		HashAlgorithm:      sigpb.DigitallySigned_SHA256,
		SignatureAlgorithm: sigpb.DigitallySigned_ECDSA,
		Signature:          []byte("signed"),
	},
	TreeRevision: 1,
}

var zeroDuration = 0 * time.Second

const writeRev = int64(24)

// newSignerWithFixedSig returns a fake signer that always returns the specified signature.
func newSignerWithFixedSig(sig *sigpb.DigitallySigned) (*crypto.Signer, error) {
	key, err := keys.NewFromPublicPEM(testonly.DemoPublicKey)
	if err != nil {
		return nil, err
	}

	if keys.SignatureAlgorithm(key) != sig.GetSignatureAlgorithm() {
		return nil, fmt.Errorf("signature algorithm does not match demo public key")
	}

	return crypto.NewSigner(testonly.NewSignerWithFixedSig(key, sig.Signature)), nil
}

func TestSequencerManagerNothingToDo(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStorage := storage.NewMockLogStorage(mockCtrl)

	registry := extension.NewMockRegistry(mockCtrl)
	registry.EXPECT().GetLogStorage().Return(mockStorage, nil)
	sm := NewSequencerManager(registry, zeroDuration)

	sm.ExecutePass([]int64{}, createTestContext(registry))
}

func TestSequencerManagerSingleLogNoLeaves(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStorage := storage.NewMockLogStorage(mockCtrl)
	mockTx := storage.NewMockLogTreeTX(mockCtrl)

	const logID int64 = 1
	signer, err := newSignerWithFixedSig(updatedRoot.Signature)
	if err != nil {
		t.Fatalf("Failed to create test signer (%v)", err)
	}

	mockStorage.EXPECT().BeginForTree(gomock.Any(), logID).Return(mockTx, nil)
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().Close().Return(nil)
	mockTx.EXPECT().WriteRevision().AnyTimes().Return(writeRev)
	mockTx.EXPECT().LatestSignedLogRoot().Return(testRoot0, nil)
	mockTx.EXPECT().DequeueLeaves(50, fakeTime).Return([]*trillian.LogLeaf{}, nil)

	registry := extension.NewMockRegistry(mockCtrl)
	registry.EXPECT().GetLogStorage().Return(mockStorage, nil)
	registry.EXPECT().GetSigner(logID).Return(signer, nil)
	sm := NewSequencerManager(registry, zeroDuration)

	sm.ExecutePass([]int64{logID}, createTestContext(registry))
}

func TestSequencerManagerSingleLogOneLeaf(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStorage := storage.NewMockLogStorage(mockCtrl)
	mockTx := storage.NewMockLogTreeTX(mockCtrl)

	const logID int64 = 1
	signer, err := newSignerWithFixedSig(updatedRoot.Signature)
	if err != nil {
		t.Fatalf("Failed to create test signer (%v)", err)
	}

	// Set up enough mockery to be able to sequence. We don't test all the error paths
	// through sequencer as other tests cover this
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().Close().Return(nil)
	mockTx.EXPECT().WriteRevision().AnyTimes().Return(testRoot0.TreeRevision + 1)
	mockTx.EXPECT().DequeueLeaves(50, fakeTime).Return([]*trillian.LogLeaf{testLeaf0}, nil)
	mockTx.EXPECT().LatestSignedLogRoot().Return(testRoot0, nil)
	mockTx.EXPECT().UpdateSequencedLeaves([]*trillian.LogLeaf{testLeaf0Updated}).Return(nil)
	mockTx.EXPECT().SetMerkleNodes(updatedNodes0).Return(nil)
	mockTx.EXPECT().StoreSignedLogRoot(updatedRoot).Return(nil)
	mockStorage.EXPECT().BeginForTree(gomock.Any(), logID).Return(mockTx, nil)

	registry := extension.NewMockRegistry(mockCtrl)
	registry.EXPECT().GetLogStorage().Return(mockStorage, nil)
	registry.EXPECT().GetSigner(logID).Return(signer, nil)
	sm := NewSequencerManager(registry, zeroDuration)

	sm.ExecutePass([]int64{logID}, createTestContext(registry))
}

func TestSequencerManagerGuardWindow(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStorage := storage.NewMockLogStorage(mockCtrl)
	mockTx := storage.NewMockLogTreeTX(mockCtrl)

	const logID int64 = 1
	signer, err := newSignerWithFixedSig(updatedRoot.Signature)
	if err != nil {
		t.Fatalf("Failed to create test signer (%v)", err)
	}

	mockStorage.EXPECT().BeginForTree(gomock.Any(), logID).Return(mockTx, nil)
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().Close().Return(nil)
	mockTx.EXPECT().WriteRevision().AnyTimes().Return(writeRev)
	mockTx.EXPECT().LatestSignedLogRoot().Return(testRoot0, nil)
	// Expect a 5 second guard window to be passed from manager -> sequencer -> storage
	mockTx.EXPECT().DequeueLeaves(50, fakeTime.Add(-time.Second*5)).Return([]*trillian.LogLeaf{}, nil)

	registry := extension.NewMockRegistry(mockCtrl)
	registry.EXPECT().GetLogStorage().Return(mockStorage, nil)
	registry.EXPECT().GetSigner(logID).Return(signer, nil)
	sm := NewSequencerManager(registry, time.Second*5)

	sm.ExecutePass([]int64{logID}, createTestContext(registry))
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
	}
}
