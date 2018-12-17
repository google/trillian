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

// Package etcd provides an implementation of master election based on etcd.
// TODO(pavelkalinnikov): Add tests.
package etcd

import (
	"context"
	"fmt"
	"strings"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/clientv3/concurrency"
	"github.com/golang/glog"
	"github.com/google/trillian/util/election2"
)

// Election is an implementation of election2.Election based on etcd.
type Election struct {
	resourceID string
	instanceID string
	lockFile   string

	client   *clientv3.Client
	session  *concurrency.Session
	election *concurrency.Election
}

// Await blocks until the instance captures mastership.
func (e *Election) Await(ctx context.Context) error {
	return e.election.Campaign(ctx, e.instanceID)
}

// WithMastership returns a "mastership context" which remains active until the
// instance stops being the master, or the passed in context is canceled.
func (e *Election) WithMastership(ctx context.Context) (context.Context, error) {
	// Get a channel for notifications of election status (using the cancelable
	// context so that the monitoring goroutine below and the goroutine started
	// by WithMastership will reliably terminate).
	cctx, cancel := context.WithCancel(ctx)
	ch := e.election.Observe(cctx)
	etcdRev := e.election.Rev() // The revision at which e became the master.

	// Verify mastership before returning context.
	select {
	case <-ctx.Done():
		cancel()
		return nil, ctx.Err()
	case rsp, ok := <-ch:
		if !ok || rsp.Kvs[0].CreateRevision != etcdRev {
			// Mastership has been overtaken in the meantime, or not capturead at all.
			cancel()
			return cctx, nil
		}
	}

	// At this point we have observed confirmation that we are the master; start
	// a goroutine to monitor for anyone else overtaking us.
	go func() {
		defer func() {
			cancel()
			glog.Infof("%s: canceled mastership context", e.resourceID)
		}()

		for rsp := range ch {
			if rsp.Kvs[0].CreateRevision != etcdRev {
				conquerorID := string(rsp.Kvs[0].Value)
				glog.Warningf("%s: mastership overtaken by %s", e.resourceID, conquerorID)
				break
			}
		}
	}()

	return cctx, nil
}

// Resign releases mastership for this instance. The instance can be elected
// again using Await. Idempotent, might be useful to retry if fails.
func (e *Election) Resign(ctx context.Context) error {
	return e.election.Resign(ctx)
}

// Close resigns and permanently stops participating in election. No other
// method should be called after Close.
func (e *Election) Close(ctx context.Context) error {
	if err := e.Resign(ctx); err != nil && err != concurrency.ErrElectionNotLeader {
		glog.Errorf("%s: Resign(): %v", e.resourceID, err)
	}
	// Session's Close revokes the underlying lease, which results in removing
	// the election-related keys. This achieves the effect of resignation even if
	// the above Resign call failed (e.g. due to ctx cancelation).
	return e.session.Close()
}

// Factory creates Election instances.
type Factory struct {
	client     *clientv3.Client
	instanceID string
	lockDir    string
}

// NewFactory builds an election factory that uses the given parameters. The
// passed in etcd client should remain valid for the lifetime of the object.
func NewFactory(instanceID string, client *clientv3.Client, lockDir string) *Factory {
	return &Factory{
		client:     client,
		instanceID: instanceID,
		lockDir:    lockDir,
	}
}

// NewElection creates a specific Election instance.
func (f *Factory) NewElection(ctx context.Context, resourceID string) (election2.Election, error) {
	// TODO(pavelkalinnikov): Re-create the session if it expires.
	// TODO(pavelkalinnikov): Share the same session between Election instances.
	session, err := concurrency.NewSession(f.client)
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd session: %v", err)
	}
	lockFile := fmt.Sprintf("%s/%s", strings.TrimRight(f.lockDir, "/"), resourceID)
	election := concurrency.NewElection(session, lockFile)

	el := Election{
		resourceID: resourceID,
		instanceID: f.instanceID,
		lockFile:   lockFile,
		client:     f.client,
		session:    session,
		election:   election,
	}
	glog.Infof("Election created: %+v", el)

	return &el, nil
}
