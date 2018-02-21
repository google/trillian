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

package server

import (
	"fmt"
	"sync"

	"github.com/google/trillian/quota"
)

const (
	// QuotaNoop represents the noop quota implementation.
	QuotaNoop = "noop"
)

// NewQuotaManagerFunc is the signature of a function which can be registered
// to provide instances of a quota manager.
type NewQuotaManagerFunc func() (quota.Manager, error)

var (
	qpMu     sync.RWMutex
	qpOnce   sync.Once
	qpByName map[string]NewQuotaManagerFunc
)

func init() {
	if err := RegisterQuotaManager(QuotaNoop, func() (quota.Manager, error) {
		return quota.Noop(), nil
	}); err != nil {
		panic(err)
	}
}

// RegisterQuotaManager registers the provided QuotaManager.
func RegisterQuotaManager(name string, qp NewQuotaManagerFunc) error {
	qpMu.Lock()
	defer qpMu.Unlock()

	qpOnce.Do(func() {
		qpByName = make(map[string]NewQuotaManagerFunc)
	})

	_, exists := qpByName[name]
	if exists {
		return fmt.Errorf("quota provider %v already registered", name)
	}
	qpByName[name] = qp
	return nil
}

// QuotaSystems returns a slice of registered quota system names.
func QuotaSystems() []string {
	qpMu.RLock()
	defer qpMu.RUnlock()

	r := []string{}
	for k := range qpByName {
		r = append(r, k)
	}

	return r
}

// NewQuotaManager returns a quota.Manager implementation.
func NewQuotaManager(name string) (quota.Manager, error) {
	qpMu.RLock()
	defer qpMu.RUnlock()

	f, exists := qpByName[name]
	if !exists {
		return nil, fmt.Errorf("unknown quota system: %v", name)
	}
	return f()
}
