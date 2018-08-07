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

package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/client"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/testonly/integration"
	"github.com/google/trillian/testonly/setup"
	"github.com/google/trillian/types"
	"github.com/google/trillian/util/flagsaver"
	"google.golang.org/grpc"

	stestonly "github.com/google/trillian/storage/testonly"

	// Load the sqlite3 driver.
	_ "github.com/mattn/go-sqlite3"
)

type trustedLogRootRow struct {
	LogID          int64
	MarshalledRoot []byte
	TreeSize       int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type testEnv struct {
	address       string
	client        *client.LogClient
	treeID        int64
	monitorDBPath string
	monitorDB     *sql.DB
	pubKeyPath    string
	cleanUp       func()
}

func TestEmptyTreeInitialization(t *testing.T) {
	defer flagsaver.Save().Restore()

	env := setupTestEnv(t)
	defer env.cleanUp()

	var monitorErr error
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())

	wg.Add(1)
	go func() {
		// Set the flags.
		setup.SetFlag(t, "server", env.address)
		setup.SetFlag(t, "log_id", fmt.Sprint(env.treeID))
		setup.SetFlag(t, "log_public_key", env.pubKeyPath)
		setup.SetFlag(t, "database", "sqlite3")
		setup.SetFlag(t, "database_uri", env.monitorDBPath)

		monitorErr = runMonitor(ctx)
		wg.Done()
	}()

	var err error
	var logRoots []trustedLogRootRow
	testonly.WaitUntil(t, func() error {
		logRoots, err = getTrustedLogRoots(env.monitorDB)
		if err != nil {
			return err
		}
		if len(logRoots) != 1 {
			return fmt.Errorf("expected exactly one log to be present, instead found: %v", logRoots)
		}
		return nil
	})

	if logRoots[0].LogID != env.treeID {
		t.Fatalf("expected Tree ID stored in the monitor database to be %d, instead got: %d", env.treeID, logRoots[0].LogID)
	}

	if logRoots[0].TreeSize != 0 {
		t.Fatalf("expected Tree Size stored in the monitor database to be 0, instead got: %d", logRoots[0].TreeSize)
	}

	cancel()
	wg.Wait()

	if monitorErr != nil && !strings.Contains(monitorErr.Error(), context.Canceled.Error()) {
		t.Fatalf("got unexpected error from the monitor: %v", monitorErr)
	}
}

func TestTreeRootUpdate(t *testing.T) {
	defer flagsaver.Save().Restore()

	env := setupTestEnv(t)
	defer env.cleanUp()

	var monitorErr error
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())

	wg.Add(1)
	go func() {
		// Set the flags.
		setup.SetFlag(t, "server", env.address)
		setup.SetFlag(t, "log_id", fmt.Sprint(env.treeID))
		setup.SetFlag(t, "log_public_key", env.pubKeyPath)
		setup.SetFlag(t, "database", "sqlite3")
		setup.SetFlag(t, "database_uri", env.monitorDBPath)

		monitorErr = runMonitor(ctx)
		wg.Done()
	}()

	// Add a of new leaves, and wait for them to be indexed.
	if err := env.client.AddLeaf(context.Background(), []byte("test data #1")); err != nil {
		t.Fatalf("Failed to add leaf to Trillian log: %v", err)
	}

	if err := env.client.AddLeaf(context.Background(), []byte("test data #2")); err != nil {
		t.Fatalf("Failed to add leaf to Trillian log: %v", err)
	}

	var err error
	var logRoots []trustedLogRootRow
	testonly.WaitUntil(t, func() error {
		logRoots, err = getTrustedLogRoots(env.monitorDB)
		if err != nil {
			return err
		}
		if len(logRoots) != 1 {
			return fmt.Errorf("expected exactly one log to be present, instead found: %v", logRoots)
		}
		if logRoots[0].TreeSize != 2 {
			return fmt.Errorf("expected Tree Size stored in the monitor database to be 2, instead got: %d", logRoots[0].TreeSize)
		}
		return nil
	})

	cancel()
	wg.Wait()

	if monitorErr != nil && !strings.Contains(monitorErr.Error(), context.Canceled.Error()) {
		t.Fatalf("got unexpected error from the monitor: %v", monitorErr)
	}
}

func setupTestEnv(t *testing.T) testEnv {
	// Set up Trillian servers
	const numSequencers = 2
	serverOpts := []grpc.ServerOption{}
	clientOpts := []grpc.DialOption{grpc.WithInsecure()}
	logEnv, err := integration.NewLogEnvWithGRPCOptions(context.Background(), numSequencers, serverOpts, clientOpts)
	if err != nil {
		t.Fatal(err)
	}
	// logEnv.Close() to be called in the cleanUp function.

	// Create a new Trillian log
	log, err := client.CreateAndInitTree(context.Background(), &trillian.CreateTreeRequest{
		Tree: stestonly.LogTree,
	}, logEnv.Admin, nil, logEnv.Log)
	if err != nil {
		t.Errorf("Failed to create test tree: %v", err)
	}

	// Write the public key somewhere for the binary to be able to read it.
	pubKeyFile, pubKeyFileCleanup := setup.TempFile(t, "test.pubkey.")
	if _, err := pubKeyFile.WriteString(strings.TrimSpace(stestonly.PublicKeyPEM)); err != nil {
		t.Fatal(err)
	}
	// pubKeyFileCleanup() to be called in the cleanUp function.

	// Prepare a temporary database file for sqlite3.
	monitorDBFile, monitorDBCleanup := setup.TempFile(t, "logmonitor.db.")
	// monitorDBCleanup() to be called in the cleanUp function.
	glog.Infof("Temporary sqlite3 database at: %s", monitorDBFile.Name())

	monitorDB, err := sql.Open("sqlite3", monitorDBFile.Name())
	if err != nil {
		t.Fatalf("failed to open the database: %v", err)
	}

	client, err := client.NewFromTree(logEnv.Log, log, types.LogRootV1{})
	if err != nil {
		t.Fatalf("failed to create a log client: %v", err)
	}

	return testEnv{
		address:       logEnv.Address,
		client:        client,
		monitorDBPath: monitorDBFile.Name(),
		monitorDB:     monitorDB,
		treeID:        log.TreeId,
		pubKeyPath:    pubKeyFile.Name(),
		cleanUp: func() {
			logEnv.Close()
			pubKeyFileCleanup()
			monitorDBCleanup()
		},
	}
}

func getTrustedLogRoots(monitorDB *sql.DB) ([]trustedLogRootRow, error) {
	results := []trustedLogRootRow{}
	rows, err := monitorDB.Query("SELECT log_id, marshalled_root, tree_size, created_at, updated_at FROM trusted_log_roots")
	if err != nil {
		return nil, fmt.Errorf("failed to run query to list trusted log roots: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var row trustedLogRootRow
		if err := rows.Scan(&row.LogID, &row.MarshalledRoot, &row.TreeSize, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan trusted log root row: %v", err)
		}
		results = append(results, row)
	}

	return results, nil
}
