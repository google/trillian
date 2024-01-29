// Copyright 2023 Google LLC. All Rights Reserved.
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

// Package etcd provides an implementation of leader election based on a SQL database.
package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/trillian/util/election2"
	"k8s.io/klog/v2"
)

type leaderData struct {
	currentLeader string
	timestamp     time.Time
}

// Election is an implementation of election2.Election based on a SQL database.
type Election struct {
	db         *sql.DB
	instanceID string
	resourceID string

	currentLeader leaderData
	leaderLock    sync.Cond

	// If a channel is supplied with the cancel, it will be signalled when the election routine has exited.
	cancel           chan *chan error
	electionInterval time.Duration
}

var _ election2.Election = (*Election)(nil)

// Await implements election2.Election
func (e *Election) Await(ctx context.Context) error {
	e.leaderLock.L.Lock()
	defer e.leaderLock.L.Unlock()
	if e.cancel == nil {
		e.cancel = make(chan *chan error)
		go e.becomeLeaderLoop(context.Background(), e.cancel)
	}
	if e.currentLeader.currentLeader == e.instanceID {
		return nil
	}
	for e.currentLeader.currentLeader != e.instanceID {
		e.leaderLock.Wait()

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			klog.Infof("Waiting for leadership, %s is the leader at %s", e.currentLeader.currentLeader, e.currentLeader.timestamp)
		}
	}
	klog.Infof("%s became leader for %s at %s", e.instanceID, e.resourceID, e.currentLeader.timestamp)
	return nil
}

// Close implements election2.Election
func (e *Election) Close(ctx context.Context) error {
	if err := e.Resign(ctx); err != nil {
		klog.Errorf("Failed to resign leadership: %v", err)
		return err
	}
	return nil
}

// Resign implements election2.Election
func (e *Election) Resign(ctx context.Context) error {
	e.leaderLock.L.Lock()
	closer := e.cancel
	e.cancel = nil
	e.leaderLock.L.Unlock()
	if closer == nil {
		return nil
	}
	// Stop trying to elect ourselves
	done := make(chan error)
	closer <- &done
	return <-done
}

// WithMastership implements election2.Election
func (e *Election) WithMastership(ctx context.Context) (context.Context, error) {
	cctx, cancel := context.WithCancel(ctx)
	e.leaderLock.L.Lock()
	defer e.leaderLock.L.Unlock()
	if e.currentLeader.currentLeader != e.instanceID {
		// Not the leader, cancel
		cancel()
		return cctx, nil
	}

	// Start a goroutine to cancel the context when we are no longer leader
	go func() {
		e.leaderLock.L.Lock()
		defer e.leaderLock.L.Unlock()
		for e.currentLeader.currentLeader == e.instanceID {
			e.leaderLock.Wait()
		}
		select {
		case <-ctx.Done():
			// Don't complain if our context already completed.
			return
		default:
			cancel()
			klog.Warningf("%s cancelled: lost leadership, %s is the leader at %s", e.resourceID, e.currentLeader.currentLeader, e.currentLeader.timestamp)
		}
	}()

	return cctx, nil
}

// becomeLeaderLoop runs continuously to participate in elections until a message is sent on `cancel`
func (e *Election) becomeLeaderLoop(ctx context.Context, closer chan *chan error) {
	for {
		select {
		case ch := <-closer:
			err := e.tearDown()
			klog.Infof("Election teardown for %s: %v", e.resourceID, err)
			if ch != nil {
				*ch <- err
			}
			return
		default:
			leader, err := e.tryBecomeLeader(ctx)
			if err != nil {
				klog.Errorf("Failed attempt to become leader for %s, retrying: %v", e.resourceID, err)
			} else {
				e.leaderLock.L.Lock()
				if leader != e.currentLeader {
					// Note: this code does not actually care _which_ instance was
					// elected, it sends notifications on each leadership change.
					e.currentLeader = leader
					e.leaderLock.Broadcast()
				}
				e.leaderLock.L.Unlock()
			}
			time.Sleep(e.electionInterval)
		}
	}
}

