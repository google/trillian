package log

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/util"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/assert"
)

// These can be shared between tests as they're never modified
var testLeaf16Hash = trillian.Hash{0, 1, 2, 3, 4, 5}
var testLeaf16 = trillian.LogLeaf{Leaf: trillian.Leaf{LeafHash: testLeaf16Hash, LeafValue: nil, ExtraData: nil}, SequenceNumber: 16}
var testRoot16 = trillian.SignedLogRoot{TreeSize: 16, TreeRevision: 5}

// These will be accepted in either order because of custom sorting in the mock
var updatedNodes []storage.Node = []storage.Node{
	storage.Node{NodeID: storage.NodeID{Path: []uint8{0x10}, PrefixLenBits: 0, PathLenBits: 6},
		Hash: trillian.Hash{0x0, 0x1, 0x2, 0x3, 0x4, 0x5}, NodeRevision: 6},
	storage.Node{
		NodeID:       storage.NodeID{Path: []uint8{0x0}, PrefixLenBits: 5, PathLenBits: 6},
		Hash:         trillian.Hash{0xbb, 0x49, 0x71, 0xbc, 0x2a, 0x37, 0x93, 0x67, 0xfb, 0x75, 0xa9, 0xf4, 0x5b, 0x67, 0xf, 0xb0, 0x97, 0xb2, 0x1e, 0x81, 0x1d, 0x58, 0xd1, 0x3a, 0xbb, 0x71, 0x7e, 0x28, 0x51, 0x17, 0xc3, 0x7c},
		NodeRevision: 6},
}

var fakeTimeForTest = fakeTime()
var expectedSignedRoot = trillian.SignedLogRoot{
	RootHash:       trillian.Hash{0xbb, 0x49, 0x71, 0xbc, 0x2a, 0x37, 0x93, 0x67, 0xfb, 0x75, 0xa9, 0xf4, 0x5b, 0x67, 0xf, 0xb0, 0x97, 0xb2, 0x1e, 0x81, 0x1d, 0x58, 0xd1, 0x3a, 0xbb, 0x71, 0x7e, 0x28, 0x51, 0x17, 0xc3, 0x7c},
	TimestampNanos: fakeTimeForTest.UnixNano(),
	TreeRevision:   6,
	TreeSize:       17,
	LogId:          []uint8(nil),
	Signature:      &trillian.DigitallySigned{},
}

// Any tests relying on time should use this fixed value
const fakeTimeStr string = "2016-05-25T10:55:05Z"

// testParameters bundles up values needed for setting mock expectations in tests
type testParameters struct {
	fakeTime time.Time

	beginFails   bool
	dequeueLimit int

	shouldCommit   bool
	commitFails    bool
	commitError    error
	shouldRollback bool

	dequeuedLeaves []trillian.LogLeaf
	dequeuedError  error

	latestSignedRootError error
	latestSignedRoot      *trillian.SignedLogRoot

	updatedLeaves      *[]trillian.LogLeaf
	updatedLeavesError error

	merkleNodesSet             *[]storage.Node
	merkleNodesSetTreeRevision int64
	merkleNodesSetError        error

	storeSignedRoot      *trillian.SignedLogRoot
	storeSignedRootError error
}

// Tests get their own mock context so they can be run in parallel safely
type testContext struct {
	mockTx      *storage.MockLogTX
	mockStorage *storage.MockLogStorage
	sequencer   *Sequencer
}

// This gets modified so tests need their own copies
func getLeaf42() trillian.LogLeaf {
	testLeaf42Hash := trillian.Hash{0, 1, 2, 3, 4, 5}
	return trillian.LogLeaf{Leaf: trillian.Leaf{LeafHash: testLeaf42Hash, LeafValue: nil, ExtraData: nil},
		SequenceNumber: 42}
}

func fakeTime() time.Time {
	fakeTimeForTest, err := time.Parse(time.RFC3339, fakeTimeStr)

	if err != nil {
		panic(fmt.Sprintf("Test has an invalid fake time: %s", err))
	}

	return fakeTimeForTest
}

