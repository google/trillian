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

package log

import (
	"context"
	gocrypto "crypto"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/util"
)

var (
	// These can be shared between tests as they're never modified
	testLeaf16Data = []byte("testdataforleaf")
	testLeaf16     = &trillian.LogLeaf{
		MerkleLeafHash: testonly.Hasher.HashLeaf(testLeaf16Data),
		LeafValue:      testLeaf16Data,
		ExtraData:      nil,
		LeafIndex:      16,
	}
)

// RootHash can't be nil because that's how the sequencer currently detects that there was no stored tree head.
var testRoot16 = trillian.SignedLogRoot{TreeSize: 16, TreeRevision: 5, RootHash: []byte{}}

// These will be accepted in either order because of custom sorting in the mock
var updatedNodes = []storage.Node{
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
	Signature: &sigpb.DigitallySigned{
		SignatureAlgorithm: sigpb.DigitallySigned_ECDSA,
		HashAlgorithm:      sigpb.DigitallySigned_SHA256,
		Signature:          []byte("signed"),
	},
}

var expectedSignedRoot16 = trillian.SignedLogRoot{
	// TODO(Martin2112): An extended test that checks the root hash
	TimestampNanos: fakeTimeForTest.UnixNano(),
	TreeRevision:   6,
	TreeSize:       16,
	RootHash:       testRoot16.RootHash,
	LogId:          0,
	Signature: &sigpb.DigitallySigned{
		SignatureAlgorithm: sigpb.DigitallySigned_ECDSA,
		HashAlgorithm:      sigpb.DigitallySigned_SHA256,
		Signature:          []byte("signed"),
	},
}

// expectedSignedRoot0 is a root for an empty tree
var expectedSignedRoot0 = trillian.SignedLogRoot{
	RootHash:       []byte{0xe3, 0xb0, 0xc4, 0x42, 0x98, 0xfc, 0x1c, 0x14, 0x9a, 0xfb, 0xf4, 0xc8, 0x99, 0x6f, 0xb9, 0x24, 0x27, 0xae, 0x41, 0xe4, 0x64, 0x9b, 0x93, 0x4c, 0xa4, 0x95, 0x99, 0x1b, 0x78, 0x52, 0xb8, 0x55},
	TimestampNanos: fakeTimeForTest.UnixNano(),
	TreeRevision:   1,
	TreeSize:       0,
	LogId:          0,
	Signature: &sigpb.DigitallySigned{
		SignatureAlgorithm: sigpb.DigitallySigned_ECDSA,
		HashAlgorithm:      sigpb.DigitallySigned_SHA256,
		Signature:          []byte("signed"),
	},
}

// Any tests relying on time should use this fixed value
const fakeTimeStr string = "2016-05-25T10:55:05Z"

// testParameters bundles up values needed for setting mock expectations in tests
type testParameters struct {
	fakeTime time.Time

	logID  int64
	signer gocrypto.Signer

	beginFails   bool
	dequeueLimit int

	shouldCommit   bool
	commitFails    bool
	commitError    error
	shouldRollback bool

	skipDequeue    bool
	dequeuedLeaves []*trillian.LogLeaf
	dequeuedError  error

	latestSignedRootError error
	latestSignedRoot      *trillian.SignedLogRoot

	updatedLeaves      *[]*trillian.LogLeaf
	updatedLeavesError error

	merkleNodesSet      *[]storage.Node
	merkleNodesSetError error

	skipStoreSignedRoot  bool
	storeSignedRoot      *trillian.SignedLogRoot
	storeSignedRootError error

	writeRevision int64

	overrideDequeueTime *time.Time
}

// Tests get their own mock context so they can be run in parallel safely
type testContext struct {
	mockTx      *storage.MockLogTreeTX
	mockStorage *storage.MockLogStorage
	signer      *crypto.Signer
	sequencer   *Sequencer
}

// This gets modified so tests need their own copies
func getLeaf42() *trillian.LogLeaf {
	return &trillian.LogLeaf{
		MerkleLeafHash: testonly.Hasher.HashLeaf(testLeaf16Data),
		LeafValue:      testLeaf16Data,
		ExtraData:      nil,
		LeafIndex:      42,
	}
}

func fakeTime() time.Time {
	fakeTimeForTest, err := time.Parse(time.RFC3339, fakeTimeStr)

	if err != nil {
		panic(fmt.Sprintf("Test has an invalid fake time: %s", err))
	}

	return fakeTimeForTest
}

