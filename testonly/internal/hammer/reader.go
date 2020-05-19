// Copyright 2019 Google Inc. All Rights Reserved.
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

package hammer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/client"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/types"
)

// validReadOps performs valid read operations against the map.
type validReadOps struct {
	mc                   *client.MapClient
	extraSize            uint
	minLeaves, maxLeaves int
	prevContents         *testonly.VersionedMapContents // copies of earlier contents of the map
	sharedState          *sharedState
}

func (o *validReadOps) getLeaves(ctx context.Context, prng *rand.Rand) error {
	return o.doGetLeaves(ctx, prng, true /*latest*/)
}

func (o *validReadOps) getLeavesRev(ctx context.Context, prng *rand.Rand) error {
	return o.doGetLeaves(ctx, prng, false /*latest*/)
}

func (o *validReadOps) doGetLeaves(ctx context.Context, prng *rand.Rand, latest bool) error {
	choices := []Choice{ExistingKey, NonexistentKey}

	if o.prevContents.Empty() {
		glog.V(3).Infof("%d: skipping get-leaves as no data yet", o.mc.MapID)
		return errSkip{}
	}
	var contents *testonly.MapContents
	if latest {
		contents = o.prevContents.LastCopy()
	} else {
		contents = o.prevContents.PickCopy(prng)
	}

	glog.V(2).Infof("%d: doGetLeaves(%v) for revision %d", o.mc.MapID, latest, contents.Rev)

	n := pickIntInRange(o.minLeaves, o.maxLeaves, prng) // can be zero
	indexMap := make(map[string]bool)
	for i := 0; i < n; i++ {
		choice := choices[prng.Intn(len(choices))]
		if contents.Empty() {
			choice = NonexistentKey
		}
		switch choice {
		case ExistingKey:
			key := contents.PickKey(prng)
			indexMap[string(key)] = true
		case NonexistentKey:
			key := testonly.TransparentHash("non-existent-key")
			indexMap[string(key)] = true
		}
	}
	indices := make([][]byte, 0, n)
	for k := range indexMap {
		indices = append(indices, []byte(k))
	}

	var err error
	var leaves []*trillian.MapLeaf
	var root *types.MapRootV1
	if latest {
		if leaves, root, err = o.mc.GetAndVerifyMapLeaves(ctx, indices); err != nil {
			return fmt.Errorf("failed to GetAndVerifyMapLeaves: %v", err)
		}
		if err := o.sharedState.advertiseSMR(*root); err != nil {
			return err
		}
	} else {
		if leaves, root, err = o.mc.GetAndVerifyMapLeavesByRevision(ctx, contents.Rev, indices); err != nil {
			return fmt.Errorf("failed to GetAndVerifyMapLeavesByRevision(%d): %v", contents.Rev, err)
		}
	}

	// We need to update contents to avoid a race condition in the `latest` case.
	if latest {
		contents = o.prevContents.PickRevision(root.Revision)
		if contents.Empty() {
			glog.V(3).Infof("%d: cannot find contents for revision %d to verify leaves", o.mc.MapID, root.Revision)
			return errSkip{}
		}
	}

	if err := contents.CheckContents(leaves, o.extraSize); err != nil {
		return fmt.Errorf("incorrect contents of leaves: %v", err)
	}
	glog.V(2).Infof("%d: got %d leaves", o.mc.MapID, len(leaves))

	if err = o.sharedState.advertiseSMR(*root); err != nil {
		return err
	}
	return nil
}

// getSMR gets & verifies the latest SMR and pushes it onto the queue of seen SMRs.
func (o *validReadOps) getSMR(ctx context.Context, prng *rand.Rand) error {
	root, err := o.mc.GetAndVerifyLatestMapRoot(ctx)
	if err != nil {
		return fmt.Errorf("failed to get-smr: %v", err)
	}

	if err := o.sharedState.advertiseSMR(*root); err != nil {
		return fmt.Errorf("got bad SMR in get-smr: %v", err)
	}
	glog.V(2).Infof("%d: got SMR(time=%q, rev=%d)", o.mc.MapID, time.Unix(0, int64(root.TimestampNanos)), root.Revision)

	return o.verify(root)
}

