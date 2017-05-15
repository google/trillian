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

package election

import "sync"

// Election is a (flawed) simulated mastership election.
type Election struct {
	mu     sync.RWMutex
	master []string
}

// IsMaster indicates whether the given name is master.
func (e *Election) IsMaster(who string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, m := range e.master {
		if m == who {
			return true
		}
	}
	return false
}

// Masters returns the current set of masters.  There should be only one, but
// bugs happen...
func (e *Election) Masters() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.master
}

// SetMaster sets a single master.
func (e *Election) SetMaster(who string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.master = []string{who}
}

// SetMasters sets multiple masters.
func (e *Election) SetMasters(who []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.master = who
}
