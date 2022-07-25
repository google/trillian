// Copyright 2016 Google LLC. All Rights Reserved.
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
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/trillian"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/types"
	"github.com/google/trillian/util/clock"
	"github.com/transparency-dev/merkle/compact"
	"github.com/transparency-dev/merkle/rfc6962"

	stestonly "github.com/google/trillian/storage/testonly"
	"github.com/google/trillian/storage/tree"
)

var (
	fakeTime = time.Date(2016, 5, 25, 10, 55, 5, 0, time.UTC)

	// These can be shared between tests as they're never modified
	testLeaf16Data = []byte("testdataforleaf")
	testLeaf16Hash = rfc6962.DefaultHasher.HashLeaf(testLeaf16Data)
	testLeaf16     = &trillian.LogLeaf{
		MerkleLeafHash:     testLeaf16Hash,
		LeafValue:          testLeaf16Data,
		ExtraData:          nil,
		LeafIndex:          16,
		IntegrateTimestamp: testonly.MustToTimestampProto(fakeTime),
	}
	testLeaf21 = &trillian.LogLeaf{
		MerkleLeafHash:     testLeaf16Hash,
		LeafValue:          testLeaf16Data,
		ExtraData:          nil,
		LeafIndex:          21,
		IntegrateTimestamp: testonly.MustToTimestampProto(fakeTime),
	}

	testRoot16 = &types.LogRootV1{
		TreeSize: 16,
		// RootHash can't be nil because that's how the sequencer currently
		// detects that there was no stored tree head.
		RootHash:       []byte{},
		TimestampNanos: uint64(fakeTime.Add(-10 * time.Millisecond).UnixNano()),
	}
	// TODO(pavelkalinnikov): Put a well-formed hash here. The current one is
	// taken from testRoot16 and retained for regression purposes.
	compactTree16 = []tree.Node{{ID: compact.NewNodeID(4, 0), Hash: []byte{}}}

	testSignedRoot16 = makeSLR(testRoot16)
	newSignedRoot16  = makeSLR(&types.LogRootV1{
		TimestampNanos: uint64(fakeTime.UnixNano()),
		TreeSize:       testRoot16.TreeSize,
		RootHash:       testRoot16.RootHash,
	})

	testRoot17 = &types.LogRootV1{
		TreeSize: 16,
		// RootHash can't be nil because that's how the sequencer currently
		// detects that there was no stored tree head.
		RootHash:       []byte{},
		TimestampNanos: uint64(fakeTime.UnixNano()),
	}
	testSignedRoot17 = makeSLR(testRoot17)

	testRoot18 = &types.LogRootV1{
		TreeSize: 16,
		// RootHash can't be nil because that's how the sequencer currently
		// detects that there was no stored tree head.
		RootHash:       []byte{},
		TimestampNanos: uint64(fakeTime.Add(10 * time.Millisecond).UnixNano()),
	}
	testSignedRoot18 = makeSLR(testRoot18)

	// These will be accepted in either order because of custom sorting in the mock
	updatedNodes = []tree.Node{
		{
			ID:   compact.NewNodeID(0, 16),
			Hash: testonly.MustDecodeBase64("L5Iyd7aFOVewxiRm29xD+EU+jvEo4RfufBijKdflWMk="),
		},
	}

	testRoot = &types.LogRootV1{
		RootHash:       []byte{71, 158, 195, 172, 164, 198, 185, 151, 99, 8, 213, 227, 191, 162, 39, 26, 185, 184, 172, 0, 75, 58, 127, 114, 90, 151, 71, 153, 131, 168, 47, 5},
		TimestampNanos: uint64(fakeTime.UnixNano()),
		TreeSize:       17,
	}
	testSignedRoot = makeSLR(testRoot)

	// TODO(pavelkalinnikov): Generate boilerplate structures, like the ones
	// below, in a more compact way.
	testRoot21 = &types.LogRootV1{
		TreeSize:       21,
		RootHash:       testonly.MustDecodeBase64("lfLXEAeBNB/zX1+97lInoqpnLJtX+AS/Ok0mwlWFpRc="),
		TimestampNanos: uint64(fakeTime.Add(-10 * time.Millisecond).UnixNano()),
	}
	testSignedRoot21 = makeSLR(testRoot21)
	// Nodes that will be loaded when updating the tree of size 21.
	compactTree21 = []tree.Node{
		{ID: compact.NewNodeID(4, 0), Hash: testonly.MustDecodeBase64("CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC=")},
		{ID: compact.NewNodeID(2, 4), Hash: testonly.MustDecodeBase64("BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB=")},
		{ID: compact.NewNodeID(0, 20), Hash: testonly.MustDecodeBase64("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")},
	}
	// Nodes that will be stored after updating the tree of size 21.
	updatedNodes21 = []tree.Node{
		{
			ID:   compact.NewNodeID(1, 10),
			Hash: testonly.MustDecodeBase64("S55qEsQMx90/eq1fSb87pYCB9WIYL7hBgiTY+B9LmPw="),
		},
		{
			ID:   compact.NewNodeID(0, 21),
			Hash: testonly.MustDecodeBase64("L5Iyd7aFOVewxiRm29xD+EU+jvEo4RfufBijKdflWMk="),
		},
	}
	// The new root after updating the tree of size 21.
	updatedRoot21 = &types.LogRootV1{
		RootHash:       testonly.MustDecodeBase64("1oUtLDlyOWXLHLAvL3NvWaO4D9kr0oQYScylDlgjey4="),
		TimestampNanos: uint64(fakeTime.UnixNano()),
		TreeSize:       22,
	}
	updatedSignedRoot21 = makeSLR(updatedRoot21)

	emptyRoot = &types.LogRootV1{
		TimestampNanos: uint64(fakeTime.Add(-10 * time.Millisecond).UnixNano()),
		TreeSize:       0,
		RootHash:       rfc6962.DefaultHasher.EmptyRoot(),
	}
	signedEmptyRoot        = makeSLR(emptyRoot)
	updatedSignedEmptyRoot = makeSLR(&types.LogRootV1{
		TimestampNanos: uint64(fakeTime.UnixNano()),
		TreeSize:       0,
		RootHash:       rfc6962.DefaultHasher.EmptyRoot(),
	})
)

