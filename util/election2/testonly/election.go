// Copyright 2018 Google LLC. All Rights Reserved.
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

// Package testonly contains an Election implementation for testing.
package testonly

import (
	"context"
	"sync"

	"github.com/google/trillian/util/election2"
)

// Factory allows creating Election instances for testing.
var Factory factory

// Election implements election2.Election interface for testing.
type Election struct {
	isMaster bool
	revision int
	mu       sync.Mutex
	cond     *sync.Cond
}

// NewElection returns a new initialized Election for testing.
func NewElection() *Election {
	e := &Election{}
	e.cond = sync.NewCond(&e.mu)
	return e
}

// update updates this instance's mastership status. Must be called under lock.
func (e *Election) update(isMaster bool) {
	e.isMaster = isMaster
	e.revision++
	e.cond.Broadcast()
}

// Await sets this instance to be the master. It always succeeds. To imitate
// errors and/or blocking behavior use the Decorator type.
func (e *Election) Await(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.isMaster {
		e.update(true)
	}
	return nil
}

// WithMastership returns mastership context, which gets canceled if / when
// this instance is not / stops being the master.
func (e *Election) WithMastership(ctx context.Context) (context.Context, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.isMaster {
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		return cctx, nil
	}

	cctx, cancel := watchContext(ctx, &e.mu, e.cond) // Notify e.cond on ctx cancelation.
	rev := e.revision
	go func() { // Watch mastership and the context in the background.
		defer cancel()
		e.mu.Lock()
		defer e.mu.Unlock()
		for e.isMaster && e.revision == rev && ctx.Err() == nil {
			e.cond.Wait()
		}
	}()
	return cctx, nil
}

// Resign resets mastership.
func (e *Election) Resign(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.update(false)
	return nil
}

// Close resets mastership permanently.
func (e *Election) Close(ctx context.Context) error {
	return e.Resign(ctx)
}

func watchContext(ctx context.Context, l sync.Locker, cond *sync.Cond) (context.Context, context.CancelFunc) {
	cctx, cancel := context.WithCancel(ctx)
	go func() {
		defer cancel()
		<-cctx.Done()
		l.Lock() // Avoid racing with cond waiters on ctx status.
		defer l.Unlock()
		cond.Broadcast()
	}()
	return cctx, cancel
}

// factory allows creating Election instances.
type factory struct{}

// NewElection creates a new Election instance.
// TODO(pavelkalinnikov): Use resourceID in tests with multiple resources.
func (f factory) NewElection(ctx context.Context, resourceID string) (election2.Election, error) {
	return NewElection(), nil
}
