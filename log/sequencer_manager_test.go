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
	"crypto"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/golang/protobuf/ptypes"
	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/types"
	"github.com/google/trillian/util/clock"

	tcrypto "github.com/google/trillian/crypto"
	stestonly "github.com/google/trillian/storage/testonly"
	"github.com/google/trillian/storage/tree"
)

// Arbitrary time for use in tests
var fakeTimeSource = clock.NewFake(fakeTime)

// We use a size zero tree for testing, Merkle tree state restore is tested elsewhere
var (
	testLogID1 = int64(1)
	leaf0Hash  = rfc6962.DefaultHasher.HashLeaf([]byte{})
	testLeaf0  = &trillian.LogLeaf{
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
	fixedLog1Signer = tcrypto.NewSigner(testLogID1, fixedGoSigner, crypto.SHA256)
	testRoot0       = &types.LogRootV1{
		TreeSize: 0,
		Revision: 0,
		RootHash: []byte{},
	}
	testSignedRoot0, _ = fixedLog1Signer.SignLogRoot(testRoot0)

	updatedNodes0 = []tree.Node{{NodeID: tree.NodeID{Path: []uint8{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, PrefixLenBits: 64}, Hash: testonly.MustDecodeBase64("bjQLnP+zepicpUTmu3gKLHiQHT+zNzh2hRGjBhevoB0="), NodeRevision: 1}}
	updatedRoot   = &types.LogRootV1{
		TimestampNanos: uint64(fakeTime.UnixNano()),
		RootHash:       []byte{110, 52, 11, 156, 255, 179, 122, 152, 156, 165, 68, 230, 187, 120, 10, 44, 120, 144, 29, 63, 179, 55, 56, 118, 133, 17, 163, 6, 23, 175, 160, 29},
		TreeSize:       1,
		Revision:       1,
	}
	updatedSignedRoot, _ = fixedSigner.SignLogRoot(updatedRoot)
)

var zeroDuration = 0 * time.Second

const writeRev = int64(24)

func TestSequencerManagerSingleLogNoLeaves(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	logID := stestonly.LogTree.GetTreeId()
	mockAdminTx := storage.NewMockReadOnlyAdminTX(mockCtrl)
	mockAdmin := &stestonly.FakeAdminStorage{ReadOnlyTX: []storage.ReadOnlyAdminTX{mockAdminTx}}
	mockTx := storage.NewMockLogTreeTX(mockCtrl)
	fakeStorage := &stestonly.FakeLogStorage{TX: mockTx}

	var keyProto ptypes.DynamicAny
	if err := ptypes.UnmarshalAny(stestonly.LogTree.PrivateKey, &keyProto); err != nil {
		t.Fatalf("Failed to unmarshal stestonly.LogTree.PrivateKey: %v", err)
	}

	keys.RegisterHandler(fakeKeyProtoHandler(keyProto.Message, fixedGoSigner, nil))
	defer keys.UnregisterHandler(keyProto.Message)

	mockTx.EXPECT().Commit(gomock.Any()).Return(nil)
	mockTx.EXPECT().Close().Return(nil)
	mockTx.EXPECT().WriteRevision(gomock.Any()).AnyTimes().Return(writeRev, nil)
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

	var keyProto ptypes.DynamicAny
	if err := ptypes.UnmarshalAny(stestonly.LogTree.PrivateKey, &keyProto); err != nil {
		t.Fatalf("Failed to unmarshal stestonly.LogTree.PrivateKey: %v", err)
	}

	keys.RegisterHandler(fakeKeyProtoHandler(keyProto.Message, fixedGoSigner, nil))

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
			mockTx.EXPECT().WriteRevision(gomock.Any()).AnyTimes().Return(writeRev, nil),
			mockTx.EXPECT().Commit(gomock.Any()).Return(nil),
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
	mockAdminTx := storage.NewMockReadOnlyAdminTX(mockCtrl)
	mockAdmin := &stestonly.FakeAdminStorage{ReadOnlyTX: []storage.ReadOnlyAdminTX{mockAdminTx}}
	fakeStorage := &stestonly.FakeLogStorage{}

	var keyProto ptypes.DynamicAny
	if err := ptypes.UnmarshalAny(stestonly.LogTree.PrivateKey, &keyProto); err != nil {
		t.Fatalf("Failed to unmarshal stestonly.LogTree.PrivateKey: %v", err)
	}

	keys.RegisterHandler(fakeKeyProtoHandler(keyProto.Message, nil, errors.New("no signer for this tree")))
	defer keys.UnregisterHandler(keyProto.Message)

	gomock.InOrder(
		mockAdminTx.EXPECT().GetTree(gomock.Any(), logID).Return(stestonly.LogTree, nil),
		mockAdminTx.EXPECT().Commit().Return(nil),
		mockAdminTx.EXPECT().Close().Return(nil),
	)

	registry := extension.Registry{
		AdminStorage: mockAdmin,
		LogStorage:   fakeStorage,
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
	mockAdminTx := storage.NewMockReadOnlyAdminTX(mockCtrl)
	mockAdmin := &stestonly.FakeAdminStorage{ReadOnlyTX: []storage.ReadOnlyAdminTX{mockAdminTx}}
	mockTx := storage.NewMockLogTreeTX(mockCtrl)
	fakeStorage := &stestonly.FakeLogStorage{TX: mockTx}

	var keyProto ptypes.DynamicAny
	if err := ptypes.UnmarshalAny(stestonly.LogTree.PrivateKey, &keyProto); err != nil {
		t.Fatalf("Failed to unmarshal stestonly.LogTree.PrivateKey: %v", err)
	}

	keys.RegisterHandler(fakeKeyProtoHandler(keyProto.Message, fixedGoSigner, nil))
	defer keys.UnregisterHandler(keyProto.Message)

	// Set up enough mockery to be able to sequence. We don't test all the error paths
	// through sequencer as other tests cover this
	mockTx.EXPECT().Commit(gomock.Any()).Return(nil)
	mockTx.EXPECT().Close().Return(nil)
	mockTx.EXPECT().WriteRevision(gomock.Any()).AnyTimes().Return(int64(testRoot0.Revision+1), nil)
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

	var keyProto ptypes.DynamicAny
	if err := ptypes.UnmarshalAny(stestonly.LogTree.PrivateKey, &keyProto); err != nil {
		t.Fatalf("Failed to unmarshal stestonly.LogTree.PrivateKey: %v", err)
	}

	keys.RegisterHandler(fakeKeyProtoHandler(keyProto.Message, fixedGoSigner, nil))
	defer keys.UnregisterHandler(keyProto.Message)

	mockTx.EXPECT().Commit(gomock.Any()).Return(nil)
	mockTx.EXPECT().Close().Return(nil)
	mockTx.EXPECT().WriteRevision(gomock.Any()).AnyTimes().Return(writeRev, nil)
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

func fakeKeyProtoHandler(wantKeyProto proto.Message, signer crypto.Signer, err error) (proto.Message, keys.ProtoHandler) {
	return wantKeyProto, func(ctx context.Context, gotKeyProto proto.Message) (crypto.Signer, error) {
		if proto.Equal(wantKeyProto, gotKeyProto) {
			return signer, err
		}
		return nil, fmt.Errorf("fakeKeyProtoHandler: got %s, want %s", gotKeyProto, wantKeyProto)
	}
}
