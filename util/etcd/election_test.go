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

package etcd

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/clientv3/concurrency"
	"github.com/google/trillian/testonly/integration/etcd"
	"github.com/google/trillian/util"
)

func TestMasterElectionThroughCommonClient(t *testing.T) {
	_, client, cleanup, err := etcd.StartEtcd()
	if err != nil {
		t.Fatalf("StartEtcd(): %v", err)
	}
	defer cleanup()

	ctx := context.Background()
	fact := NewElectionFactory("serv", client, "trees/")

	el1, err := fact.NewElection(ctx, 10)
	if err != nil {
		t.Fatalf("NewElection(10): %v", err)
	}
	el2, err := fact.NewElection(ctx, 20)
	if err != nil {
		t.Fatalf("NewElection(20): %v", err)
	}

	if err := el1.WaitForMastership(ctx); err != nil {
		t.Fatalf("WaitForMastership(10): %v", err)
	}
	if err := el2.WaitForMastership(ctx); err != nil {
		t.Fatalf("WaitForMastership(20): %v", err)
	}

	if err := el1.Close(ctx); err != nil {
		t.Fatalf("Close(10): %v", err)
	}
	if err := el2.Close(ctx); err != nil {
		t.Fatalf("Close(20): %v", err)
	}
}

func everyoneAgreesOnMaster(ctx context.Context, t *testing.T, want string, elections []util.MasterElection) {
	t.Helper()
	for _, e := range elections {
		for {
			got, err := e.GetCurrentMaster(ctx)
			if err == concurrency.ErrElectionNoLeader {
				t.Error("No leader...")
				time.Sleep(time.Second)
				continue
			} else if err != nil {
				t.Fatalf("Failed to GetCurrentMaster: %v", err)
			}
			if got != want {
				t.Errorf("Current master is %v, want %v", got, want)
			}
			break
		}
	}
}

func mustCreateClientFor(t *testing.T, e string) *clientv3.Client {
	t.Helper()
	c, err := clientv3.New(clientv3.Config{
		Endpoints: []string{e},
	})
	if err != nil {
		t.Fatalf("Error creating etcd client: %v", err)
	}
	return c
}

func TestGetCurrentMaster(t *testing.T) {
	e, client, cleanup, err := etcd.StartEtcd()
	if err != nil {
		t.Fatalf("StartEtcd(): %v", err)
	}
	defer cleanup()
	// add another active participant
	client2 := mustCreateClientFor(t, e.Config().LCUrls[0].String())
	defer client2.Close()

	// add a couple of passive observers
	ob1 := mustCreateClientFor(t, e.Config().LCUrls[0].String())
	defer ob1.Close()
	ob2 := mustCreateClientFor(t, e.Config().LCUrls[0].String())
	defer ob2.Close()

	ctx := context.Background()
	pfx := "trees/"
	facts := []*ElectionFactory{
		NewElectionFactory("serv1", client, pfx),
		NewElectionFactory("serv2", client2, pfx),
		NewElectionFactory("ob1", ob1, pfx),
		NewElectionFactory("ob2", ob2, pfx),
	}

	elections := make([]util.MasterElection, 0)
	for _, f := range facts {
		e, err := f.NewElection(ctx, 10)
		if err != nil {
			t.Fatalf("NewElection(10): %v", err)
		}
		elections = append(elections, e)
	}

	wg := &sync.WaitGroup{}
	for _, e := range elections[0:2] {
		wg.Add(1)
		go func(e util.MasterElection) {
			if err := e.WaitForMastership(ctx); err != nil {
				t.Errorf("WaitForMastership(10): %v", err)
			}
			id := e.(*MasterElection).instanceID

			everyoneAgreesOnMaster(ctx, t, id, elections)
			if err := e.Close(ctx); err != nil {
				t.Errorf("Close(10): %v", err)
			}
			wg.Done()
		}(e)
	}
	wg.Wait()
}

func TestGetCurrentMasterReturnsNoLeader(t *testing.T) {
	ctx := context.Background()
	_, client, cleanup, err := etcd.StartEtcd()
	if err != nil {
		t.Fatalf("StartEtcd(): %v", err)
	}
	defer cleanup()
	fact := NewElectionFactory("serv1", client, "trees/")
	el, err := fact.NewElection(ctx, 10)
	if err != nil {
		t.Fatalf("NewElection(10): %v", err)
	}
	if err := el.WaitForMastership(ctx); err != nil {
		t.Errorf("WaitForMastership(10): %v", err)
	}
	if err := el.Close(ctx); err != nil {
		t.Errorf("Close(10): %v", err)
	}
	_, err = el.GetCurrentMaster(ctx)
	if want := util.ErrNoLeader; err != want {
		t.Errorf("GetCurrentMaster()=%v, want %v", err, want)
	}
}
