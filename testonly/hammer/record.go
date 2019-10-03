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
	"github.com/google/trillian/types"
)

const (
	// How many SMRs to hold on to.
	smrCount = 30
)

// smrStash provides thread-safe access to an ordered, bounded queue of previously
// witnessed SMRs
type smrStash struct {
	mu sync.RWMutex

	// SMRs are arranged from later to earlier (so [0] is the most recent), and the
	// discovery of new SMRs will push older ones off the end. No SMR for a revision
	// will be stored more than once.
	smr [smrCount]*types.MapRootV1
}

// pushSMR ensures that the SMR is the latest revision and adds it to the queue of
// seen SMRs. If this SMR is the same as a previously pushed version, then it is
// equality checked and an error is returned if there is a difference.
func (s *smrStash) pushSMR(smr types.MapRootV1) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.smr[0] != nil {
		if s.smr[0].Revision > smr.Revision {
			return fmt.Errorf("pushSMR called with stale root. Received revision %d, already had revision %d", smr.Revision, s.smr[0].Revision)
		} else if s.smr[0].Revision == smr.Revision {
			if !reflect.DeepEqual(s.smr[0], &smr) {
				return fmt.Errorf("pushSMR witnessed different SMRs for revision=%d. Had %+v, received %+v", smr.Revision, s.smr[0], smr)
			}
			// Roots are equal, so no need to push on the same root twice
			return nil
		}
	}

	glog.V(2).Infof("adding new SMR: %+v", smr)
	// Shuffle earlier SMRs along.
	for i := smrCount - 1; i > 0; i-- {
		s.smr[i] = s.smr[i-1]
	}

	s.smr[0] = &smr
	return nil
}

func (s *smrStash) previousSMR(which int) *types.MapRootV1 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.smr[which]
}
