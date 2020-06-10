// Copyright 2019 Google Inc. All Rights Reserved.
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

package cloudspanner

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/spannertest"
	"google.golang.org/api/option"
	"google.golang.org/grpc"

	database "cloud.google.com/go/spanner/admin/database/apiv1"
	databasepb "google.golang.org/genproto/googleapis/spanner/admin/database/v1"
)

// To run cloudspanner tests,
// 1) Set the -test_cloud_spanner_database flag
// 2) Set application default credentials `gcloud auth application-default login`

var cloudDBPath = flag.String("test_cloud_spanner_database", ":memory:", "eg: projects/my-project/instances/my-instance/database/my-db")

func GetTestDB(ctx context.Context, t *testing.T) *spanner.Client {
	t.Helper()
	switch dbPath := *cloudDBPath; dbPath {
	case "":
		t.Skip("-test_cloud_spanner_database flag is unset")
	case ":memory:":
		ddl, err := readDDL()
		if err != nil {
			t.Fatal(err)
		}
		return inMemClient(ctx, t, uniqueDBName("fakeProject", "fakeInstance"), ddl)
	default:
		client, err := spanner.NewClient(ctx, *cloudDBPath)
		if err != nil {
			t.Fatalf("spanner.NewClient(): %v", err)
		}
		return client
	}
}

var dbCount uint32 // Unique per test invocation

func uniqueDBName(project, instance string) string {
	// Unique per test binary invocation
	timestamp := time.Now().UTC().Format("jan-02-15-04-05")
	testBinary := strings.ToLower(strings.Replace(path.Base(os.Args[0]), ".test", "", 1))
	invocationID := fmt.Sprintf("%s-%s", timestamp, testBinary)

	database := fmt.Sprintf("%s-%d", invocationID, atomic.AddUint32(&dbCount, 1))
	return fmt.Sprintf("projects/%s/instances/%s/databases/%s", project, instance, database)
}

func inMemClient(ctx context.Context, t testing.TB, dbName string, statements []string) *spanner.Client {
	t.Helper()
	// Don't use SPANNER_EMULATOR_HOST because we need the raw connection for
	// the database admin client anyway.

	t.Logf("Using in-memory fake Spanner DB: %s", dbName)
	srv, err := spannertest.NewServer("localhost:0")
	if err != nil {
		t.Fatalf("Starting in-memory fake: %v", err)
	}
	t.Cleanup(srv.Close)
	srv.SetLogger(t.Logf)
	dialCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(dialCtx, srv.Addr, grpc.WithInsecure())
	if err != nil {
		t.Fatalf("Dialing in-memory fake: %v", err)
	}
	client, err := spanner.NewClient(ctx, dbName, option.WithGRPCConn(conn))
	if err != nil {
		conn.Close()
		t.Fatalf("Connecting to in-memory fake: %v", err)
	}
	t.Cleanup(client.Close)

	// Set database schema
	t.Logf("DDL update: %s", statements)
	adminClient, err := database.NewDatabaseAdminClient(ctx, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("Connecting to in-memory fake DB admin: %v", err)
	}
	op, err := adminClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
		Database:   dbName,
		Statements: statements,
	})
	if err != nil {
		t.Fatalf("Starting DDL update: %v", err)
	}
	if err := op.Wait(ctx); err != nil {
		t.Fatalf("failed DDL update: %v", err)
	}

	return client
}

func cleanTestDB(ctx context.Context, t *testing.T, db *spanner.Client) {
	if _, err := db.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		mutations := make([]*spanner.Mutation, 0)
		for _, table := range []string{
			"TreeRoots",
			"TreeHeads",
			"SubtreeData",
			"LeafData",
			"SequencedLeafData",
			"Unsequenced",
			"MapLeafData",
		} {
			mutations = append(mutations, spanner.Delete(table, spanner.AllKeys()))
		}
		return txn.BufferWrite(mutations)
	}); err != nil {
		t.Fatalf("Failed to clean databse: %v", err)
	}
}
