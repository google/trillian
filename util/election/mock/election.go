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

// Package mock contains a mock MasterElection implementation.
package mock

import (
	"context"
	"sync"

	"github.com/google/trillian/util/election"
)

// MasterElection implements election.MasterElection interface for testing.
type MasterElection struct {
	StartErr, WaitErr, IsMasterErr, ResignErr, CloseErr error

	Master bool

	mu   sync.RWMutex
	cond *sync.Cond
}

// NewMasterElection returns a new initialized MasterElection for testing.
func NewMasterElection(isMaster bool) *MasterElection {
	res := &MasterElection{Master: isMaster}
	res.Init()
	return res
}

// Init initializes the MasterElection.
func (e *MasterElection) Init() {
	e.cond = sync.NewCond(&e.mu)
}

// Update changes mastership status and mocked error for all interface calls.
func (e *MasterElection) Update(isMaster bool, err error) {
	e.UpdateWithFn(func(e *MasterElection) {
		e.Master = isMaster
		e.StartErr, e.WaitErr, e.IsMasterErr, e.ResignErr, e.CloseErr = err, err, err, err, err
	})
}

// UpdateWithFn updates the MasterElection with the provided functor.
func (e *MasterElection) UpdateWithFn(fn func(*MasterElection)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	fn(e)
	e.cond.Broadcast()
}

// Ping triggers context canceling in WaitForMastership if the latter is
// blocked. This is due to sync.Cond being unable to catch context canceling.
// Non-test implementations normally support canceling in WaitForMastership.
func (e *MasterElection) Ping() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cond.Broadcast()
}

// Start returns the stored error for this call.
func (e *MasterElection) Start(context.Context) error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.StartErr
}

// WaitForMastership blocks until this instance is master or, and error is
// supplied or context is done (triggered by explicitly calling Ping).
func (e *MasterElection) WaitForMastership(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for ctx.Err() == nil && !e.Master && e.WaitErr == nil {
		e.cond.Wait()
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return e.WaitErr
}

// IsMaster returns the stored mastership status and error.
func (e *MasterElection) IsMaster(context.Context) (bool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.Master, e.IsMasterErr
}

// ResignAndRestart resets mastership status and returns the stored error.
func (e *MasterElection) ResignAndRestart(context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.ResignErr == nil {
		e.Master = false
	}
	return e.ResignErr
}

// Close returns the stored error.
func (e *MasterElection) Close(context.Context) error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.CloseErr
}

// GetCurrentMaster returns the current master which is *this* instance, or
// error if not currently the master.
func (e *MasterElection) GetCurrentMaster(context.Context) (string, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.Master {
		return "self", nil
	}
	return "", election.ErrNoLeader
}