func newSignerWithFixedSig(sig *sigpb.DigitallySigned) (gocrypto.Signer, error) {
	key, err := keys.NewFromPublicPEM(testonly.DemoPublicKey)
	if err != nil {
		return nil, err
	}

	if got, want := sig.GetSignatureAlgorithm(), keys.SignatureAlgorithm(key); got != want {
		return nil, fmt.Errorf("signature algorithm (%v) does not match key (%v)", got, want)
	}

	return testonly.NewSignerWithFixedSig(key, sig.Signature), nil
}

func newSignerWithErr(signErr error) (gocrypto.Signer, error) {
	key, err := keys.NewFromPublicPEM(testonly.DemoPublicKey)
	if err != nil {
		return nil, err
	}

	return testonly.NewSignerWithErr(key, signErr), nil
}

func createTestContext(ctrl *gomock.Controller, params testParameters) (testContext, context.Context) {
	mockStorage := storage.NewMockLogStorage(ctrl)
	mockTx := storage.NewMockLogTreeTX(ctrl)

	mockTx.EXPECT().WriteRevision().AnyTimes().Return(params.writeRevision)
	if params.beginFails {
		mockStorage.EXPECT().BeginForTree(gomock.Any(), params.logID).Return(mockTx, errors.New("TX"))
	} else {
		mockStorage.EXPECT().BeginForTree(gomock.Any(), params.logID).Return(mockTx, nil)
	}

	if params.shouldCommit {
		if !params.commitFails {
			mockTx.EXPECT().Commit().Return(nil)
		} else {
			mockTx.EXPECT().Commit().Return(params.commitError)
		}
	}
	// Close is always called, regardless of explicit commits
	mockTx.EXPECT().Close().AnyTimes().Return(nil)

	if !params.skipDequeue {
		if params.overrideDequeueTime != nil {
			mockTx.EXPECT().DequeueLeaves(params.dequeueLimit, *params.overrideDequeueTime).Return(params.dequeuedLeaves, params.dequeuedError)
		} else {
			mockTx.EXPECT().DequeueLeaves(params.dequeueLimit, fakeTimeForTest).Return(params.dequeuedLeaves, params.dequeuedError)
		}
	}

	if params.latestSignedRoot != nil {
		mockTx.EXPECT().LatestSignedLogRoot().Return(*params.latestSignedRoot, params.latestSignedRootError)
	}

	if params.updatedLeaves != nil {
		mockTx.EXPECT().UpdateSequencedLeaves(*params.updatedLeaves).Return(params.updatedLeavesError)
	}

	if params.merkleNodesSet != nil {
		mockTx.EXPECT().SetMerkleNodes(testonly.NodeSet(*params.merkleNodesSet)).Return(params.merkleNodesSetError)
	}

	if !params.skipStoreSignedRoot {
		if params.storeSignedRoot != nil {
			mockTx.EXPECT().StoreSignedLogRoot(*params.storeSignedRoot).Return(params.storeSignedRootError)
		} else {
			// At the moment if we're going to fail the operation we accept any root
			mockTx.EXPECT().StoreSignedLogRoot(gomock.Any()).Return(params.storeSignedRootError)
		}
	}

	signer := crypto.NewSigner(params.signer)
	sequencer := NewSequencer(testonly.Hasher, util.FakeTimeSource{FakeTime: fakeTimeForTest}, mockStorage, signer)

	return testContext{mockTx: mockTx, mockStorage: mockStorage, signer: signer, sequencer: sequencer}, util.NewLogContext(context.Background(), params.logID)
}

// Tests for sequencer. Currently relies on having a database set up. This might change in future
// as it would be better if it was not tied to a specific storage mechanism.

func TestBeginTXFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	params := testParameters{
		logID:               154035,
		beginFails:          true,
		skipDequeue:         true,
		skipStoreSignedRoot: true,
	}
	c, ctx := createTestContext(ctrl, params)

	leaves, err := c.sequencer.SequenceBatch(ctx, params.logID, 1)
	if leaves != 0 {
		t.Fatalf("Unexpectedly sequenced %d leaves on error", leaves)
	}
	testonly.EnsureErrorContains(t, err, "TX")
}

