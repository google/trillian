package log

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/util"
)

// Func that says root never expires, so we can be sure tests will only sign / sequence when we
// expect them to
func rootNeverExpiresFunc(trillian.SignedLogRoot) bool {
	return false
}

var treeHasher = merkle.NewRFC6962TreeHasher(crypto.NewSHA256())

// These can be shared between tests as they're never modified
var testLeaf16Hash = []byte{0, 1, 2, 3, 4, 5}
var testLeaf16Data = []byte("testdataforleaf")
var testLeaf16 = trillian.LogLeaf{MerkleLeafHash: treeHasher.HashLeaf(testLeaf16Data), LeafValue: testLeaf16Data, ExtraData: nil, LeafIndex: 16}

// RootHash can't be nil because that's how the sequencer currently detects that there was no stored tree head.
var testRoot16 = trillian.SignedLogRoot{TreeSize: 16, TreeRevision: 5, RootHash: []byte{}}

// These will be accepted in either order because of custom sorting in the mock
var updatedNodes []storage.Node = []storage.Node{
	{NodeID: storage.NodeID{Path: []uint8{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10}, PrefixLenBits: 64, PathLenBits: 64},
		Hash: testonly.MustDecodeBase64("L5Iyd7aFOVewxiRm29xD+EU+jvEo4RfufBijKdflWMk="), NodeRevision: 6},
	{
		NodeID: storage.NodeID{Path: []uint8{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, PrefixLenBits: 59, PathLenBits: 64},
		Hash:   testonly.MustDecodeBase64("R57DrKTGuZdjCNXjv6InGrm4rABLOn9yWpdHmYOoLwU="), NodeRevision: 6},
}

var fakeTimeForTest = fakeTime()
var expectedSignedRoot = trillian.SignedLogRoot{
	RootHash:       []byte{71, 158, 195, 172, 164, 198, 185, 151, 99, 8, 213, 227, 191, 162, 39, 26, 185, 184, 172, 0, 75, 58, 127, 114, 90, 151, 71, 153, 131, 168, 47, 5},
	TimestampNanos: fakeTimeForTest.UnixNano(),
	TreeRevision:   6,
	TreeSize:       17,
	LogId:          0,
	Signature:      &trillian.DigitallySigned{Signature: []byte("signed")},
}

var expectedSignedRoot16 = trillian.SignedLogRoot{
	// TODO(Martin2112): An extended test that checks the root hash
	TimestampNanos: fakeTimeForTest.UnixNano(),
	TreeRevision:   6,
	TreeSize:       16,
	RootHash:       testRoot16.RootHash,
	LogId:          0,
	Signature:      &trillian.DigitallySigned{Signature: []byte("signed")},
}

// expectedSignedRoot0 is a root for an empty tree
var expectedSignedRoot0 = trillian.SignedLogRoot{
	RootHash:       []byte{0xe3, 0xb0, 0xc4, 0x42, 0x98, 0xfc, 0x1c, 0x14, 0x9a, 0xfb, 0xf4, 0xc8, 0x99, 0x6f, 0xb9, 0x24, 0x27, 0xae, 0x41, 0xe4, 0x64, 0x9b, 0x93, 0x4c, 0xa4, 0x95, 0x99, 0x1b, 0x78, 0x52, 0xb8, 0x55},
	TimestampNanos: fakeTimeForTest.UnixNano(),
	TreeRevision:   1,
	TreeSize:       0,
	LogId:          0,
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

	merkleNodesSet      *[]storage.Node
	merkleNodesSetError error

	skipStoreSignedRoot  bool
	storeSignedRoot      *trillian.SignedLogRoot
	storeSignedRootError error

	setupSigner     bool
	keyManagerError error
	dataToSign      []byte
	signingResult   []byte
	signingError    error

	writeRevision int64

	overrideDequeueTime *time.Time
}

// Tests get their own mock context so they can be run in parallel safely
type testContext struct {
	mockTx         *storage.MockLogTX
	mockStorage    *storage.MockLogStorage
	mockKeyManager *crypto.MockKeyManager
	sequencer      *Sequencer
}

// This gets modified so tests need their own copies
func getLeaf42() trillian.LogLeaf {
	testLeaf42Hash := []byte{0, 1, 2, 3, 4, 5}
	return trillian.LogLeaf{MerkleLeafHash: testLeaf42Hash, LeafValue: testLeaf16Data, ExtraData: nil, LeafIndex: 42}
}

func fakeTime() time.Time {
	fakeTimeForTest, err := time.Parse(time.RFC3339, fakeTimeStr)

	if err != nil {
		panic(fmt.Sprintf("Test has an invalid fake time: %s", err))
	}

	return fakeTimeForTest
}

type protoMatcher struct {
}

func createTestContext(ctrl *gomock.Controller, params testParameters) testContext {
	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTX(ctrl)

	mockTx.EXPECT().WriteRevision().AnyTimes().Return(params.writeRevision)

	if params.beginFails {
		mockStorage.EXPECT().Begin().AnyTimes().Return(mockTx, errors.New("TX"))
	} else {
		mockStorage.EXPECT().Begin().AnyTimes().Return(mockTx, nil)
	}

	if params.shouldCommit {
		if !params.commitFails {
			mockTx.EXPECT().Commit().AnyTimes().Return(nil)
		} else {
			mockTx.EXPECT().Commit().AnyTimes().Return(params.commitError)
		}
	}

	if params.shouldRollback {
		mockTx.EXPECT().Rollback().AnyTimes().Return(nil)
	}

	if !params.skipDequeue {
		if params.overrideDequeueTime != nil {
			mockTx.EXPECT().DequeueLeaves(params.dequeueLimit, *params.overrideDequeueTime).AnyTimes().Return(params.dequeuedLeaves, params.dequeuedError)
		} else {
			mockTx.EXPECT().DequeueLeaves(params.dequeueLimit, fakeTimeForTest).AnyTimes().Return(params.dequeuedLeaves, params.dequeuedError)
		}
	}

	if params.latestSignedRoot != nil {
		mockTx.EXPECT().LatestSignedLogRoot().AnyTimes().Return(*params.latestSignedRoot, params.latestSignedRootError)
	}

	if params.updatedLeaves != nil {
		mockTx.EXPECT().UpdateSequencedLeaves(*params.updatedLeaves).AnyTimes().Return(params.updatedLeavesError)
	}

	if params.merkleNodesSet != nil {
		mockTx.EXPECT().SetMerkleNodes(testonly.NodeSet(*params.merkleNodesSet)).AnyTimes().Return(params.merkleNodesSetError)
	}

	if !params.skipStoreSignedRoot {
		if params.storeSignedRoot != nil {
			mockTx.EXPECT().StoreSignedLogRoot(*params.storeSignedRoot).AnyTimes().Return(params.storeSignedRootError)
		} else {
			// At the moment if we're going to fail the operation we accept any root
			mockTx.EXPECT().StoreSignedLogRoot(gomock.Any()).AnyTimes().Return(params.storeSignedRootError)
		}
	}

	mockKeyManager := crypto.NewMockKeyManager(ctrl)

	if params.setupSigner {
		mockSigner := crypto.NewMockSigner(ctrl)
		mockSigner.EXPECT().Sign(gomock.Any(), params.dataToSign, treeHasher.Hasher).AnyTimes().Return(params.signingResult, params.signingError)
		mockKeyManager.EXPECT().Signer().AnyTimes().Return(mockSigner, params.keyManagerError)
	}

	sequencer := NewSequencer(treeHasher, util.FakeTimeSource{FakeTime: fakeTimeForTest}, mockStorage, mockKeyManager)

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
		t.Error("Expected nil return with no work pending in queue")
	}
}

