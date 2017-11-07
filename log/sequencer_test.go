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
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/crypto/keys/pem"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/storage"
	stestonly "github.com/google/trillian/storage/testonly"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/util"
)

var (
	// These can be shared between tests as they're never modified
	testLeaf16Data    = []byte("testdataforleaf")
	testLeaf16Hash, _ = rfc6962.DefaultHasher.HashLeaf(testLeaf16Data)
	testLeaf16        = &trillian.LogLeaf{
		MerkleLeafHash: testLeaf16Hash,
		LeafValue:      testLeaf16Data,
		ExtraData:      nil,
		LeafIndex:      16,
	}
)

var fakeTimeForTest = fakeTime()

// RootHash can't be nil because that's how the sequencer currently detects that there was no stored tree head.
var testRoot16 = trillian.SignedLogRoot{
	TreeSize:       16,
	TreeRevision:   5,
	RootHash:       []byte{},
	TimestampNanos: fakeTimeForTest.Add(-10 * time.Millisecond).UnixNano(),
}

// These will be accepted in either order because of custom sorting in the mock
var updatedNodes = []storage.Node{
	{NodeID: storage.NodeID{Path: []uint8{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10}, PrefixLenBits: 64},
		Hash: testonly.MustDecodeBase64("L5Iyd7aFOVewxiRm29xD+EU+jvEo4RfufBijKdflWMk="), NodeRevision: 6},
	{
		NodeID: storage.NodeID{Path: []uint8{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, PrefixLenBits: 59},
		Hash:   testonly.MustDecodeBase64("R57DrKTGuZdjCNXjv6InGrm4rABLOn9yWpdHmYOoLwU="), NodeRevision: 6},
}

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
	logID  int64
	signer gocrypto.Signer

	beginFails   bool
	dequeueLimit int

	shouldCommit bool
	commitFails  bool
	commitError  error

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

	// qm is the quota.Manager to be used. If nil, quota.Noop() is used instead.
	qm quota.Manager
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
	testLeaf16Hash, _ := rfc6962.DefaultHasher.HashLeaf(testLeaf16Data)
	return &trillian.LogLeaf{
		MerkleLeafHash: testLeaf16Hash,
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
	key, err := pem.UnmarshalPublicKey(testonly.DemoPublicKey)
	if err != nil {
		return nil, err
	}

	if got, want := sig.GetSignatureAlgorithm(), crypto.SignatureAlgorithm(key); got != want {
		return nil, fmt.Errorf("signature algorithm (%v) does not match key (%v)", got, want)
	}

	return testonly.NewSignerWithFixedSig(key, sig.Signature), nil
}

func newSignerWithErr(signErr error) (gocrypto.Signer, error) {
	key, err := pem.UnmarshalPublicKey(testonly.DemoPublicKey)
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
			mockTx.EXPECT().DequeueLeaves(gomock.Any(), params.dequeueLimit, *params.overrideDequeueTime).Return(params.dequeuedLeaves, params.dequeuedError)
		} else {
			mockTx.EXPECT().DequeueLeaves(gomock.Any(), params.dequeueLimit, fakeTimeForTest).Return(params.dequeuedLeaves, params.dequeuedError)
		}
	}

	if params.latestSignedRoot != nil {
		mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(*params.latestSignedRoot, params.latestSignedRootError)
	}

	if params.updatedLeaves != nil {
		mockTx.EXPECT().UpdateSequencedLeaves(gomock.Any(), *params.updatedLeaves).Return(params.updatedLeavesError)
	}

	if params.merkleNodesSet != nil {
		mockTx.EXPECT().SetMerkleNodes(gomock.Any(), stestonly.NodeSet(*params.merkleNodesSet)).Return(params.merkleNodesSetError)
	}

	if !params.skipStoreSignedRoot {
		if params.storeSignedRoot != nil {
			mockTx.EXPECT().StoreSignedLogRoot(gomock.Any(), *params.storeSignedRoot).Return(params.storeSignedRootError)
		} else {
			// At the moment if we're going to fail the operation we accept any root
			mockTx.EXPECT().StoreSignedLogRoot(gomock.Any(), gomock.Any()).Return(params.storeSignedRootError)
		}
	}

	signer := crypto.NewSHA256Signer(params.signer)
	qm := params.qm
	if qm == nil {
		qm = quota.Noop()
	}
	sequencer := NewSequencer(rfc6962.DefaultHasher, util.NewFakeTimeSource(fakeTimeForTest), mockStorage, signer, nil, qm)
	return testContext{mockTx: mockTx, mockStorage: mockStorage, signer: signer, sequencer: sequencer}, context.Background()
}

