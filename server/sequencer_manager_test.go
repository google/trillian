package server

import (
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/util"
)

// Arbitrary time for use in tests
var fakeTime = time.Date(2016, 6, 28, 13, 40, 12, 45, time.UTC)
var fakeTimeSource = util.FakeTimeSource{fakeTime}

// We use a size zero tree for testing, merkle tree state restore is tested elsewhere
var logID1 = trillian.LogID{TreeID: 1, LogID: []byte("testroot")}
var testLeaf0Hash = trillian.Hash{0, 1, 2, 3, 4, 5}
var testLeaf0 = trillian.LogLeaf{Leaf: trillian.Leaf{LeafHash: testLeaf0Hash, LeafValue: nil, ExtraData: nil}, SequenceNumber: 0}
var testRoot0 = trillian.SignedLogRoot{TreeSize: 0, TreeRevision: 0, LogId: logID1.LogID, RootHash: []byte{}}
var updatedNodes0 = []storage.Node{{NodeID: storage.NodeID{Path: []uint8{0x0}, PrefixLenBits: 0, PathLenBits: 6}, Hash: trillian.Hash{0x0, 0x1, 0x2, 0x3, 0x4, 0x5}, NodeRevision: 1}}
var updatedRoot = trillian.SignedLogRoot{LogId: logID1.LogID, TimestampNanos: fakeTime.UnixNano(), RootHash: []uint8{0x0, 0x1, 0x2, 0x3, 0x4, 0x5}, TreeSize: 1, Signature: &trillian.DigitallySigned{Signature: []byte("signed")}, TreeRevision: 1}

// This is used in the signing test with no work where the treesize will be zero
var updatedRootSignOnly = trillian.SignedLogRoot{LogId: logID1.LogID, TimestampNanos: fakeTime.UnixNano(), RootHash: []uint8{0xe3, 0xb0, 0xc4, 0x42, 0x98, 0xfc, 0x1c, 0x14, 0x9a, 0xfb, 0xf4, 0xc8, 0x99, 0x6f, 0xb9, 0x24, 0x27, 0xae, 0x41, 0xe4, 0x64, 0x9b, 0x93, 0x4c, 0xa4, 0x95, 0x99, 0x1b, 0x78, 0x52, 0xb8, 0x55}, TreeSize: 0, Signature: &trillian.DigitallySigned{Signature: []byte("signed")}, TreeRevision: 1}

const writeRev = int64(24)

func TestSequencerManagerNothingToDo(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStorage := storage.NewMockLogStorage(mockCtrl)
	mockKeyManager := crypto.NewMockKeyManager(mockCtrl)

	sm := NewSequencerManager(mockKeyManager)

	sm.ExecutePass([]trillian.LogID{}, createTestContext(mockStorageProviderForSequencer(mockStorage)))
}

func TestSequencerManagerSingleLogNoLeaves(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStorage := storage.NewMockLogStorage(mockCtrl)
	mockTx := storage.NewMockLogTX(mockCtrl)
	logID := trillian.LogID{TreeID: 1, LogID: []byte("Test")}

	mockStorage.EXPECT().Begin().Return(mockTx, nil)
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().WriteRevision().AnyTimes().Return(writeRev)
	mockTx.EXPECT().LatestSignedLogRoot().Return(testRoot0, nil)
	mockTx.EXPECT().DequeueLeaves(50).Return([]trillian.LogLeaf{}, nil)
	mockKeyManager := crypto.NewMockKeyManager(mockCtrl)

	sm := NewSequencerManager(mockKeyManager)

	sm.ExecutePass([]trillian.LogID{logID}, createTestContext(mockStorageProviderForSequencer(mockStorage)))
}

