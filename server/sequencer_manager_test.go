package server

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/trillian/storage"
	"github.com/google/trillian"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian/util"
)

// Arbitrary time for use in tests
var fakeTime = time.Date(2016, 6, 28, 13, 40, 12, 45, time.UTC)
var fakeTimeSource = util.FakeTimeSource{fakeTime}

// We use a size zero tree for testing, merkle tree state restore is tested elsewhere
var logID1 = trillian.LogID{TreeID: 1, LogID: []byte("testroot")}
var testLeaf0Hash = trillian.Hash{0, 1, 2, 3, 4, 5}
var testLeaf0 = trillian.LogLeaf{Leaf: trillian.Leaf{LeafHash: testLeaf0Hash, LeafValue: nil, ExtraData: nil}, SequenceNumber: 0}
var testRoot0 = trillian.SignedLogRoot{TreeSize: proto.Int64(0), TreeRevision: proto.Int64(0), LogId: logID1.LogID}
var updatedNodes0 = []storage.Node{storage.Node{NodeID:storage.NodeID{Path:[]uint8{0x0}, PrefixLenBits:0, PathLenBits:6}, Hash:trillian.Hash{0x0, 0x1, 0x2, 0x3, 0x4, 0x5}, NodeRevision:1}}
var updatedRoot = trillian.SignedLogRoot{LogId: logID1.LogID, TimestampNanos:proto.Int64(fakeTime.UnixNano()), RootHash:[]uint8{0x0, 0x1, 0x2, 0x3, 0x4, 0x5}, TreeSize:proto.Int64(1), Signature:&trillian.DigitallySigned{}, TreeRevision:proto.Int64(1)}

func TestSequencerManagerNothingToDo(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)
	mockTx := new(storage.MockLogTX)

	mockStorage.On("Begin").Return(mockTx, nil)
	mockTx.On("GetActiveLogIDs").Return([]trillian.LogID{}, nil)
	mockTx.On("Commit").Return(nil)

	done := make(chan struct {})
	// Arrange for the sequencer to make one pass
	sm := newSequencerManagerForTest(done, mockStorageProviderForSequencer(mockStorage), 50, time.Millisecond, time.Millisecond, 1, fakeTimeSource)

	sm.SequencerLoop()

	mockStorage.AssertExpectations(t)
}

func TestSequencerManagerSingleLogNoLeaves(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)
	mockTx := new(storage.MockLogTX)
	logID := trillian.LogID{TreeID:1, LogID: []byte("Test")}

	mockStorage.On("Begin").Return(mockTx, nil)
	mockTx.On("GetActiveLogIDs").Return([]trillian.LogID{logID}, nil)
	mockTx.On("Commit").Return(nil)
	mockTx.On("DequeueLeaves", 50).Return([]trillian.LogLeaf{}, nil)

	done := make(chan struct {})
	// Arrange for the sequencer to make one pass
	sm := newSequencerManagerForTest(done, mockStorageProviderForSequencer(mockStorage), 50, time.Millisecond, time.Millisecond, 1, fakeTimeSource)

	sm.SequencerLoop()

	mockStorage.AssertExpectations(t)
}

func TestSequencerManagerSingleLogOneLeaf(t *testing.T) {
	mockStorage := new(storage.MockLogStorage)
	mockTx := new(storage.MockLogTX)
	logID := trillian.LogID{TreeID:1, LogID: []byte("Test")}

	// Set up enough mockery to be able to sequence. We don't test all the error paths
	// through sequencer as other tests cover this
	mockStorage.On("Begin").Return(mockTx, nil)
	mockTx.On("GetActiveLogIDs").Return([]trillian.LogID{logID}, nil)
	mockTx.On("Commit").Return(nil)
	mockTx.On("DequeueLeaves", 50).Return([]trillian.LogLeaf{testLeaf0}, nil)
	mockTx.On("LatestSignedLogRoot").Return(testRoot0, nil)
	mockTx.On("UpdateSequencedLeaves", []trillian.LogLeaf{testLeaf0}).Return(nil)
	mockTx.On("SetMerkleNodes", int64(1), updatedNodes0).Return(nil)
	mockTx.On("StoreSignedLogRoot", updatedRoot).Return(nil)

	done := make(chan struct {})
	// Arrange for the sequencer to make one pass
	sm := newSequencerManagerForTest(done, mockStorageProviderForSequencer(mockStorage), 50, time.Millisecond, time.Millisecond, 1, fakeTimeSource)

	sm.SequencerLoop()

	mockStorage.AssertExpectations(t)
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