// getSMRRev randomly chooses a previously seen SMR from the queue and checks that
// the map still returns the same SMR for this revision.
func (o *validReadOps) getSMRRev(ctx context.Context, prng *rand.Rand) error {
	which := prng.Intn(smrCount)
	smrRoot := o.sharedState.previousSMR(which)
	if smrRoot == nil {
		glog.V(3).Infof("%d: skipping get-smr-rev as no earlier SMR", o.mc.MapID)
		return errSkip{}
	}
	rev := int64(smrRoot.Revision)

	root, err := o.mc.GetAndVerifyMapRootByRevision(ctx, rev)
	if err != nil {
		return fmt.Errorf("failed to get-smr-rev(@%d): %v", rev, err)
	}
	glog.V(2).Infof("%d: got SMR(time=%q, rev=%d)", o.mc.MapID, time.Unix(0, int64(root.TimestampNanos)), root.Revision)

	if !reflect.DeepEqual(root, smrRoot) {
		return fmt.Errorf("get-smr-rev(@%d)=%+v, want %+v", rev, root, smrRoot)
	}

	return nil
}

func (o *validReadOps) verify(root *types.MapRootV1) error {
	mapContents := o.prevContents.PickRevision(root.Revision)
	if root.Revision > 1 && mapContents == nil {
		return fmt.Errorf("unexpected missing previous contents for revision %d", root.Revision)
	}
	want, err := mapContents.RootHash(o.mc.MapID, o.mc.Hasher)
	if err != nil {
		return err
	}
	if !bytes.Equal(root.RootHash, want) {
		return fmt.Errorf("unexpected root hash for revision %d: got %x, want %x", root.Revision, root.RootHash, want)
	}
	return nil
}

// invalidReadOps performs invalid read operations against the map.
type invalidReadOps struct {
	mapID        int64
	client       trillian.TrillianMapClient
	prevContents *testonly.VersionedMapContents // copies of earlier contents of the map
	sharedState  *sharedState
}

func (o *invalidReadOps) getLeaves(ctx context.Context, prng *rand.Rand) error {
	key := testonly.TransparentHash("..invalid-size")
	req := &trillian.GetMapLeavesRequest{
		MapId: o.mapID,
		Index: [][]byte{key[2:]},
	}
	rsp, err := o.client.GetLeaves(ctx, req)
	if err == nil {
		return fmt.Errorf("unexpected success: get-leaves(MalformedKey: %+v): %+v", req, rsp.MapRoot)
	}
	glog.V(2).Infof("%d: expected failure: get-leaves(MalformedKey: %+v): %+v", o.mapID, req, rsp)
	return nil
}

func (o *invalidReadOps) getLeavesRev(ctx context.Context, prng *rand.Rand) error {
	choices := []Choice{MalformedKey, RevTooBig, RevIsNegative}

	req := &trillian.GetMapLeavesByRevisionRequest{MapId: o.mapID}
	contents := o.prevContents.LastCopy()
	choice := choices[prng.Intn(len(choices))]

	rev := int64(0)
	var index []byte
	if contents.Empty() {
		// No contents so we can't choose a key
		choice = MalformedKey
	} else {
		rev = contents.Rev
		index = contents.PickKey(prng)
	}
	switch choice {
	case MalformedKey:
		key := testonly.TransparentHash("..invalid-size")
		req.Index = [][]byte{key[2:]}
		req.Revision = rev
	case RevTooBig:
		req.Index = [][]byte{index}
		req.Revision = rev + invalidStretch
	case RevIsNegative:
		req.Index = [][]byte{index}
		req.Revision = -rev - invalidStretch
	}
	rsp, err := o.client.GetLeavesByRevision(ctx, req)
	if err == nil {
		return fmt.Errorf("unexpected success: get-leaves-rev(%v: %+v): %+v", choice, req, rsp.MapRoot)
	}
	glog.V(2).Infof("%d: expected failure: get-leaves-rev(%v: %+v): %+v", o.mapID, choice, req, rsp)
	return nil
}

func (o *invalidReadOps) getSMRRev(ctx context.Context, prng *rand.Rand) error {
	choices := []Choice{RevTooBig, RevIsNegative}

	rev := latestRevision
	contents := o.prevContents.LastCopy()
	if contents != nil {
		rev = contents.Rev
	}

	choice := choices[prng.Intn(len(choices))]

	switch choice {
	case RevTooBig:
		rev += invalidStretch
	case RevIsNegative:
		rev = -invalidStretch
	}
	req := trillian.GetSignedMapRootByRevisionRequest{MapId: o.mapID, Revision: rev}
	rsp, err := o.client.GetSignedMapRootByRevision(ctx, &req)
	if err == nil {
		return fmt.Errorf("unexpected success: get-smr-rev(%v: @%d): %+v", choice, rev, rsp.MapRoot)
	}
	glog.V(2).Infof("%d: expected failure: get-smr-rev(%v: @%d): %+v", o.mapID, choice, rev, rsp)
	return nil
}

func (o *invalidReadOps) getSMR(ctx context.Context, prng *rand.Rand) error {
	return errors.New("no invalid request possible for getSMR")
}