func createTestContext(params testParameters) testContext {
	mockStorage := new(storage.MockLogStorage)
	mockTx := new(storage.MockLogTX)

	if params.beginFails {
		mockStorage.On("Begin").Return(mockTx, errors.New("TX"))
	} else {
		mockStorage.On("Begin").Return(mockTx, nil)
	}

	if params.shouldCommit {
		if !params.commitFails {
			mockTx.On("Commit").Return(nil)
		} else {
			mockTx.On("Commit").Return(params.commitError)
		}
	}

	if params.shouldRollback {
		mockTx.On("Rollback").Return(nil)
	}

	mockTx.On("DequeueLeaves", params.dequeueLimit).Return(params.dequeuedLeaves, params.dequeuedError)

	if params.latestSignedRoot != nil {
		mockTx.On("LatestSignedLogRoot").Return(*params.latestSignedRoot, params.latestSignedRootError)
	}

	if params.updatedLeaves != nil {
		mockTx.On("UpdateSequencedLeaves", *params.updatedLeaves).Return(params.updatedLeavesError)
	}

	if params.merkleNodesSet != nil {
		mockTx.On("SetMerkleNodes", params.merkleNodesSetTreeRevision, *params.merkleNodesSet).Return(params.merkleNodesSetError)
	}

	if params.storeSignedRoot != nil {
		mockTx.On("StoreSignedLogRoot", mock.MatchedBy(
			func(other trillian.SignedLogRoot) bool {
				return proto.Equal(params.storeSignedRoot, &other)
			})).Return(params.storeSignedRootError)
	} else {
		// At the moment if we're going to fail the operation we accept any root
		mockTx.On("StoreSignedLogRoot", mock.MatchedBy(
			func(other trillian.SignedLogRoot) bool {
				return true
			})).Return(params.storeSignedRootError)
	}

	sequencer := NewSequencer(trillian.NewSHA256(), util.FakeTimeSource{fakeTimeForTest}, mockStorage)

	return testContext{mockTx, mockStorage, sequencer}
}

func ensureErrorContains(t *testing.T, err error, s string) {
	if err == nil {
		t.Fatalf("%s operation unexpectedly succeeded", s)
	}

	if !strings.Contains(err.Error(), s) {
		t.Errorf("Got the wrong type of error: %v", err)
	}
}

// Tests for sequencer. Currently relies on having a database set up. This might change in future
// as it would be better if it was not tied to a specific storage mechanism.

func TestBeginTXFails(t *testing.T) {
	params := testParameters{beginFails: true}
	c := createTestContext(params)

	leaves, err := c.sequencer.SequenceBatch(1)
	assert.Zero(t, leaves, "Unexpectedly sequenced leaves on error")
	ensureErrorContains(t, err, "TX")

	c.mockStorage.AssertExpectations(t)
}

func TestSequenceWithNothingQueued(t *testing.T) {
	params := testParameters{dequeueLimit: 1, shouldCommit: true, dequeuedLeaves: []trillian.LogLeaf{}}
	c := createTestContext(params)

	leaves, err := c.sequencer.SequenceBatch(1)
	assert.Zero(t, leaves, "Unexpectedly sequenced leaves on error")

	if err != nil {
		t.Errorf("Expected nil return with no work pending in queue")
	}

	c.mockStorage.AssertExpectations(t)
}

func TestDequeueError(t *testing.T) {
	params := testParameters{dequeueLimit: 1, shouldRollback: true, dequeuedError: errors.New("dequeue")}
	c := createTestContext(params)

	leaves, err := c.sequencer.SequenceBatch(1)
	ensureErrorContains(t, err, "dequeue")
	assert.Zero(t, leaves, "Unexpectedly sequenced leaves on error")

	c.mockStorage.AssertExpectations(t)
}

func TestLatestRootError(t *testing.T) {
	leaves := []trillian.LogLeaf{getLeaf42()}
	params := testParameters{dequeueLimit: 1, shouldRollback: true, dequeuedLeaves: leaves,
		latestSignedRoot: &testRoot16, latestSignedRootError: errors.New("root")}
	c := createTestContext(params)

	leafCount, err := c.sequencer.SequenceBatch(1)
	assert.Zero(t, leafCount, "Unexpectedly sequenced leaves on error")
	ensureErrorContains(t, err, "root")

	c.mockStorage.AssertExpectations(t)
}

