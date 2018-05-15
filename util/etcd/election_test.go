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
	"log"
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

func everyoneAgreesOnMaster(t *testing.T, ctx context.Context, want string, elections []util.MasterElection) {
	t.Helper()
	for _, e := range elections {
		for {
			log.Print("Getting master")
			m, err := e.GetCurrentMaster(ctx)
			if err == concurrency.ErrElectionNoLeader {
				t.Error("No leader...")
				time.Sleep(time.Second)
				continue
			} else if err != nil {
				t.Fatalf("Failed to GetCurrentMaster: %v", err)
			}
			if m != want {
				log.Printf("got %v want %v", m, want)
				t.Errorf("Current master is %v, want %v", m, want)
			}
			break
		}
	}
}

func TestGetCurrentMaster(t *testing.T) {
	e, client, cleanup, err := etcd.StartEtcd()
	if err != nil {
		t.Fatalf("StartEtcd(): %v", err)
	}
	defer cleanup()
	client2, err := clientv3.New(clientv3.Config{
		Endpoints: []string{e.Config().LCUrls[0].String()},
	})
	if err != nil {
		t.Fatalf("Error creating 2nd etcd client: %v", err)
	}
	defer client2.Close()

	ctx := context.Background()
	fact1 := NewElectionFactory("serv1", client, "trees/")
	fact2 := NewElectionFactory("serv2", client2, "trees/")

	el1, err := fact1.NewElection(ctx, 10)
	if err != nil {
		t.Fatalf("NewElection(10): %v", err)
	}
	el2, err := fact2.NewElection(ctx, 10)
	if err != nil {
		t.Fatalf("NewElection(10): %v", err)
	}
	elections := []util.MasterElection{el1, el2}

	wg := &sync.WaitGroup{}
	for _, e := range elections {
		wg.Add(1)
		go func(e util.MasterElection) {
			if err := e.WaitForMastership(ctx); err != nil {
				t.Fatalf("WaitForMastership(10): %v", err)
			}
			id := e.(*MasterElection).instanceID

			log.Printf("%v is master", id)

			everyoneAgreesOnMaster(t, ctx, id, elections)
			if err := e.Close(ctx); err != nil {
				t.Fatalf("Close(10): %v", err)
			}
			wg.Done()
		}(e)
	}
	wg.Wait()
}
