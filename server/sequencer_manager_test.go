package server

import (
	"fmt"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/golang/mock/gomock"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/util"
)

// Arbitrary time for use in tests
var fakeTime = time.Date(2016, 6, 28, 13, 40, 12, 45, time.UTC)
var fakeTimeSource = util.FakeTimeSource{FakeTime: fakeTime}

// We use a size zero tree for testing, Merkle tree state restore is tested elsewhere
var testLogID1 = int64(1)
var testLeaf0Hash = []byte{0, 1, 2, 3, 4, 5}
var testLeaf0 = trillian.LogLeaf{MerkleLeafHash: testLeaf0Hash, LeafValue: nil, ExtraData: nil, LeafIndex: 0}
var testLeaf0Updated = trillian.LogLeaf{MerkleLeafHash: testonly.MustDecodeBase64("bjQLnP+zepicpUTmu3gKLHiQHT+zNzh2hRGjBhevoB0="), LeafValue: nil, ExtraData: nil, LeafIndex: 0}
var testRoot0 = trillian.SignedLogRoot{TreeSize: 0, TreeRevision: 0, LogId: testLogID1, RootHash: []byte{}, Signature: &trillian.DigitallySigned{SignatureAlgorithm: trillian.SignatureAlgorithm_ECDSA}}
var updatedNodes0 = []storage.Node{{NodeID: storage.NodeID{Path: []uint8{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, PrefixLenBits: 64, PathLenBits: 64}, Hash: testonly.MustDecodeBase64("bjQLnP+zepicpUTmu3gKLHiQHT+zNzh2hRGjBhevoB0="), NodeRevision: 1}}
var updatedRoot = trillian.SignedLogRoot{LogId: testLogID1, TimestampNanos: fakeTime.UnixNano(), RootHash: []byte{110, 52, 11, 156, 255, 179, 122, 152, 156, 165, 68, 230, 187, 120, 10, 44, 120, 144, 29, 63, 179, 55, 56, 118, 133, 17, 163, 6, 23, 175, 160, 29}, TreeSize: 1, Signature: &trillian.DigitallySigned{SignatureAlgorithm: trillian.SignatureAlgorithm_ECDSA, Signature: []byte("signed")}, TreeRevision: 1}

// This is used in the signing test with no work where the treesize will be zero
var updatedRootSignOnly = trillian.SignedLogRoot{LogId: testLogID1, TimestampNanos: fakeTime.UnixNano(), RootHash: []uint8{0xe3, 0xb0, 0xc4, 0x42, 0x98, 0xfc, 0x1c, 0x14, 0x9a, 0xfb, 0xf4, 0xc8, 0x99, 0x6f, 0xb9, 0x24, 0x27, 0xae, 0x41, 0xe4, 0x64, 0x9b, 0x93, 0x4c, 0xa4, 0x95, 0x99, 0x1b, 0x78, 0x52, 0xb8, 0x55}, TreeSize: 0, Signature: &trillian.DigitallySigned{Signature: []byte("signed")}, TreeRevision: 1}

var zeroDuration = 0 * time.Second

const writeRev = int64(24)

func TestSequencerManagerNothingToDo(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStorage := storage.NewMockLogStorage(mockCtrl)
	mockKeyManager := crypto.NewMockKeyManager(mockCtrl)

	provider := mockStorageProviderForSequencer(mockStorage)
	sm := NewSequencerManager(mockKeyManager, provider, zeroDuration)

	sm.ExecutePass([]int64{}, createTestContext(provider))
}

func TestSequencerManagerSingleLogNoLeaves(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStorage := storage.NewMockLogStorage(mockCtrl)
	mockTx := storage.NewMockLogTX(mockCtrl)
	logID := int64(1)

	mockStorage.EXPECT().Begin().Return(mockTx, nil)
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().WriteRevision().AnyTimes().Return(writeRev)
	mockTx.EXPECT().LatestSignedLogRoot().Return(testRoot0, nil)
	mockTx.EXPECT().DequeueLeaves(50, fakeTime).Return([]trillian.LogLeaf{}, nil)
	mockKeyManager := crypto.NewMockKeyManager(mockCtrl)

	provider := mockStorageProviderForSequencer(mockStorage)
	sm := NewSequencerManager(mockKeyManager, provider, zeroDuration)

	sm.ExecutePass([]int64{logID}, createTestContext(provider))
}

func TestSequencerManagerSingleLogOneLeaf(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStorage := storage.NewMockLogStorage(mockCtrl)
	mockTx := storage.NewMockLogTX(mockCtrl)
	mockKeyManager := crypto.NewMockKeyManager(mockCtrl)
	logID := int64(1)
	hasher := crypto.NewSHA256()

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
	mockStorage.EXPECT().Begin().Return(mockTx, nil)

	mockSigner := crypto.NewMockSigner(mockCtrl)
	mockSigner.EXPECT().Sign(gomock.Any(), []byte{23, 147, 61, 51, 131, 170, 136, 10, 82, 12, 93, 42, 98, 88, 131, 100, 101, 187, 124, 189, 202, 207, 66, 137, 95, 117, 205, 34, 109, 242, 103, 248}, hasher).Return([]byte("signed"), nil)
	mockKeyManager.EXPECT().Signer().Return(mockSigner, nil)

	provider := mockStorageProviderForSequencer(mockStorage)
	sm := NewSequencerManager(mockKeyManager, provider, zeroDuration)

	sm.ExecutePass([]int64{logID}, createTestContext(provider))
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
	logID := int64(1)
	hasher := crypto.NewSHA256()

	mockStorage.EXPECT().Begin().AnyTimes().Return(mockTx, nil)
	mockTx.EXPECT().WriteRevision().AnyTimes().Return(writeRev)
	mockTx.EXPECT().Commit().AnyTimes().Return(nil)
	mockTx.EXPECT().LatestSignedLogRoot().AnyTimes().Return(testRoot0, nil)
	mockTx.EXPECT().DequeueLeaves(50, fakeTime).Return([]trillian.LogLeaf{}, nil)
	mockTx.EXPECT().StoreSignedLogRoot(updatedRootSignOnly).AnyTimes().Return(nil)

	mockSigner := crypto.NewMockSigner(mockCtrl)
	mockSigner.EXPECT().Sign(gomock.Any(), []byte{0xeb, 0x7d, 0xa1, 0x4f, 0x1e, 0x60, 0x91, 0x24, 0xa, 0xf7, 0x1c, 0xcd, 0xdb, 0xd4, 0xca, 0x38, 0x4b, 0x12, 0xe4, 0xa3, 0xcf, 0x80, 0x5, 0x55, 0x17, 0x71, 0x35, 0xaf, 0x80, 0x11, 0xa, 0x87}, hasher).Return([]byte("signed"), nil)
	mockKeyManager.EXPECT().Signer().Return(mockSigner, nil)

	provider := mockStorageProviderForSequencer(mockStorage)
	sm := NewSequencerManager(mockKeyManager, provider, zeroDuration)

	tc := createTestContext(provider)
	// Lower the expiry so we can trigger a signing for a root older than 5 seconds
	tc.signInterval = time.Second * 5
	sm.ExecutePass([]int64{logID}, tc)
}

func TestSequencerManagerGuardWindow(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStorage := storage.NewMockLogStorage(mockCtrl)
	mockTx := storage.NewMockLogTX(mockCtrl)
	logID := int64(1)

	mockStorage.EXPECT().Begin().Return(mockTx, nil)
	mockTx.EXPECT().Commit().Return(nil)
	mockTx.EXPECT().WriteRevision().AnyTimes().Return(writeRev)
	mockTx.EXPECT().LatestSignedLogRoot().Return(testRoot0, nil)
	// Expect a 5 second guard window to be passed from manager -> sequencer -> storage
	mockTx.EXPECT().DequeueLeaves(50, fakeTime.Add(-time.Second*5)).Return([]trillian.LogLeaf{}, nil)
	mockKeyManager := crypto.NewMockKeyManager(mockCtrl)

	provider := mockStorageProviderForSequencer(mockStorage)
	sm := NewSequencerManager(mockKeyManager, provider, time.Second*5)

	sm.ExecutePass([]int64{logID}, createTestContext(mockStorageProviderForSequencer(mockStorage)))
}

func mockStorageProviderForSequencer(mockStorage storage.LogStorage) LogStorageProviderFunc {
	return func(id int64) (storage.LogStorage, error) {
		if id >= 0 && id <= 1 {
			return mockStorage, nil
		}
		return nil, fmt.Errorf("BADLOGID: %d", id)
	}
}

func createTestContext(sp LogStorageProviderFunc) LogOperationManagerContext {
	// Set sign interval to 100 years so it won't trigger a root expiry signing unless overridden
	ctx := util.NewLogContext(context.Background(), -1)
	return LogOperationManagerContext{
		ctx:              ctx,
		cachedProvider:   newCachedLogStorageProvider(sp),
		batchSize:        50,
		sleepBetweenRuns: time.Second,
		oneShot:          true,
		timeSource:       fakeTimeSource,
		signInterval:     time.Hour * 24 * 365 * 100,
	}
}