func makeSLR(root *types.LogRootV1) *trillian.SignedLogRoot {
	logRoot, _ := root.MarshalBinary()
	return &trillian.SignedLogRoot{LogRoot: logRoot}
}

// testParameters bundles up values needed for setting mock expectations in tests
type testParameters struct {
	logID int64

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

	merkleNodesGet      *[]tree.Node
	merkleNodesGetError error

	updatedLeaves      *[]*trillian.LogLeaf
	updatedLeavesError error

	merkleNodesSet      *[]tree.Node
	merkleNodesSetError error

	skipStoreSignedRoot  bool
	storeSignedRoot      *trillian.SignedLogRoot
	storeSignedRootError error

	overrideDequeueTime *time.Time

	// qm is the quota.Manager to be used. If nil, quota.Noop() is used instead.
	qm quota.Manager
}

// Tests get their own mock context so they can be run in parallel safely
type testContext struct {
	mockTx      *storage.MockLogTreeTX
	fakeStorage storage.LogStorage
	qm          quota.Manager
	timeSource  clock.TimeSource
}

// This gets modified so tests need their own copies
func getLeaf42() *trillian.LogLeaf {
	return &trillian.LogLeaf{
		MerkleLeafHash: testLeaf16Hash,
		LeafValue:      testLeaf16Data,
		ExtraData:      nil,
		LeafIndex:      42,
	}
}