func TestSequenceWithNothingQueued(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	params := testParameters{
		logID:               154035,
		dequeueLimit:        1,
		shouldCommit:        true,
		latestSignedRoot:    &testRoot16,
		dequeuedLeaves:      []*trillian.LogLeaf{},
		skipStoreSignedRoot: true,
	}
	c, ctx := createTestContext(ctrl, params)

	leaves, err := c.sequencer.SequenceBatch(ctx, params.logID, 1)
	if leaves != 0 {
		t.Fatalf("Unexpectedly sequenced %d leaves on error", leaves)
	}

	if err != nil {
		t.Error("Expected nil return with no work pending in queue")
	}
}

// Tests that the guard interval is being passed to storage correctly. Actual operation of the
// window is tested by storage tests.
func TestGuardWindowPassthrough(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	guardInterval := time.Second * 10
	expectedCutoffTime := fakeTimeForTest.Add(-guardInterval)
	params := testParameters{
		logID:               154035,
		dequeueLimit:        1,
		shouldCommit:        true,
		latestSignedRoot:    &testRoot16,
		dequeuedLeaves:      []*trillian.LogLeaf{},
		skipStoreSignedRoot: true,
		overrideDequeueTime: &expectedCutoffTime,
	}
	c, ctx := createTestContext(ctrl, params)
	c.sequencer.SetGuardWindow(guardInterval)

	leaves, err := c.sequencer.SequenceBatch(ctx, params.logID, 1)
	if leaves != 0 {
		t.Fatalf("Expected no leaves sequenced when in guard interval but got: %d", leaves)
	}

	if err != nil {
		t.Errorf("Expected nil return with all queued work inside guard interval but got: %v", err)
	}
}

func TestDequeueError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	params := testParameters{
		logID:               154035,
		dequeueLimit:        1,
		dequeuedError:       errors.New("dequeue"),
		skipStoreSignedRoot: true,
	}
	c, ctx := createTestContext(ctrl, params)

	leafCount, err := c.sequencer.SequenceBatch(ctx, params.logID, 1)
	testonly.EnsureErrorContains(t, err, "dequeue")
	if leafCount != 0 {
		t.Fatalf("Unexpectedly sequenced %d leaves on error", leafCount)
	}
}

func TestLatestRootError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	leaves := []*trillian.LogLeaf{getLeaf42()}
	params := testParameters{
		logID:                 154035,
		dequeueLimit:          1,
		dequeuedLeaves:        leaves,
		latestSignedRoot:      &testRoot16,
		latestSignedRootError: errors.New("root"),
		skipStoreSignedRoot:   true,
	}
	c, ctx := createTestContext(ctrl, params)

	leafCount, err := c.sequencer.SequenceBatch(ctx, params.logID, 1)
	if leafCount != 0 {
		t.Fatalf("Unexpectedly sequenced %d leaves on error", leafCount)
	}
	testonly.EnsureErrorContains(t, err, "root")
}

func TestUpdateSequencedLeavesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	leaves := []*trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []*trillian.LogLeaf{testLeaf16}
	params := testParameters{
		logID:               154035,
		writeRevision:       testRoot16.TreeRevision + 1,
		dequeueLimit:        1,
		dequeuedLeaves:      leaves,
		latestSignedRoot:    &testRoot16,
		updatedLeaves:       &updatedLeaves,
		updatedLeavesError:  errors.New("unsequenced"),
		skipStoreSignedRoot: true,
	}
	c, ctx := createTestContext(ctrl, params)

	leafCount, err := c.sequencer.SequenceBatch(ctx, params.logID, 1)
	if leafCount != 0 {
		t.Fatalf("Unexpectedly sequenced %d leaves on error", leafCount)
	}
	testonly.EnsureErrorContains(t, err, "unsequenced")
}

func TestSetMerkleNodesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	leaves := []*trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []*trillian.LogLeaf{testLeaf16}
	params := testParameters{
		logID:               154035,
		writeRevision:       testRoot16.TreeRevision + 1,
		dequeueLimit:        1,
		dequeuedLeaves:      leaves,
		latestSignedRoot:    &testRoot16,
		updatedLeaves:       &updatedLeaves,
		merkleNodesSet:      &updatedNodes,
		merkleNodesSetError: errors.New("setmerklenodes"),
		skipStoreSignedRoot: true,
	}
	c, ctx := createTestContext(ctrl, params)

	leafCount, err := c.sequencer.SequenceBatch(ctx, params.logID, 1)
	if leafCount != 0 {
		t.Fatalf("Unexpectedly sequenced %d leaves on error", leafCount)
	}
	testonly.EnsureErrorContains(t, err, "setmerklenodes")
}

