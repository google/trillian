package log

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/util"
	"github.com/stretchr/testify/mock"
)

// Long duration to prevent root signing kicking in for tests where we're only testing
// sequencing
const tenYears time.Duration = time.Hour * 24 * 365 * 10

// Func that says root never expires, so we can be sure tests will only sign / sequence when we
// expect them to
func rootNeverExpiresFunc(trillian.SignedLogRoot) bool {
	return false
}

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

var expectedSignedRoot16 = trillian.SignedLogRoot{
	// The root hash would not normally be nil but we've got a perfect tree size of 16 and
	// aren't testing merkle state loading so a nil hash gets copied in from our test data
	// TODO(Martin2112): An extended test that checks the root hash
	TimestampNanos: fakeTimeForTest.UnixNano(),
	TreeRevision:   6,
	TreeSize:       16,
	LogId:          []uint8(nil),
	Signature:      &trillian.DigitallySigned{Signature: []byte("signed")},
}

// expectedSignedRoot0 is a root for an empty tree
var expectedSignedRoot0 = trillian.SignedLogRoot{
	RootHash:       []byte{0xe3, 0xb0, 0xc4, 0x42, 0x98, 0xfc, 0x1c, 0x14, 0x9a, 0xfb, 0xf4, 0xc8, 0x99, 0x6f, 0xb9, 0x24, 0x27, 0xae, 0x41, 0xe4, 0x64, 0x9b, 0x93, 0x4c, 0xa4, 0x95, 0x99, 0x1b, 0x78, 0x52, 0xb8, 0x55},
	TimestampNanos: fakeTimeForTest.UnixNano(),
	TreeRevision:   1,
	TreeSize:       0,
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

	skipDequeue    bool
	dequeuedLeaves []trillian.LogLeaf
	dequeuedError  error

	latestSignedRootError error
	latestSignedRoot      *trillian.SignedLogRoot

	updatedLeaves      *[]trillian.LogLeaf
	updatedLeavesError error

	merkleNodesSet             *[]storage.Node
	merkleNodesSetTreeRevision int64
	merkleNodesSetError        error

	skipStoreSignedRoot  bool
	storeSignedRoot      *trillian.SignedLogRoot
	storeSignedRootError error

	setupSigner     bool
	keyManagerError error
	dataToSign      []byte
	signingResult   []byte
	signingError    error
}

// Tests get their own mock context so they can be run in parallel safely
type testContext struct {
	mockTx         *storage.MockLogTX
	mockStorage    *storage.MockLogStorage
	mockKeyManager *crypto.MockKeyManager
	sequencer      *Sequencer
}

func (c testContext) assertExpectations(t *testing.T) {
	c.mockStorage.AssertExpectations(t)
	c.mockTx.AssertExpectations(t)
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

func createTestContext(ctrl *gomock.Controller, params testParameters) testContext {
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

	if !params.skipDequeue {
		mockTx.On("DequeueLeaves", params.dequeueLimit).Return(params.dequeuedLeaves, params.dequeuedError)
	}

	if params.latestSignedRoot != nil {
		mockTx.On("LatestSignedLogRoot").Return(*params.latestSignedRoot, params.latestSignedRootError)
	}

	if params.updatedLeaves != nil {
		mockTx.On("UpdateSequencedLeaves", *params.updatedLeaves).Return(params.updatedLeavesError)
	}

	if params.merkleNodesSet != nil {
		mockTx.On("SetMerkleNodes", params.merkleNodesSetTreeRevision, *params.merkleNodesSet).Return(params.merkleNodesSetError)
	}

	if !params.skipStoreSignedRoot {
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
	}

	mockKeyManager := crypto.NewMockKeyManager(ctrl)
	hasher := merkle.NewRFC6962TreeHasher(trillian.NewSHA256())

	if params.setupSigner {
		mockSigner := crypto.NewMockSigner(ctrl)
		mockSigner.EXPECT().Sign(gomock.Any(), params.dataToSign, hasher.Hasher).AnyTimes().Return(params.signingResult, params.signingError)
		mockKeyManager.EXPECT().Signer().AnyTimes().Return(mockSigner, params.keyManagerError)
	}

	sequencer := NewSequencer(hasher, util.FakeTimeSource{fakeTimeForTest}, mockStorage, mockKeyManager)

	return testContext{mockTx: mockTx, mockStorage: mockStorage, mockKeyManager: mockKeyManager, sequencer: sequencer}
}

// Tests for sequencer. Currently relies on having a database set up. This might change in future
// as it would be better if it was not tied to a specific storage mechanism.

func TestBeginTXFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	params := testParameters{beginFails: true, skipDequeue: true, skipStoreSignedRoot: true}
	c := createTestContext(ctrl, params)

	leaves, err := c.sequencer.SequenceBatch(1, rootNeverExpiresFunc)
	if leaves != 0 {
		t.Fatalf("Unexpectedly sequenced %d leaves on error", leaves)
	}
	testonly.EnsureErrorContains(t, err, "TX")

	c.assertExpectations(t)
}

