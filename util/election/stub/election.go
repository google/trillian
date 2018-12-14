// Copyright 2018 Google Inc. All Rights Reserved.
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

// Package stub contains a MasterElection implementation for testing.
package stub

import (
	"context"
	"sync"
	"time"

	"github.com/google/trillian/util/clock"
	"github.com/google/trillian/util/election"
)

// Errors contains errors to be returned by each of MasterElection methods.
type Errors struct {
	Start     error
	Wait      error // WaitForMastership error.
	IsMaster  error
	Resign    error // ResignAndRestart error.
	Close     error
	GetMaster error // GetCurrentMaster error.
}

// ErrAll creates Errors containing the same err associated with each method.
func ErrAll(err error) *Errors {
	return &Errors{err, err, err, err, err, err}
}

// MasterElection implements election.MasterElection interface for testing.
type MasterElection struct {
	isMaster bool
	errs     Errors
	mu       sync.RWMutex
}

// NewMasterElection returns a new initialized MasterElection for testing.
func NewMasterElection(isMaster bool, errs *Errors) *MasterElection {
	me := &MasterElection{isMaster: isMaster}
	if errs != nil {
		me.errs = *errs
	}
	return me
}

// Update changes mastership status and errors returned by interface calls.
func (e *MasterElection) Update(isMaster bool, errs *Errors) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if errs == nil {
		errs = &Errors{}
	}
	e.isMaster, e.errs = isMaster, *errs
}

// Start returns the stored error for this call.
func (e *MasterElection) Start(context.Context) error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.errs.Start
}

// WaitForMastership blocks until this instance is master, or an error is
// supplied, or context is done.
func (e *MasterElection) WaitForMastership(ctx context.Context) error {
	for {
		is, err := func() (bool, error) {
			e.mu.Lock()
			defer e.mu.Unlock()
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			if e.errs.Wait != nil {
				return false, e.errs.Wait
			}
			return e.isMaster, nil
		}()
		if is || err != nil {
			return err
		}
		if err := clock.SleepContext(ctx, 5*time.Millisecond); err != nil {
			return err
		}
	}
}

// IsMaster returns the stored mastership status and error.
func (e *MasterElection) IsMaster(context.Context) (bool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.isMaster, e.errs.IsMaster
}

// Resign returns the stored error and resets mastership status if the error is
// nil.
func (e *MasterElection) Resign(context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.errs.Resign == nil {
		e.isMaster = false
	}
	return e.errs.Resign
}

// Close returns the stored error.
func (e *MasterElection) Close(context.Context) error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.errs.Close
}

// GetCurrentMaster returns the current master which is *this* instance, or
// error if not currently the master.
func (e *MasterElection) GetCurrentMaster(context.Context) (string, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.errs.GetMaster != nil {
		return "", e.errs.GetMaster
	}
	if e.isMaster {
		return "self", nil
	}
	return "", election.ErrNoMaster
}
