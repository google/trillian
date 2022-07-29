// Copyright 2017 Google LLC. All Rights Reserved.
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

package election

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"k8s.io/klog/v2"
)

// MasterTracker tracks the current mastership state across multiple IDs.
type MasterTracker struct {
	mu          sync.RWMutex
	masterFor   map[string]bool
	masterCount int
	notify      func(id string, isMaster bool)
}

// NewMasterTracker creates a new MasterTracker instance to track the
// mastership status for the given set of IDs.
func NewMasterTracker(ids []string, notify func(id string, isMaster bool)) *MasterTracker {
	mf := make(map[string]bool)
	for _, id := range ids {
		mf[id] = false
	}
	return &MasterTracker{masterFor: mf, notify: notify}
}

// Set changes the tracked mastership status for the given ID. This method
// should be called exactly once for each state transition.
func (mt *MasterTracker) Set(id string, isMaster bool) {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	wasMaster, ok := mt.masterFor[id]
	if ok && isMaster == wasMaster {
		klog.Warningf("toggle masterFor[%s] from %v to %v!", id, wasMaster, isMaster)
	}
	mt.masterFor[id] = isMaster
	if isMaster && !wasMaster {
		mt.masterCount++
	} else if !isMaster && wasMaster {
		mt.masterCount--
	}
	if mt.notify != nil {
		mt.notify(id, isMaster)
	}
}

// Count returns the number of IDs for which we are currently master.
func (mt *MasterTracker) Count() int {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	return mt.masterCount
}

// Held returns a (sorted) list of the IDs for which we are currently master.
func (mt *MasterTracker) Held() []string {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	ids := make([]string, 0, mt.masterCount)
	for id := range mt.masterFor {
		if mt.masterFor[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// IDs returns a (sorted) list of the IDs that we are currently tracking.
func (mt *MasterTracker) IDs() []string {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	ids := make([]string, 0, len(mt.masterFor))
	for id := range mt.masterFor {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// String returns a textual decription of the current mastership status.
func (mt *MasterTracker) String() string {
	return HeldInfo(mt.Held(), mt.IDs())
}

// HeldInfo produces a textual description of the set of held IDs, compared to
// a complete set of IDs.
func HeldInfo(held []string, ids []string) string {
	result := ""
	prefix := ""
	for _, id := range ids {
		show := strings.Repeat(".", len(id))
		for _, h := range held {
			if h == id {
				show = id
			}
			if h >= id {
				break
			}
		}
		result += fmt.Sprintf("%s%s", prefix, show)
		prefix = " "
	}
	return result
}
