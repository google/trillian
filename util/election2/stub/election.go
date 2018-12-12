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

// Package stub contains an Election implementation for testing.
package stub

import (
	"context"
	"sync"
)

// Errors contains errors to be returned by each of Election methods.
type Errors struct {
	Await          error
	WithMastership error
	Resign         error
	Close          error
}

// ErrAll creates Errors containing the same err associated with each method.
func ErrAll(err error) *Errors {
	return &Errors{err, err, err, err}
}

// Election implements election2.Election interface for testing.
type Election struct {
	isMaster bool
	revision int
	errs     Errors
	mu       sync.RWMutex
	cond     sync.Cond
}

// NewElection returns a new initialized Election for testing.
func NewElection(isMaster bool, errs *Errors) *Election {
	me := &Election{isMaster: isMaster}
	if errs != nil {
		me.errs = *errs
	}
	return me
}

// Update changes mastership status and errors returned by interface calls.
func (e *Election) Update(isMaster bool, errs *Errors) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if errs == nil {
		errs = &Errors{}
	}
	e.isMaster, e.errs = isMaster, *errs
	e.revision++
	e.cond.Broadcast()
}

// Await blocks until this instance is master, an error is supplied, or the
// passed in context is done.
func (e *Election) Await(ctx context.Context) error {
	isMaster := func() (bool, error) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if err := e.errs.Await; err != nil {
			return false, err
		}
		return e.isMaster, nil
	}

	_, cancel := e.watchContext(ctx)
	defer cancel()

	e.mu.Lock()
	defer e.mu.Unlock()
	for {
		is, err := isMaster()
		if is || err != nil {
			return err
		}
		e.cond.Wait()
	}
}

// WithMastership returns mastership context, or an error if stored.
func (e *Election) WithMastership(ctx context.Context) (context.Context, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if err := e.errs.WithMastership; err != nil {
		return nil, err
	}
	cctx, cancel := e.watchContext(ctx)
	if !e.isMaster {
		cancel()
		return cctx, nil
	}
	rev := e.revision
	go func() {
		defer cancel()
		e.mu.RLock()
		defer e.mu.RUnlock()
		for e.isMaster && e.revision == rev && ctx.Err() != nil {
			e.cond.Wait()
		}
	}()
	return cctx, nil
}

// Resign resets mastership, or returns the stored error.
func (e *Election) Resign(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.errs.Resign; err != nil {
		return err
	}
	e.Update(false, &e.errs)
	return nil
}

// Close resigns, or returns the stored error.
func (e *Election) Close(ctx context.Context) error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if err := e.errs.Close; err != nil {
		return err
	}
	e.Update(false, &e.errs)
	return nil
}

func (e *Election) watchContext(ctx context.Context) (context.Context, context.CancelFunc) {
	cctx, cancel := context.WithCancel(ctx)
	go func() {
		defer cancel()
		<-cctx.Done()
		e.cond.Broadcast()
	}()
	return cctx, cancel
}
