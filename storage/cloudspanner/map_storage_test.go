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
	"testing"

	"cloud.google.com/go/spanner"
	"github.com/google/trillian/integration/storagetest"
	"github.com/google/trillian/storage"
)

// To run cloudspanner tests,
// 1) Set the -test_cloud_spanner_database flag
// 2) Set application default credentials `gcloud auth application-default login`

var cloudDBPath = flag.String("test_cloud_spanner_database", "", "eg: projects/my-project/instances/my-instance/database/my-db")

func GetTestDB(ctx context.Context, t *testing.T) *spanner.Client {
	t.Helper()
	if *cloudDBPath == "" {
		t.Skip("-test_cloud_spanner_database flag is unset")
	}
	client, err := spanner.NewClient(ctx, *cloudDBPath)
	if err != nil {
		t.Fatalf("spanner.NewClient(): %v", err)
	}
	return client
}

func TestSuite(t *testing.T) {
	ctx := context.Background()
	db := GetTestDB(ctx, t)

	storageFactory := func(context.Context, *testing.T) (storage.MapStorage, storage.AdminStorage) {
		cleanTestDB(ctx, t, db)
		return NewMapStorage(ctx, db), NewAdminStorage(db)
	}

	storagetest.TestMapStorage(t, storageFactory)
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