func createTestContext(ctrl *gomock.Controller, params testParameters) (testContext, context.Context) {
	fakeStorage := &stestonly.FakeLogStorage{}
	mockTx := storage.NewMockLogTreeTX(ctrl)

	if params.beginFails {
		fakeStorage.TXErr = errors.New("TX")
	} else {
		mockTx.EXPECT().Close()
		fakeStorage.TX = mockTx
	}

	if params.shouldCommit {
		if !params.commitFails {
			mockTx.EXPECT().Commit(gomock.Any()).Return(nil)
		} else {
			mockTx.EXPECT().Commit(gomock.Any()).Return(params.commitError)
		}
	}
	// Close is always called, regardless of explicit commits
	mockTx.EXPECT().Close().AnyTimes().Return(nil)

	if !params.skipDequeue {
		if params.overrideDequeueTime != nil {
			mockTx.EXPECT().DequeueLeaves(gomock.Any(), params.dequeueLimit, *params.overrideDequeueTime).Return(params.dequeuedLeaves, params.dequeuedError)
		} else {
			mockTx.EXPECT().DequeueLeaves(gomock.Any(), params.dequeueLimit, fakeTime).Return(params.dequeuedLeaves, params.dequeuedError)
		}
	}

	if params.latestSignedRoot != nil {
		mockTx.EXPECT().LatestSignedLogRoot(gomock.Any()).Return(params.latestSignedRoot, params.latestSignedRootError)
	}

	if params.merkleNodesGet != nil {
		ids := make([]compact.NodeID, 0, len(*params.merkleNodesGet))
		for _, node := range *params.merkleNodesGet {
			ids = append(ids, node.ID)
		}
		mockTx.EXPECT().GetMerkleNodes(gomock.Any(), ids).Return(*params.merkleNodesGet, params.merkleNodesGetError)
	}

	if params.updatedLeaves != nil {
		mockTx.EXPECT().UpdateSequencedLeaves(gomock.Any(), cmpMatcher{*params.updatedLeaves}).Return(params.updatedLeavesError)
	}

	if params.merkleNodesSet != nil {
		mockTx.EXPECT().SetMerkleNodes(gomock.Any(), stestonly.NodeSet(*params.merkleNodesSet)).Return(params.merkleNodesSetError)
	}

	if !params.skipStoreSignedRoot {
		if params.storeSignedRoot != nil {
			mockTx.EXPECT().StoreSignedLogRoot(gomock.Any(), cmpMatcher{params.storeSignedRoot}).Return(params.storeSignedRootError)
		} else {
			// At the moment if we're going to fail the operation we accept any root
			mockTx.EXPECT().StoreSignedLogRoot(gomock.Any(), gomock.Any()).Return(params.storeSignedRootError)
		}
	}

	qm := params.qm
	if qm == nil {
		qm = quota.Noop()
	}
	InitMetrics(nil)
	return testContext{mockTx: mockTx, fakeStorage: fakeStorage, qm: qm, timeSource: clock.NewFake(fakeTime)}, context.Background()
}

// Tests for sequencer. Currently relies on storage mocks.
// TODO(pavelkalinnikov): Consider using real in-memory storage.