func TestUpdateSequencedLeavesError(t *testing.T) {
	leaves := []trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []trillian.LogLeaf{testLeaf16}
	params := testParameters{dequeueLimit: 1, shouldRollback: true, dequeuedLeaves: leaves,
		latestSignedRoot: &testRoot16, updatedLeaves: &updatedLeaves,
		updatedLeavesError: errors.New("unsequenced")}
	c := createTestContext(params)

	leafCount, err := c.sequencer.SequenceBatch(1)
	assert.Zero(t, leafCount, "Unexpectedly sequenced leaves on error")
	ensureErrorContains(t, err, "unsequenced")

	c.mockStorage.AssertExpectations(t)
}

func TestSetMerkleNodesError(t *testing.T) {
	leaves := []trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []trillian.LogLeaf{testLeaf16}
	params := testParameters{dequeueLimit: 1, shouldRollback: true, dequeuedLeaves: leaves,
		latestSignedRoot: &testRoot16, updatedLeaves: &updatedLeaves, merkleNodesSet: &updatedNodes,
		merkleNodesSetTreeRevision: 6, merkleNodesSetError: errors.New("setmerklenodes")}
	c := createTestContext(params)

	leafCount, err := c.sequencer.SequenceBatch(1)
	assert.Zero(t, leafCount, "Unexpectedly sequenced leaves on error")
	ensureErrorContains(t, err, "setmerklenodes")

	c.mockStorage.AssertExpectations(t)
}

func TestStoreSignedRootError(t *testing.T) {
	leaves := []trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []trillian.LogLeaf{testLeaf16}
	params := testParameters{dequeueLimit: 1, shouldRollback: true, dequeuedLeaves: leaves,
		latestSignedRoot: &testRoot16, updatedLeaves: &updatedLeaves, merkleNodesSet: &updatedNodes,
		merkleNodesSetTreeRevision: 6, storeSignedRoot: nil,
		storeSignedRootError: errors.New("storesignedroot")}
	c := createTestContext(params)

	leafCount, err := c.sequencer.SequenceBatch(1)
	assert.Zero(t, leafCount, "Unexpectedly sequenced leaves on error")
	ensureErrorContains(t, err, "storesignedroot")

	c.mockStorage.AssertExpectations(t)
}

func TestCommitFails(t *testing.T) {
	leaves := []trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []trillian.LogLeaf{testLeaf16}

	params := testParameters{dequeueLimit: 1, shouldCommit: true, commitFails: true,
		commitError: errors.New("commit"), dequeuedLeaves: leaves,
		latestSignedRoot: &testRoot16, updatedLeaves: &updatedLeaves, merkleNodesSet: &updatedNodes,
		merkleNodesSetTreeRevision: 6, storeSignedRoot: nil}
	c := createTestContext(params)

	leafCount, err := c.sequencer.SequenceBatch(1)
	assert.Zero(t, leafCount, "Unexpectedly sequenced leaves on error")
	ensureErrorContains(t, err, "commit")

	c.mockStorage.AssertExpectations(t)
}

// TODO: We used a perfect tree size so this isn't testing code that loads the compact merkle
// tree. This will be done later as it's planned to refactor it anyway.
func TestSequenceBatch(t *testing.T) {
	leaves := []trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []trillian.LogLeaf{testLeaf16}
	params := testParameters{dequeueLimit: 1, shouldCommit: true,
		dequeuedLeaves: leaves, latestSignedRoot: &testRoot16,
		updatedLeaves: &updatedLeaves, merkleNodesSet: &updatedNodes, merkleNodesSetTreeRevision: 6,
		storeSignedRoot: &expectedSignedRoot}
	c := createTestContext(params)

	c.sequencer.SequenceBatch(1)

	c.mockStorage.AssertExpectations(t)
}
