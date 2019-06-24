package cloudspanner

import (
	"context"
	"flag"
	"testing"

	"cloud.google.com/go/spanner"
	"github.com/google/trillian/storage/testharness"
)

var cloudDBPath = flag.String("test_cloud_spanner_database", "", "projects/my-project/instances/my-instance/database/my-db")

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
	s := NewMapStorage(ctx, GetTestDB(ctx, t))
	testharness.TestMapStorage(t, s)
}
