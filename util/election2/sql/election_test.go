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
	"fmt"
	"testing"
	"time"

	"github.com/google/trillian/util/election2/testonly"
	_ "github.com/mattn/go-sqlite3"
)

func TestOneElection(t *testing.T) {
	db, err := initializeDB("sqllite3", "file::one-election?mode=memory")
	if err != nil {
		t.Fatalf("Unable to initialize database: %v", err)
	}

	factory, err := NewFactory("serv", db, WithElectionInterval(10*time.Millisecond))
	if err != nil {
		t.Fatalf("Unable to open database: %v", err)
	}

	ctx := context.Background()
	el1, err := factory.NewElection(ctx, "5")
	if err != nil {
		t.Fatalf("NewElection(5): %v", err)
	}

	if err := el1.Await(ctx); err != nil {
		t.Fatalf("Await(5): %v", err)
	}
	if err := el1.Await(ctx); err != nil {
		t.Fatalf("Await when holding lock(5): %v", err)
	}

	if err := el1.Resign(ctx); err != nil {
		t.Fatalf("Resign(5): %v", err)
	}

	if err := el1.Await(ctx); err != nil {
		t.Fatalf("Await(5): %v", err)
	}

	if err := el1.Close(ctx); err != nil {
		t.Fatalf("Close(5): %v", err)
	}
}

func TestTwoElections(t *testing.T) {
	db, err := initializeDB("sqllite3", "file::two-election?mode=memory")
	if err != nil {
		t.Fatalf("Unable to initialize database: %v", err)
	}

	factory, err := NewFactory("serv", db, WithElectionInterval(10*time.Millisecond))
	if err != nil {
		t.Fatalf("Unable to open database: %v", err)
	}

	ctx := context.Background()
	el1, err := factory.NewElection(ctx, "10")
	if err != nil {
		t.Fatalf("NewElection(10): %v", err)
	}
	el2, err := factory.NewElection(ctx, "20")
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

func TestElectionTwoServers(t *testing.T) {
	db, err := initializeDB("sqllite3", "file::two-election?mode=memory")
	if err != nil {
		t.Fatalf("Unable to initialize database: %v", err)
	}

	factory1, err := NewFactory("s1", db, WithElectionInterval(10*time.Millisecond))
	if err != nil {
		t.Fatalf("Unable to open database: %v", err)
	}
	factory2, err := NewFactory("s2", db, WithElectionInterval(10*time.Millisecond))
	if err != nil {
		t.Fatalf("Unable to open database: %v", err)
	}

	ctx := context.Background()
	el1, err := factory1.NewElection(ctx, "10")
	if err != nil {
		t.Fatalf("NewElection(10): %v", err)
	}
	el2, err := factory2.NewElection(ctx, "10")
	if err != nil {
		t.Fatalf("NewElection(10, again): %t %v", err, err)
	}

	if err := el1.Await(ctx); err != nil {
		t.Fatalf("Await(el1): %v", err)
	}
	go func() {
		time.Sleep(4 * time.Millisecond)
		err := el1.Resign(ctx)
		if err != nil {
			t.Log(err)
		}
	}()
	if err := el2.Await(ctx); err != nil {
		t.Fatalf("Await(el2): %v", err)
	}

	if err := el1.Close(ctx); err != nil {
		t.Fatalf("Close(el1): %v", err)
	}
	if err := el2.Close(ctx); err != nil {
		t.Fatalf("Close(el2): %v", err)
	}
}

func TestElection(t *testing.T) {
	for _, nt := range testonly.Tests {
		// Create a new DB and Factory for each test for better isolation.
		db, err := initializeDB("sqllite3", fmt.Sprintf("file::%s?mode=memory", nt.Name))
		if err != nil {
			t.Fatalf("Initialize DB: %v", err)
		}

		fact, err := NewFactory("testID", db, WithElectionInterval(10*time.Millisecond))
		if err != nil {
			t.Fatalf("NewFactory: %v", err)
		}
		t.Run(nt.Name, func(t *testing.T) {
			nt.Run(t, fact)
		})
	}
}

func initializeDB(driver string, uri string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", uri)
	if err != nil {
		return nil, err
	}
	// Additional connections open a _new_, _empty_ database!
	db.SetMaxOpenConns(1)
	_, err = db.Exec("CREATE TABLE leader_election (resource_id TEXT PRIMARY KEY, leader TEXT, last_update TIMESTAMP);")
	if err != nil {
		return nil, err
	}

	return db, nil
}