// Tests that the guard interval is being sent to storage correctly. Actual operation of the
// window is tested by storage tests.
func TestGuardWindowPassthrough(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	guardInterval := time.Second * 10
	expectedCutoffTime := fakeTimeForTest.Add(-guardInterval)
	params := testParameters{dequeueLimit: 1, shouldCommit: true, latestSignedRoot: &testRoot16, dequeuedLeaves: []trillian.LogLeaf{}, skipStoreSignedRoot: true, overrideDequeueTime:&expectedCutoffTime}

	c := createTestContext(ctrl, params)
	c.sequencer.SetGuardWindow(guardInterval)

	leaves, err := c.sequencer.SequenceBatch(1, rootNeverExpiresFunc)
	if leaves != 0 {
		t.Fatalf("Unexpectedly sequenced %d leaves on error", leaves)
	}

	if err != nil {
		t.Error("Expected nil return with no work pending in queue")
	}
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
}

func TestUpdateSequencedLeavesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	leaves := []trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []trillian.LogLeaf{testLeaf16}
	params := testParameters{writeRevision: testRoot16.TreeRevision + 1, dequeueLimit: 1, shouldRollback: true, dequeuedLeaves: leaves,
		latestSignedRoot: &testRoot16, updatedLeaves: &updatedLeaves,
		updatedLeavesError: errors.New("unsequenced")}
	c := createTestContext(ctrl, params)

	leafCount, err := c.sequencer.SequenceBatch(1, rootNeverExpiresFunc)
	if leafCount != 0 {
		t.Fatalf("Unexpectedly sequenced %d leaves on error", leafCount)
	}
	testonly.EnsureErrorContains(t, err, "unsequenced")
}

func TestSetMerkleNodesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	leaves := []trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []trillian.LogLeaf{testLeaf16}
	params := testParameters{writeRevision: testRoot16.TreeRevision + 1, dequeueLimit: 1, shouldRollback: true, dequeuedLeaves: leaves,
		latestSignedRoot: &testRoot16, updatedLeaves: &updatedLeaves, merkleNodesSet: &updatedNodes,
		merkleNodesSetError: errors.New("setmerklenodes")}
	c := createTestContext(ctrl, params)

	leafCount, err := c.sequencer.SequenceBatch(1, rootNeverExpiresFunc)
	if leafCount != 0 {
		t.Fatalf("Unexpectedly sequenced %d leaves on error", leafCount)
	}
	testonly.EnsureErrorContains(t, err, "setmerklenodes")
}

func TestStoreSignedRootError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	leaves := []trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []trillian.LogLeaf{testLeaf16}
	params := testParameters{writeRevision: testRoot16.TreeRevision + 1, dequeueLimit: 1, shouldRollback: true, dequeuedLeaves: leaves,
		latestSignedRoot: &testRoot16, updatedLeaves: &updatedLeaves, merkleNodesSet: &updatedNodes,
		storeSignedRoot:      nil,
		storeSignedRootError: errors.New("storesignedroot"), setupSigner: true,
		dataToSign:    []byte{118, 113, 60, 123, 201, 107, 151, 27, 190, 53, 148, 77, 139, 138, 128, 71, 231, 103, 131, 160, 23, 10, 65, 81, 64, 173, 1, 151, 36, 239, 22, 3},
		signingResult: []byte("signed")}
	c := createTestContext(ctrl, params)

	leafCount, err := c.sequencer.SequenceBatch(1, rootNeverExpiresFunc)
	if leafCount != 0 {
		t.Fatalf("Unexpectedly sequenced %d leaves on error", leafCount)
	}
	testonly.EnsureErrorContains(t, err, "storesignedroot")
}

func TestStoreSignedRootKeyManagerFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	leaves := []trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []trillian.LogLeaf{testLeaf16}
	params := testParameters{writeRevision: testRoot16.TreeRevision + 1, dequeueLimit: 1, shouldRollback: true, dequeuedLeaves: leaves,
		latestSignedRoot: &testRoot16, updatedLeaves: &updatedLeaves, merkleNodesSet: &updatedNodes,
		storeSignedRoot: nil,
		setupSigner:     true,
		keyManagerError: errors.New("keymanagerfailed")}
	c := createTestContext(ctrl, params)

	leafCount, err := c.sequencer.SequenceBatch(1, rootNeverExpiresFunc)
	if leafCount != 0 {
		t.Fatalf("Unexpectedly sequenced %d leaves on error", leafCount)
	}
	testonly.EnsureErrorContains(t, err, "keymanagerfailed")
}

func TestStoreSignedRootSignerFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	leaves := []trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []trillian.LogLeaf{testLeaf16}
	params := testParameters{writeRevision: testRoot16.TreeRevision + 1, dequeueLimit: 1, shouldRollback: true, dequeuedLeaves: leaves,
		latestSignedRoot: &testRoot16, updatedLeaves: &updatedLeaves, merkleNodesSet: &updatedNodes,
		storeSignedRoot: nil,
		setupSigner:     true,
		dataToSign:      []byte{118, 113, 60, 123, 201, 107, 151, 27, 190, 53, 148, 77, 139, 138, 128, 71, 231, 103, 131, 160, 23, 10, 65, 81, 64, 173, 1, 151, 36, 239, 22, 3},
		signingError:    errors.New("signerfailed")}
	c := createTestContext(ctrl, params)

	leafCount, err := c.sequencer.SequenceBatch(1, rootNeverExpiresFunc)
	if leafCount != 0 {
		t.Fatalf("Unexpectedly sequenced %d leaves on error", leafCount)
	}
	testonly.EnsureErrorContains(t, err, "signerfailed")
}

func TestCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	leaves := []trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []trillian.LogLeaf{testLeaf16}

	params := testParameters{writeRevision: testRoot16.TreeRevision + 1, dequeueLimit: 1, shouldCommit: true, commitFails: true,
		commitError: errors.New("commit"), dequeuedLeaves: leaves,
		latestSignedRoot: &testRoot16, updatedLeaves: &updatedLeaves, merkleNodesSet: &updatedNodes,
		storeSignedRoot: nil, setupSigner: true,
		dataToSign:    []byte{118, 113, 60, 123, 201, 107, 151, 27, 190, 53, 148, 77, 139, 138, 128, 71, 231, 103, 131, 160, 23, 10, 65, 81, 64, 173, 1, 151, 36, 239, 22, 3},
		signingResult: []byte("signed")}
	c := createTestContext(ctrl, params)

	leafCount, err := c.sequencer.SequenceBatch(1, rootNeverExpiresFunc)
	if leafCount != 0 {
		t.Fatalf("Unexpectedly sequenced %d leaves on error", leafCount)
	}
	testonly.EnsureErrorContains(t, err, "commit")
}

// TODO: We used a perfect tree size so this isn't testing code that loads the compact Merkle
// tree. This will be done later as it's planned to refactor it anyway.
func TestSequenceBatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	leaves := []trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []trillian.LogLeaf{testLeaf16}
	params := testParameters{writeRevision: testRoot16.TreeRevision + 1, dequeueLimit: 1, shouldCommit: true,
		dequeuedLeaves: leaves, latestSignedRoot: &testRoot16,
		updatedLeaves: &updatedLeaves, merkleNodesSet: &updatedNodes,
		storeSignedRoot: &expectedSignedRoot, setupSigner: true,
		dataToSign:    []byte{118, 113, 60, 123, 201, 107, 151, 27, 190, 53, 148, 77, 139, 138, 128, 71, 231, 103, 131, 160, 23, 10, 65, 81, 64, 173, 1, 151, 36, 239, 22, 3},
		signingResult: []byte("signed")}
	c := createTestContext(ctrl, params)

	leafCount, err := c.sequencer.SequenceBatch(1, rootNeverExpiresFunc)
	if err != nil {
		t.Fatalf("Expected sequencing to succeed, but got err: %v", err)
	}
	if got, want := leafCount, 1; got != want {
		t.Fatalf("Sequenced %d leaf, expected %d", got, want)
	}
}

func TestSignBeginTxFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	params := testParameters{beginFails: true}
	c := createTestContext(ctrl, params)

	err := c.sequencer.SignRoot()
	testonly.EnsureErrorContains(t, err, "TX")
}

func TestSignLatestRootFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	params := testParameters{writeRevision: testRoot16.TreeRevision + 1,
		dequeueLimit: 1, shouldRollback: true,
		latestSignedRoot: &testRoot16, latestSignedRootError: errors.New("root")}
	c := createTestContext(ctrl, params)

	err := c.sequencer.SignRoot()
	testonly.EnsureErrorContains(t, err, "root")
}

func TestSignedRootKeyManagerFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	params := testParameters{writeRevision: testRoot16.TreeRevision + 1,
		dequeueLimit: 1, shouldRollback: true,
		latestSignedRoot: &testRoot16,
		storeSignedRoot:  nil,
		setupSigner:      true, keyManagerError: errors.New("keymanagerfailed")}
	c := createTestContext(ctrl, params)

	err := c.sequencer.SignRoot()
	testonly.EnsureErrorContains(t, err, "keymanager")
}

func TestSignRootSignerFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	params := testParameters{writeRevision: testRoot16.TreeRevision + 1,
		dequeueLimit: 1, shouldRollback: true,
		latestSignedRoot: &testRoot16,
		storeSignedRoot:  nil, setupSigner: true,
		dataToSign:   []byte{0x95, 0x46, 0xdc, 0x25, 0xfb, 0x74, 0x41, 0x4b, 0x50, 0x2e, 0xb0, 0x93, 0x99, 0xbb, 0x5e, 0xf6, 0x57, 0x58, 0xb9, 0x7a, 0x3a, 0x8f, 0xae, 0x35, 0xe1, 0xf6, 0xcd, 0x6c, 0x2a, 0xe6, 0x27, 0xbe},
		signingError: errors.New("signerfailed")}
	c := createTestContext(ctrl, params)

	err := c.sequencer.SignRoot()
	testonly.EnsureErrorContains(t, err, "signer")
}

func TestSignRootStoreSignedRootFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	params := testParameters{writeRevision: testRoot16.TreeRevision + 1,
		shouldRollback:       true,
		latestSignedRoot:     &testRoot16,
		storeSignedRoot:      nil,
		storeSignedRootError: errors.New("storesignedroot"), setupSigner: true,
		dataToSign:    []byte{0x95, 0x46, 0xdc, 0x25, 0xfb, 0x74, 0x41, 0x4b, 0x50, 0x2e, 0xb0, 0x93, 0x99, 0xbb, 0x5e, 0xf6, 0x57, 0x58, 0xb9, 0x7a, 0x3a, 0x8f, 0xae, 0x35, 0xe1, 0xf6, 0xcd, 0x6c, 0x2a, 0xe6, 0x27, 0xbe},
		signingResult: []byte("signed")}
	c := createTestContext(ctrl, params)

	err := c.sequencer.SignRoot()
	testonly.EnsureErrorContains(t, err, "storesignedroot")
}

func TestSignRootCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	params := testParameters{writeRevision: testRoot16.TreeRevision + 1,
		shouldCommit: true, commitFails: true,
		commitError:      errors.New("commit"),
		latestSignedRoot: &testRoot16,
		storeSignedRoot:  nil, setupSigner: true,
		dataToSign:    []byte{0x95, 0x46, 0xdc, 0x25, 0xfb, 0x74, 0x41, 0x4b, 0x50, 0x2e, 0xb0, 0x93, 0x99, 0xbb, 0x5e, 0xf6, 0x57, 0x58, 0xb9, 0x7a, 0x3a, 0x8f, 0xae, 0x35, 0xe1, 0xf6, 0xcd, 0x6c, 0x2a, 0xe6, 0x27, 0xbe},
		signingResult: []byte("signed")}
	c := createTestContext(ctrl, params)

	err := c.sequencer.SignRoot()
	testonly.EnsureErrorContains(t, err, "commit")
}

func TestSignRoot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	params := testParameters{writeRevision: testRoot16.TreeRevision + 1,
		shouldRollback:   true,
		latestSignedRoot: &testRoot16,
		storeSignedRoot:  &expectedSignedRoot16,
		setupSigner:      true,
		dataToSign:       []byte{0x95, 0x46, 0xdc, 0x25, 0xfb, 0x74, 0x41, 0x4b, 0x50, 0x2e, 0xb0, 0x93, 0x99, 0xbb, 0x5e, 0xf6, 0x57, 0x58, 0xb9, 0x7a, 0x3a, 0x8f, 0xae, 0x35, 0xe1, 0xf6, 0xcd, 0x6c, 0x2a, 0xe6, 0x27, 0xbe},
		signingResult:    []byte("signed"), shouldCommit: true}
	c := createTestContext(ctrl, params)

	if err := c.sequencer.SignRoot(); err != nil {
		t.Fatalf("Expected signing to succeed, but got err: %v", err)
	}
}

func TestSignRootNoExistingRoot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	params := testParameters{writeRevision: testRoot16.TreeRevision + 1,
		shouldRollback:   true,
		latestSignedRoot: &trillian.SignedLogRoot{},
		storeSignedRoot:  &expectedSignedRoot0,
		setupSigner:      true,
		dataToSign:       []byte{0xc2, 0xc, 0x1e, 0x33, 0x8, 0xcd, 0x2d, 0x50, 0xbb, 0xf9, 0xf9, 0x1, 0x29, 0xb2, 0xfb, 0xb9, 0x4d, 0x30, 0x27, 0x84, 0xf2, 0xc0, 0x48, 0x5f, 0x46, 0xd4, 0xbe, 0x8a, 0xb8, 0x27, 0x96, 0x22},
		signingResult:    []byte("signed"), shouldCommit: true}
	c := createTestContext(ctrl, params)

	if err := c.sequencer.SignRoot(); err != nil {
		t.Fatalf("Expected signing to succeed, but got err: %v", err)
	}
}