func TestStoreSignedRootError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	leaves := []*trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []*trillian.LogLeaf{testLeaf16}

	signer, err := newSignerWithFixedSig(expectedSignedRoot16.Signature)
	if err != nil {
		t.Fatalf("Failed to create test signer (%v)", err)
	}

	params := testParameters{
		logID:                154035,
		writeRevision:        testRoot16.TreeRevision + 1,
		dequeueLimit:         1,
		dequeuedLeaves:       leaves,
		latestSignedRoot:     &testRoot16,
		updatedLeaves:        &updatedLeaves,
		merkleNodesSet:       &updatedNodes,
		storeSignedRoot:      nil,
		storeSignedRootError: errors.New("storesignedroot"),
		signer:               signer,
	}
	c, ctx := createTestContext(ctrl, params)

	leafCount, err := c.sequencer.SequenceBatch(ctx, params.logID, 1)
	if leafCount != 0 {
		t.Fatalf("Unexpectedly sequenced %d leaves on error", leafCount)
	}
	testonly.EnsureErrorContains(t, err, "storesignedroot")
}

func TestStoreSignedRootSignerFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	leaves := []*trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []*trillian.LogLeaf{testLeaf16}

	signer, err := newSignerWithErr(errors.New("signerfailed"))
	if err != nil {
		t.Fatalf("Failed to create test signer (%v)", err)
	}

	params := testParameters{
		logID:               154035,
		writeRevision:       testRoot16.TreeRevision + 1,
		dequeueLimit:        1,
		dequeuedLeaves:      leaves,
		latestSignedRoot:    &testRoot16,
		updatedLeaves:       &updatedLeaves,
		merkleNodesSet:      &updatedNodes,
		storeSignedRoot:     nil,
		signer:              signer,
		skipStoreSignedRoot: true,
	}
	c, ctx := createTestContext(ctrl, params)

	leafCount, err := c.sequencer.SequenceBatch(ctx, params.logID, 1)
	if leafCount != 0 {
		t.Fatalf("Unexpectedly sequenced %d leaves on error", leafCount)
	}
	testonly.EnsureErrorContains(t, err, "signerfailed")
}

func TestCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	leaves := []*trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []*trillian.LogLeaf{testLeaf16}

	signer, err := newSignerWithFixedSig(expectedSignedRoot16.Signature)
	if err != nil {
		t.Fatalf("Failed to create test signer (%v)", err)
	}

	params := testParameters{
		logID:            154035,
		writeRevision:    testRoot16.TreeRevision + 1,
		dequeueLimit:     1,
		shouldCommit:     true,
		commitFails:      true,
		commitError:      errors.New("commit"),
		dequeuedLeaves:   leaves,
		latestSignedRoot: &testRoot16,
		updatedLeaves:    &updatedLeaves,
		merkleNodesSet:   &updatedNodes,
		storeSignedRoot:  nil,
		signer:           signer,
	}
	c, ctx := createTestContext(ctrl, params)

	leafCount, err := c.sequencer.SequenceBatch(ctx, params.logID, 1)
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

	leaves := []*trillian.LogLeaf{getLeaf42()}
	updatedLeaves := []*trillian.LogLeaf{testLeaf16}

	signer, err := newSignerWithFixedSig(expectedSignedRoot.Signature)
	if err != nil {
		t.Fatalf("Failed to create test signer (%v)", err)
	}

	params := testParameters{
		logID:            154035,
		writeRevision:    testRoot16.TreeRevision + 1,
		dequeueLimit:     1,
		shouldCommit:     true,
		dequeuedLeaves:   leaves,
		latestSignedRoot: &testRoot16,
		updatedLeaves:    &updatedLeaves,
		merkleNodesSet:   &updatedNodes,
		storeSignedRoot:  &expectedSignedRoot,
		signer:           signer,
	}
	c, ctx := createTestContext(ctrl, params)

	leafCount, err := c.sequencer.SequenceBatch(ctx, params.logID, 1)
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

	params := testParameters{
		logID:               154035,
		beginFails:          true,
		skipDequeue:         true,
		skipStoreSignedRoot: true,
	}
	c, ctx := createTestContext(ctrl, params)

	err := c.sequencer.SignRoot(ctx, params.logID)
	testonly.EnsureErrorContains(t, err, "TX")
}

func TestSignLatestRootFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	params := testParameters{
		logID:                 154035,
		writeRevision:         testRoot16.TreeRevision + 1,
		dequeueLimit:          1,
		latestSignedRoot:      &testRoot16,
		latestSignedRootError: errors.New("root"),
		skipDequeue:           true,
		skipStoreSignedRoot:   true,
	}
	c, ctx := createTestContext(ctrl, params)

	err := c.sequencer.SignRoot(ctx, params.logID)
	testonly.EnsureErrorContains(t, err, "root")
}

func TestSignRootSignerFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	signer, err := newSignerWithErr(errors.New("signerfailed"))
	if err != nil {
		t.Fatalf("Failed to create test signer (%v)", err)
	}

	params := testParameters{
		logID:               154035,
		writeRevision:       testRoot16.TreeRevision + 1,
		dequeueLimit:        1,
		latestSignedRoot:    &testRoot16,
		storeSignedRoot:     nil,
		signer:              signer,
		skipDequeue:         true,
		skipStoreSignedRoot: true,
	}
	c, ctx := createTestContext(ctrl, params)

	err = c.sequencer.SignRoot(ctx, params.logID)
	testonly.EnsureErrorContains(t, err, "signer")
}

func TestSignRootStoreSignedRootFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	signer, err := newSignerWithFixedSig(expectedSignedRoot16.Signature)
	if err != nil {
		t.Fatalf("Failed to create test signer (%v)", err)
	}

	params := testParameters{
		logID:                154035,
		writeRevision:        testRoot16.TreeRevision + 1,
		latestSignedRoot:     &testRoot16,
		storeSignedRoot:      nil,
		storeSignedRootError: errors.New("storesignedroot"),
		signer:               signer,
		skipDequeue:          true,
	}
	c, ctx := createTestContext(ctrl, params)

	err = c.sequencer.SignRoot(ctx, params.logID)
	testonly.EnsureErrorContains(t, err, "storesignedroot")
}

func TestSignRootCommitFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	signer, err := newSignerWithFixedSig(expectedSignedRoot16.Signature)
	if err != nil {
		t.Fatalf("Failed to create test signer (%v)", err)
	}

	params := testParameters{
		logID:            154035,
		writeRevision:    testRoot16.TreeRevision + 1,
		shouldCommit:     true,
		commitFails:      true,
		commitError:      errors.New("commit"),
		latestSignedRoot: &testRoot16,
		storeSignedRoot:  nil,
		signer:           signer,
		skipDequeue:      true,
	}
	c, ctx := createTestContext(ctrl, params)

	err = c.sequencer.SignRoot(ctx, params.logID)
	testonly.EnsureErrorContains(t, err, "commit")
}

func TestSignRoot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	signer, err := newSignerWithFixedSig(expectedSignedRoot16.Signature)
	if err != nil {
		t.Fatalf("Failed to create test signer (%v)", err)
	}

	params := testParameters{
		logID:            154035,
		writeRevision:    testRoot16.TreeRevision + 1,
		latestSignedRoot: &testRoot16,
		storeSignedRoot:  &expectedSignedRoot16,
		signer:           signer,
		shouldCommit:     true,
		skipDequeue:      true,
	}
	c, ctx := createTestContext(ctrl, params)

	if err := c.sequencer.SignRoot(ctx, params.logID); err != nil {
		t.Fatalf("Expected signing to succeed, but got err: %v", err)
	}
}

func TestSignRootNoExistingRoot(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	signer, err := newSignerWithFixedSig(expectedSignedRoot0.Signature)
	if err != nil {
		t.Fatalf("Failed to create test signer (%v)", err)
	}

	params := testParameters{
		logID:            154035,
		writeRevision:    testRoot16.TreeRevision + 1,
		latestSignedRoot: &trillian.SignedLogRoot{},
		storeSignedRoot:  &expectedSignedRoot0,
		signer:           signer,
		shouldCommit:     true,
		skipDequeue:      true,
	}
	c, ctx := createTestContext(ctrl, params)

	if err := c.sequencer.SignRoot(ctx, params.logID); err != nil {
		t.Fatalf("Expected signing to succeed, but got err: %v", err)
	}
}
