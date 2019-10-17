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
	"sort"
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

// sharedState shares details of what has been written to and read from the Map
// between different workers running in the same process. The same interface
// could be implemented by something which shared these details with a remote
// process via gRPC if that was desirable.
// The goal is that VersionedMapContents can be subsumed into this class - there
// should be no reason to address it directly. For now, this class coordinates
// writes, reads, and local state such that VersionedMapContents always contains
// revisions which are no further ahead than the most recent published SMR.
type sharedState struct {
	contents *testonly.VersionedMapContents

	mu sync.RWMutex // Guards everything below.

	pendingWrites map[uint64][]*trillian.MapLeaf

	// SMRs are arranged from later to earlier (so [0] is the most recent), and the
	// discovery of new SMRs will push older ones off the end. No SMR for a revision
	// will be stored more than once.
	smrs [smrCount]*types.MapRootV1
}

func newSharedState() *sharedState {
	return &sharedState{
		contents:      &testonly.VersionedMapContents{},
		pendingWrites: make(map[uint64][]*trillian.MapLeaf),
	}
}

func (g *sharedState) getLastReadRev() (rev uint64, found bool) {
	if lastRoot := g.previousSMR(0); lastRoot != nil {
		return lastRoot.Revision, true
	}
	return 0, false
}

// proposeLeaves should be called *before* writing a new map revision. The revision
// will be available to readers only once advertiseSMR has been called with a root
// that is >= the revision of this write.
func (g *sharedState) proposeLeaves(rev uint64, leaves []*trillian.MapLeaf) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.pendingWrites[rev] = leaves
	return nil
}

func (g *sharedState) advertiseSMR(smr types.MapRootV1) error {
	update, err := func() (bool, error) {
		g.mu.RLock()
		defer g.mu.RUnlock()

		if g.smrs[0] == nil || g.smrs[0].Revision < smr.Revision {
			return true, nil
		}
		pos := sort.Search(smrCount, func(i int) bool {
			return g.smrs[i] == nil || g.smrs[i].Revision <= smr.Revision
		})
		if known := g.smrs[pos]; known != nil && known.Revision == smr.Revision && !reflect.DeepEqual(known, &smr) {
			return false, fmt.Errorf("SMR mismatch at revision %d: had %+v, got %+v", smr.Revision, known, smr)
		}
		return false, nil
	}()
	if err != nil {
		return err
	}

	if update {
		g.mu.Lock()
		defer g.mu.Unlock()

		// update is a suggestion. We need to confirm the condition holds now we have write lock.
		if g.smrs[0] != nil && g.smrs[0].Revision >= smr.Revision {
			return nil
		}

		var prevRev uint64 // The last revision committed to contents.
		if g.smrs[0] != nil {
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
				return fmt.Errorf("found SMR(r=%d), but failed to find pending write for r=%d", smr.Revision, i)
			}
		}
	}

	return nil
}

func (g *sharedState) previousSMR(which int) *types.MapRootV1 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.smrs[which]
}
