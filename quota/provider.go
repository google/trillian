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

package quota

import (
	"flag"
	"fmt"
	"sync"
)

var (
	// System is a flag specifying which quota system is in use.
	// TODO(RJPercival): Only the "noop" quota system is guaranteed to be
	// present, so should default to that.
	System = flag.String("quota_system", "mysql", fmt.Sprintf("Quota system to use. One of: %v", quotaSystems()))

	qpMu     sync.RWMutex
	qpByName map[string]NewManagerFunc
)

// NewManagerFunc is the signature of a function which can be registered
// to provide instances of a quota manager.
type NewManagerFunc func() (Manager, error)

// RegisterProvider registers a function that provides Manager instances.
func RegisterProvider(name string, qp NewManagerFunc) error {
	qpMu.Lock()
	defer qpMu.Unlock()

	if qpByName == nil {
		qpByName = make(map[string]NewManagerFunc)
	}

	_, exists := qpByName[name]
	if exists {
		return fmt.Errorf("quota provider %v already registered", name)
	}
	qpByName[name] = qp
	return nil
}

// quotaSystems returns a slice of registered quota system names.
func quotaSystems() []string {
	qpMu.RLock()
	defer qpMu.RUnlock()

	r := []string{}
	for k := range qpByName {
		r = append(r, k)
	}

	return r
}

// NewManagerFromFlags returns a Manager implementation as speficied by flag.
func NewManagerFromFlags() (Manager, error) {
	return NewManager(*System)
}

// NewManager returns a Manager implementation.
func NewManager(name string) (Manager, error) {
	qpMu.RLock()
	defer qpMu.RUnlock()

	f, exists := qpByName[name]
	if !exists {
		return nil, fmt.Errorf("unknown quota system: %v", name)
	}
	return f()
}
