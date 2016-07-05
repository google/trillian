package log

import (
	"errors"
	"fmt"
	"io"
	"testing"
	"time"


	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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
	Signature:      &trillian.DigitallySigned{Signature: []byte("signed")},
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

	setupSigner bool
	keyManagerError error
	dataToSign []byte
	signingResult []byte
	signingError error
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

	mockKeyManager := new(crypto.MockKeyManager)
	hasher := merkle.NewRFC6962TreeHasher(trillian.NewSHA256())

	if params.setupSigner {
		mockSigner := new(crypto.MockSigner)
		mockSigner.On("Sign", mock.MatchedBy(
			func(other io.Reader) bool {
				return true
			}), params.dataToSign, hasher.Hasher).Return(params.signingResult, params.signingError)
		mockKeyManager.On("Signer").Return(*mockSigner, params.keyManagerError)
	}

	sequencer := NewSequencer(hasher, util.FakeTimeSource{fakeTimeForTest}, mockStorage, mockKeyManager)

	return testContext{mockTx, mockStorage, sequencer}
}

// Tests for sequencer. Currently relies on having a database set up. This might change in future
// as it would be better if it was not tied to a specific storage mechanism.

func TestBeginTXFails(t *testing.T) {
	params := testParameters{beginFails: true}
	c := createTestContext(params)

	leaves, err := c.sequencer.SequenceBatch(1)
	assert.Zero(t, leaves, "Unexpectedly sequenced leaves on error")
	testonly.EnsureErrorContains(t, err, "TX")

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
	testonly.EnsureErrorContains(t, err, "dequeue")
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
	testonly.EnsureErrorContains(t, err, "root")

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
	testonly.EnsureErrorContains(t, err, "unsequenced")

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
	testonly.EnsureErrorContains(t, err, "setmerklenodes")

	c.mockStorage.AssertExpectations(t)
}

func TestStoreSignedRootError(t *testing.T) {
	leaves := []trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []trillian.LogLeaf{testLeaf16}
	params := testParameters{dequeueLimit: 1, shouldRollback: true, dequeuedLeaves: leaves,
		latestSignedRoot: &testRoot16, updatedLeaves: &updatedLeaves, merkleNodesSet: &updatedNodes,
		merkleNodesSetTreeRevision: 6, storeSignedRoot: nil,
		storeSignedRootError: errors.New("storesignedroot"), setupSigner: true,
		dataToSign: []byte{0xf2, 0xcc, 0xb0, 0x34, 0x2b, 0x0, 0x9d, 0x14, 0xa5, 0xa2, 0xb9, 0xa2, 0xd5, 0xb0, 0x15, 0xf5, 0x1c, 0xfc, 0x56, 0x5a, 0x0, 0xef, 0x3f, 0x8a, 0x3c, 0x45, 0xba, 0xd, 0x54, 0xd9, 0xba, 0x81},
		signingResult: []byte("signed")}
	c := createTestContext(params)

	leafCount, err := c.sequencer.SequenceBatch(1)
	assert.Zero(t, leafCount, "Unexpectedly sequenced leaves on error")
	testonly.EnsureErrorContains(t, err, "storesignedroot")

	c.mockStorage.AssertExpectations(t)
}

func TestStoreSignedRootKeyManagerFails(t *testing.T) {
	leaves := []trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []trillian.LogLeaf{testLeaf16}
	params := testParameters{dequeueLimit: 1, shouldRollback: true, dequeuedLeaves: leaves,
		latestSignedRoot: &testRoot16, updatedLeaves: &updatedLeaves, merkleNodesSet: &updatedNodes,
		merkleNodesSetTreeRevision: 6, storeSignedRoot: nil,
		storeSignedRootError: errors.New("storesignedroot"), setupSigner: true,
		keyManagerError: errors.New("keymanagerfailed")}
	c := createTestContext(params)

	leafCount, err := c.sequencer.SequenceBatch(1)
	assert.Zero(t, leafCount, "Unexpectedly sequenced leaves on error")
	ensureErrorContains(t, err, "keymanagerfailed")

	c.mockStorage.AssertExpectations(t)
}

func TestStoreSignedRootSignerFails(t *testing.T) {
	leaves := []trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []trillian.LogLeaf{testLeaf16}
	params := testParameters{dequeueLimit: 1, shouldRollback: true, dequeuedLeaves: leaves,
		latestSignedRoot: &testRoot16, updatedLeaves: &updatedLeaves, merkleNodesSet: &updatedNodes,
		merkleNodesSetTreeRevision: 6, storeSignedRoot: nil,
		storeSignedRootError: errors.New("storesignedroot"), setupSigner: true,
		dataToSign: []byte{0xf2, 0xcc, 0xb0, 0x34, 0x2b, 0x0, 0x9d, 0x14, 0xa5, 0xa2, 0xb9, 0xa2, 0xd5, 0xb0, 0x15, 0xf5, 0x1c, 0xfc, 0x56, 0x5a, 0x0, 0xef, 0x3f, 0x8a, 0x3c, 0x45, 0xba, 0xd, 0x54, 0xd9, 0xba, 0x81},
		signingError: errors.New("signerfailed")}
	c := createTestContext(params)

	leafCount, err := c.sequencer.SequenceBatch(1)
	assert.Zero(t, leafCount, "Unexpectedly sequenced leaves on error")
	ensureErrorContains(t, err, "signerfailed")

	c.mockStorage.AssertExpectations(t)
}

func TestCommitFails(t *testing.T) {
	leaves := []trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []trillian.LogLeaf{testLeaf16}

	params := testParameters{dequeueLimit: 1, shouldCommit: true, commitFails: true,
		commitError: errors.New("commit"), dequeuedLeaves: leaves,
		latestSignedRoot: &testRoot16, updatedLeaves: &updatedLeaves, merkleNodesSet: &updatedNodes,
		merkleNodesSetTreeRevision: 6, storeSignedRoot: nil, setupSigner: true,
		dataToSign: []byte{0xf2, 0xcc, 0xb0, 0x34, 0x2b, 0x0, 0x9d, 0x14, 0xa5, 0xa2, 0xb9, 0xa2, 0xd5, 0xb0, 0x15, 0xf5, 0x1c, 0xfc, 0x56, 0x5a, 0x0, 0xef, 0x3f, 0x8a, 0x3c, 0x45, 0xba, 0xd, 0x54, 0xd9, 0xba, 0x81},
		signingResult: []byte("signed")}
	c := createTestContext(params)

	leafCount, err := c.sequencer.SequenceBatch(1)
	assert.Zero(t, leafCount, "Unexpectedly sequenced leaves on error")
	testonly.EnsureErrorContains(t, err, "commit")

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
		storeSignedRoot: &expectedSignedRoot, setupSigner: true,
		dataToSign: []byte{0xf2, 0xcc, 0xb0, 0x34, 0x2b, 0x0, 0x9d, 0x14, 0xa5, 0xa2, 0xb9, 0xa2, 0xd5, 0xb0, 0x15, 0xf5, 0x1c, 0xfc, 0x56, 0x5a, 0x0, 0xef, 0x3f, 0x8a, 0x3c, 0x45, 0xba, 0xd, 0x54, 0xd9, 0xba, 0x81},
		signingResult: []byte("signed")}
	c := createTestContext(params)

	c.sequencer.SequenceBatch(1)

	c.mockStorage.AssertExpectations(t)
}
