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
	"testing"

	"github.com/google/trillian/testonly/integration/etcd"
	"github.com/google/trillian/util/election2/testonly"
)

func TestElectionThroughCommonClient(t *testing.T) {
	_, client, cleanup, err := etcd.StartEtcd()
	if err != nil {
		t.Fatalf("StartEtcd(): %v", err)
	}
	defer cleanup()

	ctx := context.Background()
	fact := NewFactory("serv", client, "res/")

	el1, err := fact.NewElection(ctx, "10")
	if err != nil {
		t.Fatalf("NewElection(10): %v", err)
	}
	el2, err := fact.NewElection(ctx, "20")
	if err != nil {
		t.Fatalf("NewElection(20): %v", err)
	}

	if err := el1.Await(ctx); err != nil {
		t.Fatalf("Await(10): %v", err)
	}
	if err := el2.Await(ctx); err != nil {
		t.Fatalf("Await(20): %v", err)
	}

	if err := el1.Close(ctx); err != nil {
		t.Fatalf("Close(10): %v", err)
	}
	if err := el2.Close(ctx); err != nil {
		t.Fatalf("Close(20): %v", err)
	}
}

func TestElection(t *testing.T) {
	_, client, cleanup, err := etcd.StartEtcd()
	if err != nil {
		t.Fatalf("StartEtcd(): %v", err)
	}
	defer cleanup()

	for _, nt := range testonly.Tests {
		// Create a new Factory for each test for better isolation.
		fact := NewFactory("testID", client, fmt.Sprintf("%s/resources/", nt.Name))
		t.Run(nt.Name, func(t *testing.T) {
			nt.Run(t, fact)
		})
	}
}