func TestSequenceWithNothingQueued(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	params := testParameters{dequeueLimit: 1, shouldCommit: true, latestSignedRoot: &testRoot16, dequeuedLeaves: []trillian.LogLeaf{}, skipStoreSignedRoot: true}

	c := createTestContext(ctrl, params)

	leaves, err := c.sequencer.SequenceBatch(1, rootNeverExpiresFunc)
	if leaves != 0 {
		t.Fatalf("Unexpectedly sequenced %d leaves on error", leaves)
	}

	if err != nil {
		t.Errorf("Expected nil return with no work pending in queue")
	}

	c.assertExpectations(t)
}

func TestDequeueError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	params := testParameters{dequeueLimit: 1, shouldRollback: true, dequeuedError: errors.New("dequeue")}
	c := createTestContext(ctrl, params)

	leafCount, err := c.sequencer.SequenceBatch(1, rootNeverExpiresFunc)
	testonly.EnsureErrorContains(t, err, "dequeue")
	if leafCount != 0 {
		t.Fatalf("Unexpectedly sequenced %d leaves on error", leafCount)
	}

	c.mockStorage.AssertExpectations(t)
}

func TestLatestRootError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	leaves := []trillian.LogLeaf{getLeaf42()}
	params := testParameters{dequeueLimit: 1, shouldRollback: true, dequeuedLeaves: leaves,
		latestSignedRoot: &testRoot16, latestSignedRootError: errors.New("root")}
	c := createTestContext(ctrl, params)

	leafCount, err := c.sequencer.SequenceBatch(1, rootNeverExpiresFunc)
	if leafCount != 0 {
		t.Fatalf("Unexpectedly sequenced %d leaves on error", leafCount)
	}
	testonly.EnsureErrorContains(t, err, "root")

	c.mockStorage.AssertExpectations(t)
}

func TestUpdateSequencedLeavesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	leaves := []trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []trillian.LogLeaf{testLeaf16}
	params := testParameters{dequeueLimit: 1, shouldRollback: true, dequeuedLeaves: leaves,
		latestSignedRoot: &testRoot16, updatedLeaves: &updatedLeaves,
		updatedLeavesError: errors.New("unsequenced")}
	c := createTestContext(ctrl, params)

	leafCount, err := c.sequencer.SequenceBatch(1, rootNeverExpiresFunc)
	if leafCount != 0 {
		t.Fatalf("Unexpectedly sequenced %d leaves on error", leafCount)
	}
	testonly.EnsureErrorContains(t, err, "unsequenced")

	c.mockStorage.AssertExpectations(t)
}

func TestSetMerkleNodesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	leaves := []trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []trillian.LogLeaf{testLeaf16}
	params := testParameters{dequeueLimit: 1, shouldRollback: true, dequeuedLeaves: leaves,
		latestSignedRoot: &testRoot16, updatedLeaves: &updatedLeaves, merkleNodesSet: &updatedNodes,
		merkleNodesSetTreeRevision: 6, merkleNodesSetError: errors.New("setmerklenodes")}
	c := createTestContext(ctrl, params)

	leafCount, err := c.sequencer.SequenceBatch(1, rootNeverExpiresFunc)
	if leafCount != 0 {
		t.Fatalf("Unexpectedly sequenced %d leaves on error", leafCount)
	}
	testonly.EnsureErrorContains(t, err, "setmerklenodes")

	c.mockStorage.AssertExpectations(t)
}