func TestSequencerManagerSingleLogOneLeaf(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStorage := storage.NewMockLogStorage(mockCtrl)
	mockTx := storage.NewMockLogTX(mockCtrl)
	mockKeyManager := crypto.NewMockKeyManager(mockCtrl)
	logID := trillian.LogID{TreeID: 1, LogID: []byte("Test")}
	hasher := trillian.NewSHA256()

	// Set up enough mockery to be able to sequence. We don't test all the error paths
	// through sequencer as other tests cover this
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().Rollback().AnyTimes().Do(func() { panic(nil) })
	mockTx.EXPECT().WriteRevision().AnyTimes().Return(testRoot0.TreeRevision + 1)
	mockTx.EXPECT().DequeueLeaves(50).Return([]trillian.LogLeaf{testLeaf0}, nil)
	mockTx.EXPECT().LatestSignedLogRoot().Return(testRoot0, nil)
	mockTx.EXPECT().UpdateSequencedLeaves([]trillian.LogLeaf{testLeaf0}).Return(nil)
	mockTx.EXPECT().SetMerkleNodes(updatedNodes0).Return(nil)
	mockTx.EXPECT().StoreSignedLogRoot(updatedRoot).Return(nil)
	mockStorage.EXPECT().Begin().Return(mockTx, nil)

	mockSigner := crypto.NewMockSigner(mockCtrl)
	mockSigner.EXPECT().Sign(gomock.Any(), []byte{0x13, 0xa6, 0xf3, 0xcb, 0xa2, 0x82, 0x52, 0xfc, 0x5a, 0x98, 0xfe, 0x81, 0x7c, 0xb7, 0xaf, 0x68, 0x1f, 0x83, 0x30, 0xcf, 0x80, 0x71, 0x1e, 0x9e, 0x16, 0xf6, 0x1e, 0x55, 0xcf, 0x78, 0xa, 0xb9}, hasher).Return([]byte("signed"), nil)
	mockKeyManager.EXPECT().Signer().Return(mockSigner, nil)

	sm := NewSequencerManager(mockKeyManager)

	sm.ExecutePass([]trillian.LogID{logID}, createTestContext(mockStorageProviderForSequencer(mockStorage)))
}

// Tests that a new root is signed if it's due even when there is no work to sequence.
// The various failure cases of SignRoot() are tested in the sequencer tests. This is
// an interaction test.
func TestSignsIfNoWorkAndRootExpired(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStorage := storage.NewMockLogStorage(mockCtrl)
	mockTx := storage.NewMockLogTX(mockCtrl)
	mockKeyManager := crypto.NewMockKeyManager(mockCtrl)
	logID := trillian.LogID{TreeID: 1, LogID: []byte("Test")}
	hasher := trillian.NewSHA256()

	mockStorage.EXPECT().Begin().AnyTimes().Return(mockTx, nil)
	mockTx.EXPECT().WriteRevision().AnyTimes().Return(writeRev)
	mockTx.EXPECT().Commit().AnyTimes().Return(nil)
	mockTx.EXPECT().LatestSignedLogRoot().AnyTimes().Return(testRoot0, nil)
	mockTx.EXPECT().DequeueLeaves(50).Return([]trillian.LogLeaf{}, nil)
	mockTx.EXPECT().StoreSignedLogRoot(updatedRootSignOnly).AnyTimes().Return(nil)

	mockSigner := crypto.NewMockSigner(mockCtrl)
	mockSigner.EXPECT().Sign(gomock.Any(), []byte{0xeb, 0x7d, 0xa1, 0x4f, 0x1e, 0x60, 0x91, 0x24, 0xa, 0xf7, 0x1c, 0xcd, 0xdb, 0xd4, 0xca, 0x38, 0x4b, 0x12, 0xe4, 0xa3, 0xcf, 0x80, 0x5, 0x55, 0x17, 0x71, 0x35, 0xaf, 0x80, 0x11, 0xa, 0x87}, hasher).Return([]byte("signed"), nil)
	mockKeyManager.EXPECT().Signer().Return(mockSigner, nil)

	sm := NewSequencerManager(mockKeyManager)

	tc := createTestContext(mockStorageProviderForSequencer(mockStorage))
	// Lower the expiry so we can trigger a signing for a root older than 5 seconds
	tc.signInterval = time.Second * 5
	sm.ExecutePass([]trillian.LogID{logID}, tc)
}

func mockStorageProviderForSequencer(mockStorage storage.LogStorage) LogStorageProviderFunc {
	return func(id int64) (storage.LogStorage, error) {
		if id >= 0 && id <= 1 {
			return mockStorage, nil
		} else {
			return nil, fmt.Errorf("BADLOGID: ", id)
		}
	}
}

func createTestContext(sp LogStorageProviderFunc) LogOperationManagerContext {
	done := make(chan struct{})

	// Set sign interval to 100 years so it won't trigger a root expiry signing unless overridden
	return LogOperationManagerContext{done: done, storageProvider: sp, batchSize: 50, sleepBetweenRuns: time.Second, oneShot: true, timeSource: fakeTimeSource, signInterval: time.Hour * 24 * 365 * 100}
}