func TestIntegrateBatch(t *testing.T) {
	leaves16 := []*trillian.LogLeaf{testLeaf16}
	guardWindow := time.Second * 10
	expectedCutoffTime := fakeTime.Add(-guardWindow)
	noLeaves := []*trillian.LogLeaf{}
	noNodes := []tree.Node{}
	specs := []quota.Spec{
		{Group: quota.Tree, Kind: quota.Read, TreeID: 154035},
		{Group: quota.Tree, Kind: quota.Write, TreeID: 154035},
		{Group: quota.Global, Kind: quota.Read},
		{Group: quota.Global, Kind: quota.Write},
	}

	tests := []struct {
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
				latestSignedRoot:    testSignedRoot16,
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
				latestSignedRoot:    testSignedRoot16,
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
				latestSignedRoot: testSignedRoot16,
				dequeuedLeaves:   noLeaves,
				merkleNodesGet:   &compactTree16,
				updatedLeaves:    &noLeaves,
				merkleNodesSet:   &noNodes,
				storeSignedRoot:  newSignedRoot16,
			},
			maxRootDuration: 9 * time.Millisecond,
		},
		{
			desc: "nothing-queued-on-max",
			params: testParameters{
				logID:            154035,
				dequeueLimit:     1,
				shouldCommit:     true,
				latestSignedRoot: testSignedRoot16,
				dequeuedLeaves:   noLeaves,
				merkleNodesGet:   &compactTree16,
				updatedLeaves:    &noLeaves,
				merkleNodesSet:   &noNodes,
				storeSignedRoot:  newSignedRoot16,
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
				latestSignedRoot:    testSignedRoot16,
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
				latestSignedRoot:    testSignedRoot16,
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
				latestSignedRoot:      testSignedRoot16,
				latestSignedRootError: errors.New("root"),
				skipDequeue:           true,
				skipStoreSignedRoot:   true,
			},
			errStr: "root",
		},
		{
			desc: "get-merkle-nodes-fails",
			params: testParameters{
				logID:               154035,
				dequeueLimit:        1,
				dequeuedLeaves:      []*trillian.LogLeaf{getLeaf42()},
				latestSignedRoot:    testSignedRoot21,
				merkleNodesGet:      &compactTree21,
				merkleNodesGetError: errors.New("getmerklenodes"),
				skipStoreSignedRoot: true,
			},
			errStr: "getmerklenodes",
		},
		{
			desc: "update-seq-leaves-fails",
			params: testParameters{
				logID:               154035,
				dequeueLimit:        1,
				dequeuedLeaves:      []*trillian.LogLeaf{getLeaf42()},
				latestSignedRoot:    testSignedRoot16,
				merkleNodesGet:      &compactTree16,
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
				dequeueLimit:        1,
				dequeuedLeaves:      []*trillian.LogLeaf{getLeaf42()},
				latestSignedRoot:    testSignedRoot16,
				merkleNodesGet:      &compactTree16,
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
				dequeueLimit:         1,
				dequeuedLeaves:       []*trillian.LogLeaf{getLeaf42()},
				latestSignedRoot:     testSignedRoot16,
				merkleNodesGet:       &compactTree16,
				updatedLeaves:        &leaves16,
				merkleNodesSet:       &updatedNodes,
				storeSignedRoot:      nil,
				storeSignedRootError: errors.New("storesignedroot"),
			},
			errStr: "storesignedroot",
		},
		{
			desc: "commit-fails",
			params: testParameters{
				logID:            154035,
				dequeueLimit:     1,
				shouldCommit:     true,
				commitFails:      true,
				commitError:      errors.New("commit"),
				dequeuedLeaves:   []*trillian.LogLeaf{getLeaf42()},
				latestSignedRoot: testSignedRoot16,
				merkleNodesGet:   &compactTree16,
				updatedLeaves:    &leaves16,
				merkleNodesSet:   &updatedNodes,
				storeSignedRoot:  nil,
			},
			errStr: "commit",
		},
		{
			desc: "sequence-empty",
			params: testParameters{
				logID:            154035,
				dequeueLimit:     1,
				shouldCommit:     true,
				dequeuedLeaves:   noLeaves,
				latestSignedRoot: signedEmptyRoot,
				updatedLeaves:    &noLeaves,
				merkleNodesSet:   &noNodes,
				storeSignedRoot:  updatedSignedEmptyRoot,
			},
			maxRootDuration: 5 * time.Millisecond,
		},
		{
			desc: "sequence-leaf-16",
			params: testParameters{
				logID:            154035,
				dequeueLimit:     1,
				shouldCommit:     true,
				dequeuedLeaves:   []*trillian.LogLeaf{getLeaf42()},
				latestSignedRoot: testSignedRoot16,
				merkleNodesGet:   &compactTree16,
				updatedLeaves:    &leaves16,
				merkleNodesSet:   &updatedNodes,
				storeSignedRoot:  testSignedRoot,
			},
			wantCount: 1,
		},
		{
			desc: "sequence-leaf-21",
			params: testParameters{
				logID:            154035,
				dequeueLimit:     1,
				shouldCommit:     true,
				dequeuedLeaves:   []*trillian.LogLeaf{getLeaf42()},
				latestSignedRoot: testSignedRoot21,
				merkleNodesGet:   &compactTree21,
				updatedLeaves:    &[]*trillian.LogLeaf{testLeaf21},
				merkleNodesSet:   &updatedNodes21,
				storeSignedRoot:  updatedSignedRoot21,
			},
			wantCount: 1,
		},
		{
			desc: "prev-root-timestamp-equals",
			params: testParameters{
				logID:               154035,
				dequeueLimit:        1,
				dequeuedLeaves:      []*trillian.LogLeaf{getLeaf42()},
				latestSignedRoot:    testSignedRoot17,
				merkleNodesGet:      &compactTree16,
				updatedLeaves:       &leaves16,
				merkleNodesSet:      &updatedNodes,
				skipStoreSignedRoot: true,
			},
			errStr: "refusing to sign root with timestamp earlier than previous root (1464173705000000000 <= 1464173705000000000)",
		},
		{
			desc: "prev-root-timestamp-in-future",
			params: testParameters{
				logID:               154035,
				dequeueLimit:        1,
				dequeuedLeaves:      []*trillian.LogLeaf{getLeaf42()},
				latestSignedRoot:    testSignedRoot18,
				merkleNodesGet:      &compactTree16,
				updatedLeaves:       &leaves16,
				merkleNodesSet:      &updatedNodes,
				skipStoreSignedRoot: true,
			},
			errStr: "refusing to sign root with timestamp earlier than previous root (1464173705000000000 <= 1464173705010000000)",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			qm := quota.NewMockManager(ctrl)
			test.params.qm = qm
			if test.wantCount > 0 {
				qm.EXPECT().PutTokens(gomock.Any(), test.wantCount, specs).Return(nil)
			}
			c, ctx := createTestContext(ctrl, test.params)
			tree := &trillian.Tree{TreeId: test.params.logID, TreeType: trillian.TreeType_LOG}

			got, err := IntegrateBatch(ctx, tree, 1, test.guardWindow, test.maxRootDuration, c.timeSource, c.fakeStorage, c.qm)
			if err != nil {
				if test.errStr == "" {
					t.Errorf("IntegrateBatch(%+v)=%v,%v; want _,nil", test.params, got, err)
				} else if !strings.Contains(err.Error(), test.errStr) || got != 0 {
					t.Errorf("IntegrateBatch(%+v)=%v,%v; want 0, error with %q", test.params, got, err, test.errStr)
				}
				return
			}
			if got != test.wantCount {
				t.Errorf("IntegrateBatch(%+v)=%v,nil; want %v,nil", test.params, got, test.wantCount)
			}
		})
	}
}

