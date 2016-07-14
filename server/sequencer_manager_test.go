package server

import (
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/util"
	"github.com/stretchr/testify/mock"
)

// Arbitrary time for use in tests
var fakeTime = time.Date(2016, 6, 28, 13, 40, 12, 45, time.UTC)
var fakeTimeSource = util.FakeTimeSource{fakeTime}

// We use a size zero tree for testing, merkle tree state restore is tested elsewhere
var logID1 = trillian.LogID{TreeID: 1, LogID: []byte("testroot")}
var testLeaf0Hash = trillian.Hash{0, 1, 2, 3, 4, 5}
var testLeaf0 = trillian.LogLeaf{Leaf: trillian.Leaf{LeafHash: testLeaf0Hash, LeafValue: nil, ExtraData: nil}, SequenceNumber: 0}
var testRoot0 = trillian.SignedLogRoot{TreeSize: 0, TreeRevision: 0, LogId: logID1.LogID}
var updatedNodes0 = []storage.Node{storage.Node{NodeID: storage.NodeID{Path: []uint8{0x0}, PrefixLenBits: 0, PathLenBits: 6}, Hash: trillian.Hash{0x0, 0x1, 0x2, 0x3, 0x4, 0x5}, NodeRevision: 1}}
var updatedRoot = trillian.SignedLogRoot{LogId: logID1.LogID, TimestampNanos: fakeTime.UnixNano(), RootHash: []uint8{0x0, 0x1, 0x2, 0x3, 0x4, 0x5}, TreeSize: 1, Signature: &trillian.DigitallySigned{Signature: []byte("signed")}, TreeRevision: 1}

func TestSequencerManagerNothingToDo(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)
	mockTx := new(storage.MockLogTX)
	mockKeyManager := new(crypto.MockKeyManager)

	sm := NewSequencerManager(mockKeyManager)

	sm.ExecutePass([]trillian.LogID{}, createTestContext(mockStorageProviderForSequencer(mockStorage)))

	mockStorage.AssertExpectations(t)
	mockTx.AssertExpectations(t)
}

func TestSequencerManagerSingleLogNoLeaves(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)
	mockTx := new(storage.MockLogTX)
	logID := trillian.LogID{TreeID: 1, LogID: []byte("Test")}

	mockStorage.On("Begin").Return(mockTx, nil)
	mockTx.On("Commit").Return(nil)
	mockTx.On("DequeueLeaves", 50).Return([]trillian.LogLeaf{}, nil)
	mockKeyManager := new(crypto.MockKeyManager)

	sm := NewSequencerManager(mockKeyManager)

	sm.ExecutePass([]trillian.LogID{logID}, createTestContext(mockStorageProviderForSequencer(mockStorage)))

	mockStorage.AssertExpectations(t)
	mockTx.AssertExpectations(t)
}

func TestSequencerManagerSingleLogOneLeaf(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)
	mockTx := new(storage.MockLogTX)
	mockKeyManager := new(crypto.MockKeyManager)
	logID := trillian.LogID{TreeID: 1, LogID: []byte("Test")}
	hasher := trillian.NewSHA256()

	// Set up enough mockery to be able to sequence. We don't test all the error paths
	// through sequencer as other tests cover this
	mockTx.On("Commit").Return(nil)
	mockTx.On("DequeueLeaves", 50).Return([]trillian.LogLeaf{testLeaf0}, nil)
	mockTx.On("LatestSignedLogRoot").Return(testRoot0, nil)
	mockTx.On("UpdateSequencedLeaves", []trillian.LogLeaf{testLeaf0}).Return(nil)
	mockTx.On("SetMerkleNodes", int64(1), updatedNodes0).Return(nil)
	mockTx.On("StoreSignedLogRoot", updatedRoot).Return(nil)
	mockStorage.On("Begin").Return(mockTx, nil)

	mockSigner := new(crypto.MockSigner)
	mockSigner.On("Sign", mock.MatchedBy(
		func(other io.Reader) bool {
			return true
		}), []byte{0x13, 0xa6, 0xf3, 0xcb, 0xa2, 0x82, 0x52, 0xfc, 0x5a, 0x98, 0xfe, 0x81, 0x7c, 0xb7, 0xaf, 0x68, 0x1f, 0x83, 0x30, 0xcf, 0x80, 0x71, 0x1e, 0x9e, 0x16, 0xf6, 0x1e, 0x55, 0xcf, 0x78, 0xa, 0xb9}, hasher).Return([]byte("signed"), nil)
	mockKeyManager.On("Signer").Return(*mockSigner, nil)

	sm := NewSequencerManager(mockKeyManager)

	sm.ExecutePass([]trillian.LogID{logID}, createTestContext(mockStorageProviderForSequencer(mockStorage)))

	mockStorage.AssertExpectations(t)
	mockTx.AssertExpectations(t)
	mockSigner.AssertExpectations(t)
	mockKeyManager.AssertExpectations(t)
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

	return LogOperationManagerContext{done: done, storageProvider: sp, batchSize: 50, sleepBetweenRuns: time.Second, oneShot: true, timeSource: fakeTimeSource}
}