func (e *Election) tryBecomeLeader(ctx context.Context) (leaderData, error) {
	leader := leaderData{}
	tx, err := e.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return leader, fmt.Errorf("BeginTX: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			klog.Errorf("Rollback failed: %v", err)
		}
	}()
	row := tx.QueryRow(
		"SELECT leader, last_update FROM LeaderElection WHERE resource_id = ?",
		e.resourceID)
	if err := row.Scan(&leader.currentLeader, &leader.timestamp); err != nil {
		return leader, fmt.Errorf("Select: %w", err)
	}

	if leader.currentLeader != e.instanceID && leader.timestamp.Add(e.electionInterval*10).After(time.Now()) {
		return leader, nil // Someone else won the election
	}

	timestamp := time.Now()
	_, err = tx.Exec(
		"UPDATE LeaderElection SET leader = ?, last_update = ? WHERE resource_id = ? AND leader = ? AND last_update = ?",
		e.instanceID, timestamp, e.resourceID, leader.currentLeader, leader.timestamp)
	if err != nil {
		return leader, fmt.Errorf("Update: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return leader, fmt.Errorf("Commit failed: %w", err)
	}
	leader = leaderData{currentLeader: e.instanceID, timestamp: timestamp}
	return leader, nil
}

func (e *Election) tearDown() error {
	e.leaderLock.L.Lock()
	defer e.leaderLock.L.Unlock()
	if e.currentLeader.currentLeader != e.instanceID {
		return nil
	}
	e.currentLeader.currentLeader = "empty leader"
	e.leaderLock.Broadcast()

	// Reset election time to epoch to allow a faster fail-over
	res, err := e.db.Exec(
		"UPDATE LeaderElection SET last_update = ? WHERE resource_id = ? AND leader = ? AND last_update = ?",
		time.Time{}, e.resourceID, e.instanceID, e.currentLeader.timestamp)
	if err != nil {
		return fmt.Errorf("Update: %w", err)
	}
	if n, err := res.RowsAffected(); n != 1 || err != nil {
		return fmt.Errorf("failed to resign leadership: %d, %w", n, err)
	}
	return nil
}

func (e *Election) initializeLock(ctx context.Context) error {
	var leader string
	err := e.db.QueryRow(
		"SELECT leader FROM LeaderElection WHERE resource_id = ?",
		e.resourceID,
	).Scan(&leader)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = e.db.Exec(
			"INSERT INTO LeaderElection (resource_id, leader, last_update) VALUES (?, ?, ?)",
			e.resourceID, "empty leader", time.Time{},
		)
	}
	return err
}

type SqlFactory struct {
	db         *sql.DB
	instanceID string
	opts       []Option
}

var _ election2.Factory = (*SqlFactory)(nil)

type Option func(*Election) *Election

func NewFactory(instanceID string, database *sql.DB, opts ...Option) (*SqlFactory, error) {
	return &SqlFactory{db: database, instanceID: instanceID, opts: opts}, nil
}

func WithElectionInterval(interval time.Duration) Option {
	return func(f *Election) *Election {
		f.electionInterval = interval
		return f
	}
}

// NewElection implements election2.Factory.
func (f *SqlFactory) NewElection(ctx context.Context, resourceID string) (election2.Election, error) {
	// Ensure we have a database connection
	if f.db == nil {
		return nil, fmt.Errorf("no database connection")
	}
	if err := f.db.Ping(); err != nil {
		return nil, err
	}
	e := &Election{
		db:               f.db,
		instanceID:       f.instanceID,
		resourceID:       resourceID,
		leaderLock:       sync.Cond{L: &sync.Mutex{}},
		electionInterval: 1 * time.Second,
	}
	for _, opt := range f.opts {
		e = opt(e)
	}
	if err := e.initializeLock(ctx); err != nil {
		return nil, err
	}

	return e, nil
}