// Tests for sequencer. Currently relies on having a database set up. This might change in future
// as it would be better if it was not tied to a specific storage mechanism.

func TestSequenceBatch(t *testing.T) {
	signer1, err := newSignerWithFixedSig(expectedSignedRoot.Signature)
	if err != nil {
		t.Fatalf("Failed to create test signer (%v)", err)
	}
	signer16, err := newSignerWithFixedSig(expectedSignedRoot16.Signature)
	if err != nil {
		t.Fatalf("Failed to create test signer (%v)", err)
	}
	signerErr, err := newSignerWithErr(errors.New("signerfailed"))
	if err != nil {
		t.Fatalf("Failed to create test signer (%v)", err)
	}
	leaves16 := []*trillian.LogLeaf{testLeaf16}
	guardWindow := time.Second * 10
	expectedCutoffTime := fakeTimeForTest.Add(-guardWindow)
	noLeaves := []*trillian.LogLeaf{}
	noNodes := []storage.Node{}
	newRoot16 := trillian.SignedLogRoot{
		TimestampNanos: fakeTimeForTest.UnixNano(),
		TreeSize:       16,
		TreeRevision:   6,
		RootHash:       []byte{},
		Signature: &sigpb.DigitallySigned{
			SignatureAlgorithm: sigpb.DigitallySigned_ECDSA,
			HashAlgorithm:      sigpb.DigitallySigned_SHA256,
			Signature:          []byte("signed"),
		},
	}
	specs := []quota.Spec{
		{Group: quota.Tree, Kind: quota.Read, TreeID: 154035},
		{Group: quota.Tree, Kind: quota.Write, TreeID: 154035},
		{Group: quota.Global, Kind: quota.Read},
		{Group: quota.Global, Kind: quota.Write},
	}

	var tests = []struct {
		desc            string
		params          testParameters
		guardWindow     time.Duration
		maxRootDuration time.Duration
		wantCount       int
		errStr          string
	}{
		{
			desc: "begin-tx-fails",
			params: testParameters{
				logID:               154035,
				beginFails:          true,
				skipDequeue:         true,
				skipStoreSignedRoot: true,
			},
			errStr: "TX",
		},
		{
			desc: "nothing-queued-no-max",
			params: testParameters{
				logID:               154035,
				dequeueLimit:        1,
				shouldCommit:        true,
				latestSignedRoot:    &testRoot16,
				dequeuedLeaves:      noLeaves,
				skipStoreSignedRoot: true,
			},
		},
		{
			desc: "nothing-queued-within-max",
			params: testParameters{
				logID:               154035,
				dequeueLimit:        1,
				shouldCommit:        true,
				latestSignedRoot:    &testRoot16,
				dequeuedLeaves:      noLeaves,
				skipStoreSignedRoot: true,
			},
			maxRootDuration: 15 * time.Millisecond,
		},
		{
			desc: "nothing-queued-after-max",
			params: testParameters{
				logID:            154035,
				dequeueLimit:     1,
				shouldCommit:     true,
				latestSignedRoot: &testRoot16,
				dequeuedLeaves:   noLeaves,
				writeRevision:    testRoot16.TreeRevision + 1,
				updatedLeaves:    &noLeaves,
				merkleNodesSet:   &noNodes,
				signer:           signer16,
				storeSignedRoot:  &newRoot16,
			},
			maxRootDuration: 9 * time.Millisecond,
		},
		{
			desc: "nothing-queued-on-max",
			params: testParameters{
				logID:            154035,
				dequeueLimit:     1,
				shouldCommit:     true,
				latestSignedRoot: &testRoot16,
				dequeuedLeaves:   noLeaves,
				writeRevision:    testRoot16.TreeRevision + 1,
				updatedLeaves:    &noLeaves,
				merkleNodesSet:   &noNodes,
				signer:           signer16,
				storeSignedRoot:  &newRoot16,
			},
			maxRootDuration: 10 * time.Millisecond,
		},
		{
			// Tests that the guard interval is being passed to storage correctly.
			// Actual operation of the window is tested by storage tests.
			desc: "guard-interval",
			params: testParameters{
				logID:               154035,
				dequeueLimit:        1,
				shouldCommit:        true,
				latestSignedRoot:    &testRoot16,
				dequeuedLeaves:      []*trillian.LogLeaf{},
				skipStoreSignedRoot: true,
				overrideDequeueTime: &expectedCutoffTime,
			},
			guardWindow: guardWindow,
		},
		{
			desc: "dequeue-fails",
			params: testParameters{
				logID:               154035,
				dequeueLimit:        1,
				dequeuedError:       errors.New("dequeue"),
				skipStoreSignedRoot: true,
			},
			errStr: "dequeue",
		},
		{
			desc: "get-signed-root-fails",
			params: testParameters{
				logID:                 154035,
				dequeueLimit:          1,
				dequeuedLeaves:        []*trillian.LogLeaf{getLeaf42()},
				latestSignedRoot:      &testRoot16,
				latestSignedRootError: errors.New("root"),
				skipStoreSignedRoot:   true,
			},
			errStr: "root",
		},
		{
			desc: "update-seq-leaves-fails",
			params: testParameters{
				logID:               154035,
				writeRevision:       testRoot16.TreeRevision + 1,
				dequeueLimit:        1,
				dequeuedLeaves:      []*trillian.LogLeaf{getLeaf42()},
				latestSignedRoot:    &testRoot16,
				updatedLeaves:       &leaves16,
				updatedLeavesError:  errors.New("unsequenced"),
				skipStoreSignedRoot: true,
			},
			errStr: "unsequenced",
		},
		{
			desc: "set-merkle-nodes-fails",
			params: testParameters{
				logID:               154035,
				writeRevision:       testRoot16.TreeRevision + 1,
				dequeueLimit:        1,
				dequeuedLeaves:      []*trillian.LogLeaf{getLeaf42()},
				latestSignedRoot:    &testRoot16,
				updatedLeaves:       &leaves16,
				merkleNodesSet:      &updatedNodes,
				merkleNodesSetError: errors.New("setmerklenodes"),
				skipStoreSignedRoot: true,
			},
			errStr: "setmerklenodes",
		},
		{
			desc: "store-root-fails",
			params: testParameters{
				logID:                154035,
				writeRevision:        testRoot16.TreeRevision + 1,
				dequeueLimit:         1,
				dequeuedLeaves:       []*trillian.LogLeaf{getLeaf42()},
				latestSignedRoot:     &testRoot16,
				updatedLeaves:        &leaves16,
				merkleNodesSet:       &updatedNodes,
				storeSignedRoot:      nil,
				storeSignedRootError: errors.New("storesignedroot"),
				signer:               signer16,
			},
			errStr: "storesignedroot",
		},
		{
			desc: "signer-fails",
			params: testParameters{
				logID:               154035,
				writeRevision:       testRoot16.TreeRevision + 1,
				dequeueLimit:        1,
				dequeuedLeaves:      []*trillian.LogLeaf{getLeaf42()},
				latestSignedRoot:    &testRoot16,
				updatedLeaves:       &leaves16,
				merkleNodesSet:      &updatedNodes,
				storeSignedRoot:     nil,
				signer:              signerErr,
				skipStoreSignedRoot: true,
			},
			errStr: "signerfailed",
		},
		{
			desc: "commit-fails",
			params: testParameters{
				logID:            154035,
				writeRevision:    testRoot16.TreeRevision + 1,
				dequeueLimit:     1,
				shouldCommit:     true,
				commitFails:      true,
				commitError:      errors.New("commit"),
				dequeuedLeaves:   []*trillian.LogLeaf{getLeaf42()},
				latestSignedRoot: &testRoot16,
				updatedLeaves:    &leaves16,
				merkleNodesSet:   &updatedNodes,
				storeSignedRoot:  nil,
				signer:           signer16,
			},
			errStr: "commit",
		},
		{
			// TODO: We used a perfect tree size so this isn't testing code that loads the compact Merkle
			// tree. This will be done later as it's planned to refactor it anyway.
			desc: "sequence-leaf-16",
			params: testParameters{
				logID:            154035,
				writeRevision:    testRoot16.TreeRevision + 1,
				dequeueLimit:     1,
				shouldCommit:     true,
				dequeuedLeaves:   []*trillian.LogLeaf{getLeaf42()},
				latestSignedRoot: &testRoot16,
				updatedLeaves:    &leaves16,
				merkleNodesSet:   &updatedNodes,
				storeSignedRoot:  &expectedSignedRoot,
				signer:           signer1,
			},
			wantCount: 1,
		},
	}

	for _, test := range tests {
		func() {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			qm := quota.NewMockManager(ctrl)
			test.params.qm = qm
			if test.wantCount > 0 {
				qm.EXPECT().PutTokens(gomock.Any(), test.wantCount, specs).Return(nil)
			}
			c, ctx := createTestContext(ctrl, test.params)

			got, err := c.sequencer.SequenceBatch(ctx, test.params.logID, 1, test.guardWindow, test.maxRootDuration)
			if err != nil {
				if test.errStr == "" {
					t.Errorf("SequenceBatch(%+v)=%v,%v; want _,nil", test.params, got, err)
				} else if !strings.Contains(err.Error(), test.errStr) || got != 0 {
					t.Errorf("SequenceBatch(%+v)=%v,%v; want 0, error with %q", test.params, got, err, test.errStr)
				}
				return
			}
			if got != test.wantCount {
				t.Errorf("SequenceBatch(%+v)=%v,nil; want %v,nil", test.params, got, test.wantCount)
			}
		}()
	}
}

