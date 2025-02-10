// Copyright 2025 Google LLC. All Rights Reserved.
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

package k8s

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/trillian/util/election2"
	"github.com/google/trillian/util/election2/testonly"
	v1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
)

var Tests []testonly.NamedTest

func init() {
	Tests = append(testonly.Tests,
		testonly.NamedTest{Name: "RunLeaseEvents", Run: runLeaseEvents},
		testonly.NamedTest{Name: "RunParallelElections", Run: runParallelElections},
		testonly.NamedTest{Name: "RunWithMastershipMultiple", Run: runWithMastershipMultiple},
	)
}

func runLeaseEvents(t *testing.T, f election2.Factory) {
	for _, tc := range []struct {
		desc  string
		event watch.EventType
		done  bool
	}{
		{desc: "modified", event: watch.Modified, done: false},
		{desc: "modified-cancel", event: watch.Modified, done: true},
		{desc: "deleted", event: watch.Deleted, done: true},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			factory := f.(*Factory)
			client := factory.client

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			e, err := f.NewElection(ctx, tc.desc)
			if err != nil {
				t.Fatalf("NewElection(): %v", err)
			}
			if err = e.Await(ctx); err != nil {
				t.Fatalf("Await(): %v", err)
			}
			mctx, err := e.WithMastership(ctx)
			if err != nil {
				t.Errorf("WithMastership): %v", err)
			}
			testonly.CheckNotDone(mctx, t)

			// Simulate events
			lease := &v1.Lease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tc.desc,
					Namespace: factory.namespace,
				},
				Spec: v1.LeaseSpec{
					HolderIdentity: &factory.instanceID,
				},
			}

			newHolder := "new-holder-identity"
			if tc.done {
				lease.Spec.HolderIdentity = &newHolder
			}

			switch tc.event {
			case watch.Modified:
				_, err = client.Leases(factory.namespace).Update(ctx, lease, metav1.UpdateOptions{})
				if err != nil {
					t.Errorf("failed to update lease %v", err)
				}
			case watch.Deleted:
				err = client.Leases(factory.namespace).Delete(ctx, tc.desc, metav1.DeleteOptions{})
				if err != nil {
					t.Errorf("failed to delete lease %v", err)
				}
			}

			if tc.done {
				testonly.CheckDone(mctx, t, 1*time.Second)
			} else {
				testonly.CheckNotDone(mctx, t)
			}
		})
	}
}

func runParallelElections(t *testing.T, f election2.Factory) {
	for _, tc := range []struct {
		desc  string
		block bool
		err   error
	}{
		{desc: "same", block: true, err: context.DeadlineExceeded},
		{desc: "different", block: false},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			resource1, resource2 := "resource-1", "resource-2"

			factory1 := *f.(*Factory)
			factory1.instanceID = "instance-1"

			factory2 := *f.(*Factory)
			factory2.instanceID = "instance-2"

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			e1, err := factory1.NewElection(ctx, resource1)
			if err != nil {
				t.Fatalf("NewElection(): %v", err)
			}

			if tc.block {
				resource2 = resource1
			}
			e2, err := factory2.NewElection(ctx, resource2)
			if err != nil {
				t.Fatalf("NewElection(): %v", err)
			}

			if got := e1.Await(ctx); got != nil {
				t.Errorf("Await(): %v, want %v", got, nil)
			}

			if tc.block {
				ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()
			}

			if got, want := e2.Await(ctx), tc.err; !errors.Is(got, want) {
				t.Errorf("Await(): %v, want %v", got, want)
			}
		})
	}
}

func runWithMastershipMultiple(t *testing.T, f election2.Factory) {
	ctxErr := errors.New("WithMastership error")
	for _, tc := range []struct {
		desc     string
		beMaster bool
		cancel   bool
		err      error
		wantErr  error
	}{
		{desc: "master", beMaster: true},
		{desc: "master-cancel", beMaster: true, cancel: true},
		{desc: "not-master", beMaster: false},
		{desc: "not-master-cancel", cancel: true},
		{desc: "error", err: ctxErr, wantErr: ctxErr},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			e, err := f.NewElection(ctx, tc.desc)
			if err != nil {
				t.Fatalf("NewElection(): %v", err)
			}
			d := testonly.NewDecorator(e)
			d.Update(testonly.Errs{WithMastership: tc.err})
			if tc.beMaster {
				if err := d.Await(ctx); err != nil {
					t.Fatalf("Await(): %v", err)
				}
			}
			mctx1, err := d.WithMastership(ctx)
			if want := tc.wantErr; err != want {
				t.Fatalf("WithMastership(): %v, want %v", err, want)
			}
			mctx2, err := d.WithMastership(ctx)
			if want := tc.wantErr; err != want {
				t.Fatalf("WithMastership(): %v, want %v", err, want)
			}
			if err != nil {
				return
			}
			if tc.cancel {
				cancel()
			}
			if tc.beMaster && !tc.cancel {
				testonly.CheckNotDone(mctx1, t)
				testonly.CheckNotDone(mctx2, t)
			} else {
				testonly.CheckDone(mctx1, t, 0)
				testonly.CheckDone(mctx2, t, 0)
			}
		})
	}
}

func TestElection(t *testing.T) {
	for _, nt := range Tests {
		c := fake.NewClientset()

		fact := &Factory{
			client:        c.CoordinationV1(),
			instanceID:    "testID",
			namespace:     "default",
			leaseDuration: 15 * time.Second,
			retryPeriod:   2 * time.Second,
		}
		t.Run(nt.Name, func(t *testing.T) {
			nt.Run(t, fact)
		})
	}
}