func TestStoreSignedRootError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	leaves := []trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []trillian.LogLeaf{testLeaf16}
	params := testParameters{dequeueLimit: 1, shouldRollback: true, dequeuedLeaves: leaves,
		latestSignedRoot: &testRoot16, updatedLeaves: &updatedLeaves, merkleNodesSet: &updatedNodes,
		merkleNodesSetTreeRevision: 6, storeSignedRoot: nil,
		storeSignedRootError: errors.New("storesignedroot"), setupSigner: true,
		dataToSign:    []byte{0x4f, 0x21, 0x7d, 0x10, 0xe2, 0x6, 0x9f, 0x10, 0x4d, 0x7e, 0x42, 0x75, 0x24, 0x3b, 0xb3, 0x5b, 0x63, 0xa6, 0x7, 0x8d, 0x6c, 0x97, 0x23, 0x4, 0x8, 0x5e, 0x3b, 0xe2, 0xc4, 0xb8, 0x7a, 0xa2},
		signingResult: []byte("signed")}
	c := createTestContext(ctrl, params)

	leafCount, err := c.sequencer.SequenceBatch(1, rootNeverExpiresFunc)
	if leafCount != 0 {
		t.Fatalf("Unexpectedly sequenced %d leaves on error", leafCount)
	}
	testonly.EnsureErrorContains(t, err, "storesignedroot")

	c.mockStorage.AssertExpectations(t)
}

func TestStoreSignedRootKeyManagerFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	leaves := []trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []trillian.LogLeaf{testLeaf16}
	params := testParameters{dequeueLimit: 1, shouldRollback: true, dequeuedLeaves: leaves,
		latestSignedRoot: &testRoot16, updatedLeaves: &updatedLeaves, merkleNodesSet: &updatedNodes,
		merkleNodesSetTreeRevision: 6, storeSignedRoot: nil,
		setupSigner:     true,
		keyManagerError: errors.New("keymanagerfailed")}
	c := createTestContext(ctrl, params)

	leafCount, err := c.sequencer.SequenceBatch(1, rootNeverExpiresFunc)
	if leafCount != 0 {
		t.Fatalf("Unexpectedly sequenced %d leaves on error", leafCount)
	}
	testonly.EnsureErrorContains(t, err, "keymanagerfailed")

	c.mockStorage.AssertExpectations(t)
}

func TestStoreSignedRootSignerFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	leaves := []trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []trillian.LogLeaf{testLeaf16}
	params := testParameters{dequeueLimit: 1, shouldRollback: true, dequeuedLeaves: leaves,
		latestSignedRoot: &testRoot16, updatedLeaves: &updatedLeaves, merkleNodesSet: &updatedNodes,
		merkleNodesSetTreeRevision: 6, storeSignedRoot: nil,
		setupSigner:  true,
		dataToSign:   []byte{0x4f, 0x21, 0x7d, 0x10, 0xe2, 0x6, 0x9f, 0x10, 0x4d, 0x7e, 0x42, 0x75, 0x24, 0x3b, 0xb3, 0x5b, 0x63, 0xa6, 0x7, 0x8d, 0x6c, 0x97, 0x23, 0x4, 0x8, 0x5e, 0x3b, 0xe2, 0xc4, 0xb8, 0x7a, 0xa2},
		signingError: errors.New("signerfailed")}
	c := createTestContext(ctrl, params)

	leafCount, err := c.sequencer.SequenceBatch(1, rootNeverExpiresFunc)
	if leafCount != 0 {
		t.Fatalf("Unexpectedly sequenced %d leaves on error", leafCount)
	}
	testonly.EnsureErrorContains(t, err, "signerfailed")

	c.mockStorage.AssertExpectations(t)
}

func TestCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	leaves := []trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []trillian.LogLeaf{testLeaf16}

	params := testParameters{dequeueLimit: 1, shouldCommit: true, commitFails: true,
		commitError: errors.New("commit"), dequeuedLeaves: leaves,
		latestSignedRoot: &testRoot16, updatedLeaves: &updatedLeaves, merkleNodesSet: &updatedNodes,
		merkleNodesSetTreeRevision: 6, storeSignedRoot: nil, setupSigner: true,
		dataToSign:    []byte{0x4f, 0x21, 0x7d, 0x10, 0xe2, 0x6, 0x9f, 0x10, 0x4d, 0x7e, 0x42, 0x75, 0x24, 0x3b, 0xb3, 0x5b, 0x63, 0xa6, 0x7, 0x8d, 0x6c, 0x97, 0x23, 0x4, 0x8, 0x5e, 0x3b, 0xe2, 0xc4, 0xb8, 0x7a, 0xa2},
		signingResult: []byte("signed")}
	c := createTestContext(ctrl, params)

	leafCount, err := c.sequencer.SequenceBatch(1, rootNeverExpiresFunc)
	if leafCount != 0 {
		t.Fatalf("Unexpectedly sequenced %d leaves on error", leafCount)
	}
	testonly.EnsureErrorContains(t, err, "commit")

	c.mockStorage.AssertExpectations(t)
}

// TODO: We used a perfect tree size so this isn't testing code that loads the compact merkle
// tree. This will be done later as it's planned to refactor it anyway.
func TestSequenceBatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	leaves := []trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []trillian.LogLeaf{testLeaf16}
	params := testParameters{dequeueLimit: 1, shouldCommit: true,
		dequeuedLeaves: leaves, latestSignedRoot: &testRoot16,
		updatedLeaves: &updatedLeaves, merkleNodesSet: &updatedNodes, merkleNodesSetTreeRevision: 6,
		storeSignedRoot: &expectedSignedRoot, setupSigner: true,
		dataToSign:    []byte{0x4f, 0x21, 0x7d, 0x10, 0xe2, 0x6, 0x9f, 0x10, 0x4d, 0x7e, 0x42, 0x75, 0x24, 0x3b, 0xb3, 0x5b, 0x63, 0xa6, 0x7, 0x8d, 0x6c, 0x97, 0x23, 0x4, 0x8, 0x5e, 0x3b, 0xe2, 0xc4, 0xb8, 0x7a, 0xa2},
		signingResult: []byte("signed")}
	c := createTestContext(ctrl, params)

	leafCount, err := c.sequencer.SequenceBatch(1, rootNeverExpiresFunc)
	if err != nil {
		t.Fatalf("Expected sequencing to succeed, but got err: %v", err)
	}
	if got, want := leafCount, 1; got != want {
		t.Fatalf("Sequenced %d leaf, expected %d", got, want)
	}

	c.mockStorage.AssertExpectations(t)
}

func TestSignBeginTxFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	params := testParameters{beginFails: true}
	c := createTestContext(ctrl, params)

	err := c.sequencer.SignRoot()
	testonly.EnsureErrorContains(t, err, "TX")

	c.mockStorage.AssertExpectations(t)
}

func TestSignLatestRootFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	params := testParameters{dequeueLimit: 1, shouldRollback: true,
		latestSignedRoot: &testRoot16, latestSignedRootError: errors.New("root")}
	c := createTestContext(ctrl, params)

	err := c.sequencer.SignRoot()
	testonly.EnsureErrorContains(t, err, "root")

	c.mockStorage.AssertExpectations(t)
}

func TestSignedRootKeyManagerFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	params := testParameters{dequeueLimit: 1, shouldRollback: true,
		latestSignedRoot:           &testRoot16,
		merkleNodesSetTreeRevision: 6, storeSignedRoot: nil,
		setupSigner: true, keyManagerError: errors.New("keymanagerfailed")}
	c := createTestContext(ctrl, params)

	err := c.sequencer.SignRoot()
	testonly.EnsureErrorContains(t, err, "keymanager")

	c.mockStorage.AssertExpectations(t)
}

func TestSignRootSignerFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	params := testParameters{dequeueLimit: 1, shouldRollback: true,
		latestSignedRoot:           &testRoot16,
		merkleNodesSetTreeRevision: 6, storeSignedRoot: nil,
		setupSigner:  true,
		dataToSign:   []byte{0x95, 0x46, 0xdc, 0x25, 0xfb, 0x74, 0x41, 0x4b, 0x50, 0x2e, 0xb0, 0x93, 0x99, 0xbb, 0x5e, 0xf6, 0x57, 0x58, 0xb9, 0x7a, 0x3a, 0x8f, 0xae, 0x35, 0xe1, 0xf6, 0xcd, 0x6c, 0x2a, 0xe6, 0x27, 0xbe},
		signingError: errors.New("signerfailed")}
	c := createTestContext(ctrl, params)

	err := c.sequencer.SignRoot()
	testonly.EnsureErrorContains(t, err, "signer")

	c.mockStorage.AssertExpectations(t)
}

func TestSignRootStoreSignedRootFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	params := testParameters{shouldRollback: true,
		latestSignedRoot:           &testRoot16,
		merkleNodesSetTreeRevision: 6, storeSignedRoot: nil,
		storeSignedRootError: errors.New("storesignedroot"), setupSigner: true,
		dataToSign:    []byte{0x95, 0x46, 0xdc, 0x25, 0xfb, 0x74, 0x41, 0x4b, 0x50, 0x2e, 0xb0, 0x93, 0x99, 0xbb, 0x5e, 0xf6, 0x57, 0x58, 0xb9, 0x7a, 0x3a, 0x8f, 0xae, 0x35, 0xe1, 0xf6, 0xcd, 0x6c, 0x2a, 0xe6, 0x27, 0xbe},
		signingResult: []byte("signed")}
	c := createTestContext(ctrl, params)

	err := c.sequencer.SignRoot()
	testonly.EnsureErrorContains(t, err, "storesignedroot")

	c.mockStorage.AssertExpectations(t)
}

func TestSignRootCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	params := testParameters{shouldCommit: true, commitFails: true,
		commitError:                errors.New("commit"),
		latestSignedRoot:           &testRoot16,
		merkleNodesSetTreeRevision: 6, storeSignedRoot: nil, setupSigner: true,
		dataToSign:    []byte{0x95, 0x46, 0xdc, 0x25, 0xfb, 0x74, 0x41, 0x4b, 0x50, 0x2e, 0xb0, 0x93, 0x99, 0xbb, 0x5e, 0xf6, 0x57, 0x58, 0xb9, 0x7a, 0x3a, 0x8f, 0xae, 0x35, 0xe1, 0xf6, 0xcd, 0x6c, 0x2a, 0xe6, 0x27, 0xbe},
		signingResult: []byte("signed")}
	c := createTestContext(ctrl, params)

	err := c.sequencer.SignRoot()
	testonly.EnsureErrorContains(t, err, "commit")

	c.mockStorage.AssertExpectations(t)
}

func TestSignRoot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	params := testParameters{shouldRollback: true,
		latestSignedRoot:           &testRoot16,
		merkleNodesSetTreeRevision: 6, storeSignedRoot: &expectedSignedRoot16,
		setupSigner:   true,
		dataToSign:    []byte{0x95, 0x46, 0xdc, 0x25, 0xfb, 0x74, 0x41, 0x4b, 0x50, 0x2e, 0xb0, 0x93, 0x99, 0xbb, 0x5e, 0xf6, 0x57, 0x58, 0xb9, 0x7a, 0x3a, 0x8f, 0xae, 0x35, 0xe1, 0xf6, 0xcd, 0x6c, 0x2a, 0xe6, 0x27, 0xbe},
		signingResult: []byte("signed"), shouldCommit: true}
	c := createTestContext(ctrl, params)

	if err := c.sequencer.SignRoot(); err != nil {
		t.Fatalf("Expected signing to succeed, but got err: %v", err)
	}

	c.mockStorage.AssertExpectations(t)
}

func TestSignRootNoExistingRoot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	params := testParameters{shouldRollback: true,
		latestSignedRoot:           &trillian.SignedLogRoot{},
		merkleNodesSetTreeRevision: 6, storeSignedRoot: &expectedSignedRoot0,
		setupSigner:   true,
		dataToSign:    []byte{0xc2, 0xc, 0x1e, 0x33, 0x8, 0xcd, 0x2d, 0x50, 0xbb, 0xf9, 0xf9, 0x1, 0x29, 0xb2, 0xfb, 0xb9, 0x4d, 0x30, 0x27, 0x84, 0xf2, 0xc0, 0x48, 0x5f, 0x46, 0xd4, 0xbe, 0x8a, 0xb8, 0x27, 0x96, 0x22},
		signingResult: []byte("signed"), shouldCommit: true}
	c := createTestContext(ctrl, params)

	if err := c.sequencer.SignRoot(); err != nil {
		t.Fatalf("Expected signing to succeed, but got err: %v", err)
	}

	c.mockStorage.AssertExpectations(t)
}
