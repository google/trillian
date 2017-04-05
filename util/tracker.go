// Copyright 2017 Google Inc. All Rights Reserved.
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

package util

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/golang/glog"
)

// MasterTracker tracks the current mastership state across multiple IDs.
type MasterTracker struct {
	mu          sync.RWMutex
	masterFor   map[int64]bool
	masterCount int
}

// NewMasterTracker creates a new MasterTracker instance to track the mastership
// status for the given set of ids.
func NewMasterTracker(ids []int64) *MasterTracker {
	mf := make(map[int64]bool)
	for _, id := range ids {
		mf[id] = false
	}
	return &MasterTracker{masterFor: mf}
}

// Set changes the tracked mastership status for the given id.  This method should
// be called exactly once for each state transition.
func (mt *MasterTracker) Set(id int64, val bool) {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	existing, ok := mt.masterFor[id]
	if ok && val == existing {
		glog.Warningf("toggle masterFor[%d] from %v to %v!", id, existing, val)
	}
	mt.masterFor[id] = val
	if val && !existing {
		mt.masterCount++
	} else if !val && existing {
		mt.masterCount--
	}
}

// Count returns the number of IDs for which we are currently master.
func (mt *MasterTracker) Count() int {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	return mt.masterCount
}

// Held returns a (sorted) list of the IDs for which we are currently master.
func (mt *MasterTracker) Held() []int64 {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	ids := make([]int64, 0, mt.masterCount)
	for id := range mt.masterFor {
		if mt.masterFor[id] {
			ids = append(ids, id)
		}
	}
	sort.Sort(int64arr(ids))
	return ids
}

// IDs returns a (sorted) list of the IDs that we are currently tracking.
func (mt *MasterTracker) IDs() []int64 {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	ids := make([]int64, 0, len(mt.masterFor))
	for id := range mt.masterFor {
		ids = append(ids, id)
	}
	sort.Sort(int64arr(ids))
	return ids
}

// String returns a textual decription of the current mastership status.
func (mt *MasterTracker) String() string {
	return HeldInfo(mt.Held(), mt.IDs())
}

// HeldInfo produces a textual description of the set of held IDs, compared
// to a complete set of IDs.
func HeldInfo(held []int64, ids []int64) string {
	result := ""
	prefix := ""
	for _, id := range ids {
		idStr := fmt.Sprintf("%d", id)
		show := strings.Repeat(".", len(idStr))
		for _, h := range held {
			if h == id {
				show = idStr
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

// Make int64 slice sortable:
type int64arr []int64

// Len returns length
func (a int64arr) Len() int {
	return len(a)
}

// Swap swaps
func (a int64arr) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

// Less compares
func (a int64arr) Less(i, j int) bool {
	return a[i] < a[j]
}
