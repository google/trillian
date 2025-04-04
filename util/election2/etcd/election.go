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

// Package etcd provides an implementation of master election based on etcd.
package etcd

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/trillian/util/election2"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	"k8s.io/klog/v2"
)

const resignID = "<resign>"

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
	etcdRev := e.election.Rev() // The revision at which e became the master.
	if etcdRev == 0 {
		// Not even tried to become the master. Return a canceled context.
		cancel()
		return cctx, nil
	}

	// Was the master once, so watch for latest mastership updates.
	ch := e.election.Observe(cctx)
	// Verify that we are still the master, before returning context.
	select {
	case <-ctx.Done():
		cancel()
		return nil, ctx.Err()
	case rsp, ok := <-ch:
		kv := rsp.Kvs[0]
		if !ok || kv.CreateRevision != etcdRev || string(kv.Value) == resignID {
			// Mastership has been overtaken, released, or not capturead at all.
			cancel()
			return cctx, nil
		}
	}

	// At this point we have observed confirmation that we are the master; start
	// a goroutine to monitor for anyone else overtaking us.
	go func() {
		defer func() {
			cancel()
			klog.Infof("%s: canceled mastership context", e.resourceID)
		}()

		for rsp := range ch {
			kv := rsp.Kvs[0]
			if kv.CreateRevision != etcdRev {
				conquerorID := string(kv.Value)
				// TODO(pavelkalinnikov): conquerorID can be resignID too. Serialize a
				// protobuf with all mastership details instead of ID string.
				klog.Warningf("%s: mastership overtaken by %s", e.resourceID, conquerorID)
				break
			} else if string(kv.Value) == resignID {
				klog.Infof("%s: canceling context due to resignation", e.resourceID)
				break
			}
		}
	}()

	return cctx, nil
}

// Resign releases mastership for this instance. The instance can be elected
// again using Await. Idempotent, might be useful to retry if fails.
func (e *Election) Resign(ctx context.Context) error {
	// Trigger Observe callers to see the update, and cancel mastership contexts.
	err := e.election.Proclaim(ctx, resignID)
	if err == concurrency.ErrElectionNotLeader {
		return nil // Resigning if not master is a no-op.
	} else if err != nil {
		return err
	}
	return e.election.Resign(ctx)
}

// Close resigns and permanently stops participating in election. No other
// method should be called after Close.
func (e *Election) Close(ctx context.Context) error {
	if err := e.Resign(ctx); err != nil && err != concurrency.ErrElectionNotLeader {
		klog.Errorf("%s: Resign(): %v", e.resourceID, err)
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
	klog.Infof("Election created: %+v", el)

	return &el, nil
}
