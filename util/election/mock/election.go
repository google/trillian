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
)

// MasterElection implements election.MasterElection interface for testing.
type MasterElection struct {
	StartErr, WaitErr, IsMasterErr, ResignErr, CloseErr error

	Master bool

	mu   sync.RWMutex
	cond *sync.Cond
}

func NewMasterElection(isMaster bool) *MasterElection {
	res := &MasterElection{Master: isMaster}
	res.Init()
	return res
}

func (e *MasterElection) Init() {
	e.cond = sync.NewCond(&e.mu)
}

func (e *MasterElection) Update(isMaster bool, err error) {
	e.UpdateWithFn(func(e *MasterElection) {
		e.Master = isMaster
		e.StartErr, e.WaitErr, e.IsMasterErr, e.ResignErr, e.CloseErr = err, err, err, err, err
	})
}

func (e *MasterElection) UpdateWithFn(fn func(*MasterElection)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	fn(e)
	e.cond.Broadcast()
}

// Ping can be used to trigger context canceling in WaitForMastership which is
// not supported in sync.Cond unfortunately. Normally, implementations will
// support canceling in WaitForMastership properly.
func (e *MasterElection) Ping() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cond.Broadcast()
}

func (e *MasterElection) Start(context.Context) error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.StartErr
}

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

func (e *MasterElection) IsMaster(context.Context) (bool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.Master, e.IsMasterErr
}

func (e *MasterElection) ResignAndRestart(context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.ResignErr == nil {
		e.Master = false
	}
	return e.ResignErr
}

func (e *MasterElection) Close(context.Context) error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.CloseErr
}
