package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/postgres"
	"github.com/google/trillian/storage/testonly"
)

var allTables = []string{"unsequenced", "tree_head", "sequenced_leaf_data", "leaf_data", "subtree", "tree_control", "trees"}
var DB *sql.DB

func TestPgAdminStorage(t *testing.T) {
	tester := &testonly.AdminStorageTester{NewAdminStorage: func() storage.AdminStorage {
		cleanTestDB(DB)
		return postgres.NewAdminStorage(DB)
	}}
	tester.TestCreateTree(t)
}

// cleanTestDB deletes all the entries in the database.
func cleanTestDB(db *sql.DB) {
	for _, table := range allTables {
		if _, err := db.ExecContext(context.TODO(), fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			panic(fmt.Sprintf("Failed to delete rows in %s: %v", table, err))
		}
	}
}