func TestIntegrateBatch_PutTokens(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Needed to create a signer
	ts := clock.NewFake(fakeTime)

	// Needed for IntegrateBatch calls
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

	InitMetrics(nil)

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
			logTX.EXPECT().LatestSignedLogRoot(any).Return(testSignedRoot16, nil)
			if len(test.leaves) != 0 {
				logTX.EXPECT().GetMerkleNodes(any, any).Return(compactTree16, nil)
			}
			logTX.EXPECT().UpdateSequencedLeaves(any, any).AnyTimes().Return(nil)
			logTX.EXPECT().SetMerkleNodes(any, any).AnyTimes().Return(nil)
			logTX.EXPECT().StoreSignedLogRoot(any, any).AnyTimes().Return(nil)
			logTX.EXPECT().Commit(gomock.Any()).Return(nil)
			logTX.EXPECT().Close().Return(nil)
			logStorage := &stestonly.FakeLogStorage{TX: logTX}

			qm := quota.NewMockManager(ctrl)
			if test.wantTokens > 0 {
				qm.EXPECT().PutTokens(any, test.wantTokens, specs)
			}

			tree := &trillian.Tree{TreeId: treeID, TreeType: trillian.TreeType_LOG}
			leaves, err := IntegrateBatch(ctx, tree, limit, guardWindow, maxRootDuration, ts, logStorage, qm)
			if err != nil {
				t.Errorf("%v: IntegrateBatch() returned err = %v", test.desc, err)
				return
			}
			if leaves != test.wantLeaves {
				t.Errorf("%v: IntegrateBatch() returned %v leaves, want = %v", test.desc, leaves, test.wantLeaves)
			}
		}()
	}
}
