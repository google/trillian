package hammer

import (
	"bytes"
	"context"
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

type intSelector func(prng *rand.Rand) int

// validReadOps performs valid read operations against the map.
type validReadOps struct {
	mc              *client.MapClient
	extraSize       uint
	chooseLeafCount intSelector
	prevContents    *testonly.VersionedMapContents // copies of earlier contents of the map
	smrs            *smrStash
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

	n := o.chooseLeafCount(prng) // can be zero
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
	if latest {
		leaves, err = o.mc.GetAndVerifyMapLeaves(ctx, indices)
		if err != nil {
			return fmt.Errorf("failed to GetAndVerifyMapLeaves: %v", err)
		}
	} else {
		leaves, err = o.mc.GetAndVerifyMapLeavesByRevision(ctx, contents.Rev, indices)
		if err != nil {
			return fmt.Errorf("failed to GetAndVerifyMapLeavesByRevision: %v", err)
		}
	}
	if err := contents.CheckContents(leaves, o.extraSize); err != nil {
		return fmt.Errorf("incorrect contents of leaves: %v", err)
	}
	glog.V(2).Infof("%d: got %d leaves", o.mc.MapID, len(leaves))
	return nil
}

// getSMR gets & verifies the latest SMR and pushes it onto the queue of seen SMRs.
func (o *validReadOps) getSMR(ctx context.Context, prng *rand.Rand) error {
	root, err := o.mc.GetAndVerifyLatestMapRoot(ctx)
	if err != nil {
		return fmt.Errorf("failed to get-smr: %v", err)
	}

	err = o.smrs.pushSMR(*root)
	if err != nil {
		return fmt.Errorf("got bad SMR in get-smr: %v", err)
	}
	glog.V(2).Infof("%d: got SMR(time=%q, rev=%d)", o.mc.MapID, time.Unix(0, int64(root.TimestampNanos)), root.Revision)

	if err := o.verify(root); err != nil {
		return err
	}
	return nil
}

// getSMRRev randomly chooses a previously seen SMR from the queue and checks that
// the map still returns the same SMR for this revision.
func (o *validReadOps) getSMRRev(ctx context.Context, prng *rand.Rand) error {
	which := prng.Intn(smrCount)
	smrRoot := o.smrs.previousSMR(which)
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
	want, err := mapContents.RootHash(o.mc.MapID, o.mc.Hasher)
	if err != nil {
		return err
	}
	if !bytes.Equal(root.RootHash, want) {
		return fmt.Errorf("unexpected root hash for revision %d: got %x, want %x", root.Revision, root.RootHash, want)
	}
	return nil
}
