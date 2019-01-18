package bigquery

import (
	"context"
	"errors"

	"cloud.google.com/go/bigquery"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	pingStatement = `
	SELECT year
	FROM ` + "`bigquery-public-data.usa_names.usa_1910_2013`" + `
	WHERE name = "William" 
	GROUP by year 
	ORDER by year
	`
)

var (
	// ErrNotFound is returned when a read/lookup fails because there was no such
	// item.
	ErrNotFound = status.Errorf(codes.NotFound, "not found")

	// ErrNotImplemented is returned by any interface methods which have not been
	// implemented yet.
	ErrNotImplemented = errors.New("not implemented")

	// ErrTransactionClosed is returned by interface methods when an operation is
	// attempted on a transaction whose Commit or Rollback methods have
	// previously been called.
	ErrTransactionClosed = errors.New("transaction is closed")

	// ErrWrongTXType is returned when, somehow, a write operation is attempted
	// with a read-only transaction.  This should not even be possible.
	ErrWrongTXType = errors.New("mutating method called on read-only transaction")
)

// treeStorage provides a shared base for the concrete BigQuery-backed
// implementation of the Trillian storage.LogStorage and storage.MapStorage
// interfaces.
type treeStorage struct {
	admin  storage.AdminStorage
	opts   TreeStorageOptions
	client *bigquery.Client
}

// TreeStorageOptions holds various levers for configuring the tree storage instance.
type TreeStorageOptions struct {
}

func newTreeStorageWithOpts(client *bigquery.Client, opts TreeStorageOptions) *treeStorage {
	return &treeStorage{client: client, admin: nil, opts: opts}
}

func (t *treeStorage) getTreeAndConfig(ctx context.Context, tree *trillian.Tree) (*trillian.Tree, proto.Message, error) {
}
func (t *treeStorage) begin(ctx context.Context, tree *trillian.Tree, newCache newCacheFn, stx spanRead) (*treeTX, error) {
}
func (t *treeTX) getLatestRoot(ctx context.Context) error {
}

func checkDatabaseAccessible(ctx context.Context, client *bigquery.Client) error {
	query := client.Query(pingStatement)
	it, err := query.Read(ctx)
	if err != nil {
		return err
	}
	return nil
}
