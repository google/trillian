package election2

import (
	"context"
	"errors"
	"sync"

	"github.com/google/trillian/util/election"
)

// ElectionAdapter implements legacy election interface using election2.
// TODO(pavelkalinnikov): Drop it together with the legacy interface.
type ElectionAdapter struct {
	e      Election
	mu     sync.RWMutex
	cancel context.CancelFunc
	done   <-chan struct{}
}

// NewAdapter returns an adapter to the old election interface.
func NewAdapter(e Election) election.MasterElection {
	return &ElectionAdapter{e: e}
}

// Start kicks off this instance's participation in master election, but really
// does nothing.
func (a *ElectionAdapter) Start(ctx context.Context) error {
	return nil
}

// WaitForMastership blocks until the current instance is the master.
func (a *ElectionAdapter) WaitForMastership(ctx context.Context) error {
	if err := a.e.Await(ctx); err != nil {
		return err
	}
	return a.watch(ctx)
}

// IsMaster returns whether the current instance is the master.
func (a *ElectionAdapter) IsMaster(ctx context.Context) (bool, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.done == nil { // WaitForMastership was not called, so not watching.
		return false, nil
	}
	select {
	case <-a.done: // Mastership context is closed.
		return false, nil
	default:
	}
	return true, nil
}

// Resign releases mastership, and stops participating in election.
func (a *ElectionAdapter) Resign(ctx context.Context) error {
	a.unwatch()
	return a.e.Resign(ctx)
}

// Close releases the resources, and stops participating in election.
func (a *ElectionAdapter) Close(ctx context.Context) error {
	a.unwatch()
	return a.e.Close(ctx)
}

// GetCurrentMaster would return instance ID of the current elected master if
// it was implemented.
func (a *ElectionAdapter) GetCurrentMaster(ctx context.Context) (string, error) {
	return "", errors.New("not implemented")
}

func (a *ElectionAdapter) watch(ctx context.Context) error {
	cctx, cancel := context.WithCancel(ctx)
	mctx, err := a.e.WithMastership(cctx)
	if err != nil {
		cancel()
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cancel, a.done = cancel, mctx.Done()
	return nil
}

func (a *ElectionAdapter) unwatch() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
		a.cancel, a.done = nil, nil
	}
}
