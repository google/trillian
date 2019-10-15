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
	"fmt"
	"reflect"
	"sync"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/types"
)

const (
	// How many SMRs to hold on to.
	smrCount = 30
)

// gossipHub allows details of what has been written to and read from the Map
// to be shared between different workers running in the same process. The same
// interface could be implemented by something which shared these details with
// a remote process via gRPC if that was desirable.
// The goal is thatVersionedMapContents can be subsumed into this class - there
// should be no reason to address it directly. For now, this class coordinates
// writes, reads, and local state such that VersionedMapContents always contains
// revisions which are no further ahead than the most recent SMR published by the
// Map.
type gossipHub struct {
	contents *testonly.VersionedMapContents

	mu            sync.RWMutex
	pendingWrites map[uint64][]*trillian.MapLeaf

	// SMRs are arranged from later to earlier (so [0] is the most recent), and the
	// discovery of new SMRs will push older ones off the end. No SMR for a revision
	// will be stored more than once.
	smrs [smrCount]*types.MapRootV1
}

func newGossipHub() *gossipHub {
	return &gossipHub{
		contents:      &testonly.VersionedMapContents{},
		pendingWrites: make(map[uint64][]*trillian.MapLeaf),
	}
}

func (g *gossipHub) getLastReadRev() (rev uint64, found bool) {
	if lastRoot := g.previousSMR(0); lastRoot != nil {
		return lastRoot.Revision, true
	}
	return 0, false
}

// proposeLeaves should be called *before* writing a new map revision. The revision
// will be available to readers only once advertiseSMR has been called with a root
// that is >= the revision of this write.
func (g *gossipHub) proposeLeaves(rev uint64, leaves []*trillian.MapLeaf) error {
	if head, found := g.getLastReadRev(); found && rev <= head {
		return fmt.Errorf("Cannot create revision %d, latest SMR is %d", rev, head)
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	g.pendingWrites[rev] = leaves
	return nil
}

// TODO(mhutchinson): Make this way more tolerant so it accepts older SMRs and
// checks they are equivalent to previously seen versions if applicable.
func (g *gossipHub) advertiseSMR(smr types.MapRootV1) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	var prevRev uint64 // The last revision committed to contents.
	if g.smrs[0] != nil {
		if g.smrs[0].Revision > smr.Revision {
			return fmt.Errorf("pushSMR called with stale root. Received revision %d, already had revision %d", smr.Revision, g.smrs[0].Revision)
		} else if g.smrs[0].Revision == smr.Revision {
			if !reflect.DeepEqual(g.smrs[0], &smr) {
				return fmt.Errorf("pushSMR witnessed different SMRs for revision=%d. Had %+v, received %+v", smr.Revision, g.smrs[0], smr)
			}
			// Roots are equal, so no need to push on the same root twice
			return nil
		}
		prevRev = g.smrs[0].Revision
	}

	glog.V(2).Infof("adding new SMR: %+v", smr)
	// Shuffle earlier SMRs along.
	for i := smrCount - 1; i > 0; i-- {
		g.smrs[i] = g.smrs[i-1]
	}

	g.smrs[0] = &smr

	for i := prevRev + 1; i <= smr.Revision; i++ {
		if leaves, ok := g.pendingWrites[i]; ok {
			if _, err := g.contents.UpdateContentsWith(i, leaves); err != nil {
				return err
			}
			delete(g.pendingWrites, i)
		} else {
			return fmt.Errorf("Found SMR(r=%d), but failed to find pending write for r=%d", smr.Revision, i)
		}
	}
	return nil
}

func (g *gossipHub) previousSMR(which int) *types.MapRootV1 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.smrs[which]
}
