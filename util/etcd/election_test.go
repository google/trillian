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
	"fmt"
	"os"
	"testing"

	"github.com/coreos/etcd/clientv3"
	"github.com/google/trillian/testonly/integration/etcd"
)

var (
	// client is an etcd client. Initialized by TestMain().
	client *clientv3.Client
)

func TestMain(m *testing.M) {
	_, c, cleanup, err := etcd.StartEtcd()
	if err != nil {
		panic(fmt.Sprintf("StartEtcd(): %v", err))
	}
	client = c
	code := m.Run()
	cleanup()
	os.Exit(code)
}

func TestMasterElectionThroughCommonClient(t *testing.T) {
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