func TestSequenceBatch_PutTokens(t *testing.T) {
	cryptoSigner, err := newSignerWithFixedSig(expectedSignedRoot.Signature)
	if err != nil {
		t.Fatalf("Failed to create test signer (%v)", err)
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Needed to create a signer
	hasher := rfc6962.DefaultHasher
	ts := util.NewFakeTimeSource(fakeTimeForTest)
	signer := crypto.NewSHA256Signer(cryptoSigner)

	// Needed for SequenceBatch calls
	const treeID int64 = 1234
	const limit = 1000
	const guardWindow = 10 * time.Second
	const maxRootDuration = 1 * time.Hour

	// Expected PutTokens specs
	specs := []quota.Spec{
		{Group: quota.Tree, Kind: quota.Read, TreeID: treeID},
		{Group: quota.Tree, Kind: quota.Write, TreeID: treeID},
		{Group: quota.Global, Kind: quota.Read},
		{Group: quota.Global, Kind: quota.Write},
	}

	oneHundredLeaves := make([]*trillian.LogLeaf, 100)
	for i := range oneHundredLeaves {
		oneHundredLeaves[i] = &trillian.LogLeaf{
			LeafValue: []byte(fmt.Sprintf("leaf-%v", i)),
		}
	}

	tests := []struct {
		desc                   string
		leaves                 []*trillian.LogLeaf
		quotaFactor            float64
		wantLeaves, wantTokens int
	}{
		{desc: "noLeaves"},
		{
			desc:       "singleLeaf",
			leaves:     []*trillian.LogLeaf{getLeaf42()},
			wantLeaves: 1,
			wantTokens: 1,
		},
		{
			desc:        "badFactor",
			leaves:      oneHundredLeaves,
			quotaFactor: 0.7, // factor <1 is normalized to 1
			wantLeaves:  100,
			wantTokens:  100,
		},
		{
			desc:        "factorOne",
			leaves:      oneHundredLeaves,
			quotaFactor: 1,
			wantLeaves:  100,
			wantTokens:  100,
		},
		{
			desc:        "10%-factor",
			leaves:      oneHundredLeaves,
			quotaFactor: 1.1,
			wantLeaves:  100,
			wantTokens:  110,
		},
	}

	any := gomock.Any()
	ctx := context.Background()
	for _, test := range tests {
		func() {
			if test.quotaFactor != 0 {
				defer func(qf float64) {
					QuotaIncreaseFactor = qf
				}(QuotaIncreaseFactor)
				QuotaIncreaseFactor = test.quotaFactor
			}

			// Correctness of operation is tested elsewhere. The focus here is the interaction
			// between Sequencer and quota.Manager.
			logTX := storage.NewMockLogTreeTX(ctrl)
			logTX.EXPECT().DequeueLeaves(any, any, any).Return(test.leaves, nil)
			logTX.EXPECT().LatestSignedLogRoot(any).Return(testRoot16, nil)
			logTX.EXPECT().WriteRevision().AnyTimes().Return(testRoot16.TreeRevision + 1)
			logTX.EXPECT().UpdateSequencedLeaves(any, any).AnyTimes().Return(nil)
			logTX.EXPECT().SetMerkleNodes(any, any).AnyTimes().Return(nil)
			logTX.EXPECT().StoreSignedLogRoot(any, any).AnyTimes().Return(nil)
			logTX.EXPECT().Commit().Return(nil)
			logTX.EXPECT().Close().Return(nil)
			logStorage := storage.NewMockLogStorage(ctrl)
			logStorage.EXPECT().BeginForTree(any, any).Return(logTX, nil)

			qm := quota.NewMockManager(ctrl)
			if test.wantTokens > 0 {
				qm.EXPECT().PutTokens(any, test.wantTokens, specs)
			}

			sequencer := NewSequencer(hasher, ts, logStorage, signer, nil /* mf */, qm)
			leaves, err := sequencer.SequenceBatch(ctx, treeID, limit, guardWindow, maxRootDuration)
			if err != nil {
				t.Errorf("%v: SequenceBatch() returned err = %v", test.desc, err)
				return
			}
			if leaves != test.wantLeaves {
				t.Errorf("%v: SequenceBatch() returned %v leaves, want = %v", test.desc, leaves, test.wantLeaves)
			}
		}()
	}
}

func TestSignRoot(t *testing.T) {
	signer0, err := newSignerWithFixedSig(expectedSignedRoot0.Signature)
	if err != nil {
		t.Fatalf("Failed to create test signer (%v)", err)
	}
	signer16, err := newSignerWithFixedSig(expectedSignedRoot16.Signature)
	if err != nil {
		t.Fatalf("Failed to create test signer (%v)", err)
	}
	signerErr, err := newSignerWithErr(errors.New("signerfailed"))
	if err != nil {
		t.Fatalf("Failed to create test signer (%v)", err)
	}
	var tests = []struct {
		desc   string
		params testParameters
		errStr string
	}{
		{
			desc: "begin-tx-fails",
			params: testParameters{
				logID:               154035,
				beginFails:          true,
				skipDequeue:         true,
				skipStoreSignedRoot: true,
			},
			errStr: "TX",
		},
		{
			desc: "sign-latest-root-fails",
			params: testParameters{
				logID:                 154035,
				writeRevision:         testRoot16.TreeRevision + 1,
				dequeueLimit:          1,
				latestSignedRoot:      &testRoot16,
				latestSignedRootError: errors.New("root"),
				skipDequeue:           true,
				skipStoreSignedRoot:   true,
			},
			errStr: "root",
		},
		{
			desc: "signer-fails",
			params: testParameters{
				logID:               154035,
				writeRevision:       testRoot16.TreeRevision + 1,
				dequeueLimit:        1,
				latestSignedRoot:    &testRoot16,
				storeSignedRoot:     nil,
				signer:              signerErr,
				skipDequeue:         true,
				skipStoreSignedRoot: true,
			},
			errStr: "signer",
		},
		{
			desc: "store-root-fail",
			params: testParameters{
				logID:                154035,
				writeRevision:        testRoot16.TreeRevision + 1,
				latestSignedRoot:     &testRoot16,
				storeSignedRoot:      nil,
				storeSignedRootError: errors.New("storesignedroot"),
				signer:               signer16,
				skipDequeue:          true,
			},
			errStr: "storesignedroot",
		},
		{
			desc: "root-commit-fail",
			params: testParameters{
				logID:            154035,
				writeRevision:    testRoot16.TreeRevision + 1,
				shouldCommit:     true,
				commitFails:      true,
				commitError:      errors.New("commit"),
				latestSignedRoot: &testRoot16,
				storeSignedRoot:  nil,
				signer:           signer16,
				skipDequeue:      true,
			},
			errStr: "commit",
		},
		{
			desc: "existing-root",
			params: testParameters{
				logID:            154035,
				writeRevision:    testRoot16.TreeRevision + 1,
				latestSignedRoot: &testRoot16,
				storeSignedRoot:  &expectedSignedRoot16,
				signer:           signer16,
				shouldCommit:     true,
				skipDequeue:      true,
			},
		},
		{
			desc: "no-existing-root",
			params: testParameters{
				logID:            154035,
				writeRevision:    testRoot16.TreeRevision + 1,
				latestSignedRoot: &trillian.SignedLogRoot{},
				storeSignedRoot:  &expectedSignedRoot0,
				signer:           signer0,
				shouldCommit:     true,
				skipDequeue:      true,
			},
		},
	}

	for _, test := range tests {
		func() {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			c, ctx := createTestContext(ctrl, test.params)
			err := c.sequencer.SignRoot(ctx, test.params.logID)
			if test.errStr != "" {
				if err == nil {
					t.Errorf("SignRoot(%+v)=nil; want error with %q", test.params, test.errStr)
				} else if !strings.Contains(err.Error(), test.errStr) {
					t.Errorf("SignRoot(%+v)=%v; want error with %q", test.params, err, test.errStr)
				}
				return
			}
			if err != nil {
				t.Errorf("SignRoot(%+v)=%v; want nil", test.params, err)
			}
		}()
	}
}
