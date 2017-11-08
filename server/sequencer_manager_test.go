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
	"crypto"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian"
	tcrypto "github.com/google/trillian/crypto"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/crypto/keys/pem"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/storage"
	stestonly "github.com/google/trillian/storage/testonly"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/util"
)

// Arbitrary time for use in tests
var fakeTime = time.Date(2016, 6, 28, 13, 40, 12, 45, time.UTC)
var fakeTimeSource = util.NewFakeTimeSource(fakeTime)

// We use a size zero tree for testing, Merkle tree state restore is tested elsewhere
var testLogID1 = int64(1)
var leaf0Hash, _ = rfc6962.DefaultHasher.HashLeaf([]byte{})
var testLeaf0 = &trillian.LogLeaf{
	MerkleLeafHash: leaf0Hash,
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
var updatedNodes0 = []storage.Node{{NodeID: storage.NodeID{Path: []uint8{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, PrefixLenBits: 64}, Hash: testonly.MustDecodeBase64("bjQLnP+zepicpUTmu3gKLHiQHT+zNzh2hRGjBhevoB0="), NodeRevision: 1}}
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
func newSignerWithFixedSig(sig *sigpb.DigitallySigned) (crypto.Signer, error) {
	key, err := pem.UnmarshalPublicKey(testonly.DemoPublicKey)
	if err != nil {
		return nil, err
	}

	if tcrypto.SignatureAlgorithm(key) != sig.GetSignatureAlgorithm() {
		return nil, errors.New("signature algorithm does not match demo public key")
	}

	return testonly.NewSignerWithFixedSig(key, sig.Signature), nil
}

func TestSequencerManagerSingleLogNoLeaves(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	logID := stestonly.LogTree.GetTreeId()
	mockAdmin := storage.NewMockAdminStorage(mockCtrl)
	mockAdminTx := storage.NewMockReadOnlyAdminTX(mockCtrl)
	mockStorage := storage.NewMockLogStorage(mockCtrl)
	mockTx := storage.NewMockLogTreeTX(mockCtrl)

	var keyProto ptypes.DynamicAny
	if err := ptypes.UnmarshalAny(stestonly.LogTree.PrivateKey, &keyProto); err != nil {
		t.Fatalf("Failed to unmarshal stestonly.LogTree.PrivateKey: %v", err)
	}

	signer, err := newSignerWithFixedSig(updatedRoot.Signature)
	if err != nil {
		t.Fatalf("Failed to create fake signer: %v", err)
	}

	keys.RegisterHandler(fakeKeyProtoHandler(keyProto.Message, signer, nil))
	defer keys.UnregisterHandler(keyProto.Message)

	mockStorage.EXPECT().BeginForTree(gomock.Any(), logID).Return(mockTx, nil)
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().Close().Return(nil)
	mockTx.EXPECT().WriteRevision().AnyTimes().Return(writeRev)
	mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(testRoot0, nil)
	mockTx.EXPECT().DequeueLeaves(gomock.Any(), 50, fakeTime).Return([]*trillian.LogLeaf{}, nil)

	mockAdmin.EXPECT().Snapshot(gomock.Any()).Return(mockAdminTx, nil)
	mockAdminTx.EXPECT().GetTree(gomock.Any(), logID).Return(stestonly.LogTree, nil)
	mockAdminTx.EXPECT().Commit().Return(nil)
	mockAdminTx.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: mockAdmin,
		LogStorage:   mockStorage,
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
	mockAdmin := storage.NewMockAdminStorage(mockCtrl)
	mockAdminTx := storage.NewMockReadOnlyAdminTX(mockCtrl)
	mockStorage := storage.NewMockLogStorage(mockCtrl)
	mockTx := storage.NewMockLogTreeTX(mockCtrl)

	var keyProto ptypes.DynamicAny
	if err := ptypes.UnmarshalAny(stestonly.LogTree.PrivateKey, &keyProto); err != nil {
		t.Fatalf("Failed to unmarshal stestonly.LogTree.PrivateKey: %v", err)
	}

	signer, err := newSignerWithFixedSig(updatedRoot.Signature)
	if err != nil {
		t.Fatalf("Failed to create fake signer: %v", err)
	}

	keys.RegisterHandler(fakeKeyProtoHandler(keyProto.Message, signer, nil))

	registry := extension.Registry{
		AdminStorage: mockAdmin,
		LogStorage:   mockStorage,
		QuotaManager: quota.Noop(),
	}
	sm := NewSequencerManager(registry, zeroDuration)

	// Expect two sequencing passes.
	for i := 0; i < 2; i++ {
		gomock.InOrder(
			mockAdmin.EXPECT().Snapshot(gomock.Any()).Return(mockAdminTx, nil),
			mockAdminTx.EXPECT().GetTree(gomock.Any(), logID).Return(stestonly.LogTree, nil),
			mockAdminTx.EXPECT().Commit().Return(nil),
			mockAdminTx.EXPECT().Close().Return(nil),
		)

		gomock.InOrder(
			mockStorage.EXPECT().BeginForTree(gomock.Any(), logID).Return(mockTx, nil),
			mockTx.EXPECT().DequeueLeaves(gomock.Any(), 50, fakeTime).Return([]*trillian.LogLeaf{}, nil),
			mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(testRoot0, nil),
			mockTx.EXPECT().WriteRevision().AnyTimes().Return(writeRev),
			mockTx.EXPECT().Commit().Return(nil),
			mockTx.EXPECT().Close().Return(nil),
		)

		if _, err := sm.ExecutePass(ctx, logID, createTestInfo(registry)); err != nil {
			t.Fatal(err)
		}

		// Remove the ProtoHandler added earlier in the test.
		// This guarantees that no further calls to keys.NewSigner() will succeed.
		// This tests that the signer obtained by SequencerManager during the first sequencing
		// pass is cached and re-used for the second pass.
		keys.UnregisterHandler(keyProto.Message)
	}
}

// Test that sequencing is skipped if no signer is available.
func TestSequencerManagerSingleLogNoSigner(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	logID := stestonly.LogTree.GetTreeId()
	mockAdmin := storage.NewMockAdminStorage(mockCtrl)
	mockAdminTx := storage.NewMockReadOnlyAdminTX(mockCtrl)
	mockStorage := storage.NewMockLogStorage(mockCtrl)

	var keyProto ptypes.DynamicAny
	if err := ptypes.UnmarshalAny(stestonly.LogTree.PrivateKey, &keyProto); err != nil {
		t.Fatalf("Failed to unmarshal stestonly.LogTree.PrivateKey: %v", err)
	}

	keys.RegisterHandler(fakeKeyProtoHandler(keyProto.Message, nil, errors.New("no signer for this tree")))
	defer keys.UnregisterHandler(keyProto.Message)

	gomock.InOrder(
		mockAdmin.EXPECT().Snapshot(gomock.Any()).Return(mockAdminTx, nil),
		mockAdminTx.EXPECT().GetTree(gomock.Any(), logID).Return(stestonly.LogTree, nil),
		mockAdminTx.EXPECT().Commit().Return(nil),
		mockAdminTx.EXPECT().Close().Return(nil),
	)

	registry := extension.Registry{
		AdminStorage: mockAdmin,
		LogStorage:   mockStorage,
		QuotaManager: quota.Noop(),
	}

	sm := NewSequencerManager(registry, zeroDuration)
	if _, err := sm.ExecutePass(ctx, logID, createTestInfo(registry)); err == nil {
		t.Fatal("ExecutePass() = (_, nil), want err")
	}
}

func TestSequencerManagerSingleLogOneLeaf(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	logID := stestonly.LogTree.GetTreeId()
	mockAdmin := storage.NewMockAdminStorage(mockCtrl)
	mockAdminTx := storage.NewMockReadOnlyAdminTX(mockCtrl)
	mockStorage := storage.NewMockLogStorage(mockCtrl)
	mockTx := storage.NewMockLogTreeTX(mockCtrl)

	var keyProto ptypes.DynamicAny
	if err := ptypes.UnmarshalAny(stestonly.LogTree.PrivateKey, &keyProto); err != nil {
		t.Fatalf("Failed to unmarshal stestonly.LogTree.PrivateKey: %v", err)
	}

	signer, err := newSignerWithFixedSig(updatedRoot.Signature)
	if err != nil {
		t.Fatalf("Failed to create fake signer: %v", err)
	}

	keys.RegisterHandler(fakeKeyProtoHandler(keyProto.Message, signer, nil))
	defer keys.UnregisterHandler(keyProto.Message)

	// Set up enough mockery to be able to sequence. We don't test all the error paths
	// through sequencer as other tests cover this
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().Close().Return(nil)
	mockTx.EXPECT().WriteRevision().AnyTimes().Return(testRoot0.TreeRevision + 1)
	mockTx.EXPECT().DequeueLeaves(gomock.Any(), 50, fakeTime).Return([]*trillian.LogLeaf{testLeaf0}, nil)
	mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(testRoot0, nil)
	mockTx.EXPECT().UpdateSequencedLeaves(gomock.Any(), []*trillian.LogLeaf{testLeaf0Updated}).Return(nil)
	mockTx.EXPECT().SetMerkleNodes(gomock.Any(), updatedNodes0).Return(nil)
	mockTx.EXPECT().StoreSignedLogRoot(gomock.Any(), updatedRoot).Return(nil)
	mockStorage.EXPECT().BeginForTree(gomock.Any(), logID).Return(mockTx, nil)

	mockAdmin.EXPECT().Snapshot(gomock.Any()).Return(mockAdminTx, nil)
	mockAdminTx.EXPECT().GetTree(gomock.Any(), logID).Return(stestonly.LogTree, nil)
	mockAdminTx.EXPECT().Commit().Return(nil)
	mockAdminTx.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: mockAdmin,
		LogStorage:   mockStorage,
		QuotaManager: quota.Noop(),
	}

	sm := NewSequencerManager(registry, zeroDuration)
	sm.ExecutePass(ctx, logID, createTestInfo(registry))
}

func TestSequencerManagerGuardWindow(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	logID := stestonly.LogTree.GetTreeId()
	mockAdmin := storage.NewMockAdminStorage(mockCtrl)
	mockAdminTx := storage.NewMockReadOnlyAdminTX(mockCtrl)
	mockStorage := storage.NewMockLogStorage(mockCtrl)
	mockTx := storage.NewMockLogTreeTX(mockCtrl)

	var keyProto ptypes.DynamicAny
	if err := ptypes.UnmarshalAny(stestonly.LogTree.PrivateKey, &keyProto); err != nil {
		t.Fatalf("Failed to unmarshal stestonly.LogTree.PrivateKey: %v", err)
	}

	signer, err := newSignerWithFixedSig(updatedRoot.Signature)
	if err != nil {
		t.Fatalf("Failed to create fake signer: %v", err)
	}

	keys.RegisterHandler(fakeKeyProtoHandler(keyProto.Message, signer, nil))
	defer keys.UnregisterHandler(keyProto.Message)

	mockStorage.EXPECT().BeginForTree(gomock.Any(), logID).Return(mockTx, nil)
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().Close().Return(nil)
	mockTx.EXPECT().WriteRevision().AnyTimes().Return(writeRev)
	mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(testRoot0, nil)
	// Expect a 5 second guard window to be passed from manager -> sequencer -> storage
	mockTx.EXPECT().DequeueLeaves(gomock.Any(), 50, fakeTime.Add(-time.Second*5)).Return([]*trillian.LogLeaf{}, nil)

	mockAdmin.EXPECT().Snapshot(gomock.Any()).Return(mockAdminTx, nil)
	mockAdminTx.EXPECT().GetTree(gomock.Any(), logID).Return(stestonly.LogTree, nil)
	mockAdminTx.EXPECT().Commit().Return(nil)
	mockAdminTx.EXPECT().Close().Return(nil)

	registry := extension.Registry{
		AdminStorage: mockAdmin,
		LogStorage:   mockStorage,
		QuotaManager: quota.Noop(),
	}

	sm := NewSequencerManager(registry, time.Second*5)
	sm.ExecutePass(ctx, logID, createTestInfo(registry))
}

func createTestInfo(registry extension.Registry) *LogOperationInfo {
	// Set sign interval to 100 years so it won't trigger a root expiry signing unless overridden
	return &LogOperationInfo{
		Registry:    registry,
		BatchSize:   50,
		RunInterval: time.Second,
		TimeSource:  fakeTimeSource,
	}
}

func fakeKeyProtoHandler(wantKeyProto proto.Message, signer crypto.Signer, err error) (proto.Message, keys.ProtoHandler) {
	return wantKeyProto, func(ctx context.Context, gotKeyProto proto.Message) (crypto.Signer, error) {
		if proto.Equal(wantKeyProto, gotKeyProto) {
			return signer, err
		}
		return nil, fmt.Errorf("fakeKeyProtoHandler: got %s, want %s", gotKeyProto, wantKeyProto)
	}
}
