package bigquery

import (
	"context"
	"sync"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
)

// LogStorageOptions are tuning, experiments and workarounds that can be used.
type LogStorageOptions struct {
	TreeStorageOptions

	// DequeueAcrossMerkleBuckets controls whether DequeueLeaves will only dequeue
	// from within the chosen Time+Merkle bucket, or whether it will attempt to
	// continue reading from contiguous Merkle buckets until a sufficient number
	// of leaves have been dequeued, or the entire Time bucket has been read.
	DequeueAcrossMerkleBuckets bool
	// DequeueAcrossMerkleBucketsRangeFraction specifies the fraction of Merkle
	// keyspace to dequeue from when using multi-bucket-dequeue.
	DequeueAcrossMerkleBucketsRangeFraction float64
}

// NewLogStorage initialises and returns a new LogStorage.
func NewLogStorage(client *bigquery.Client) storage.LogStorage {
	return NewLogStorageWithOpts(client, LogStorageOptions{})
}

// NewLogStorageWithOpts initialises and returns a new LogStorage.
// The opts parameter can be used to enable custom workarounds.
func NewLogStorageWithOpts(client *bigquery.Client, opts LogStorageOptions) storage.LogStorage {
	ret := &logStorage{
		ts:   newTreeStorageWithOpts(client, opts.TreeStorageOptions),
		opts: opts,
	}
	return ret
}

// logStorage provides a BigQuery backed trillian.LogStorage implementation.
// See third_party/golang/trillian/storage/log_storage.go for more details.
type logStorage struct {
	// ts provides the merkle-tree level primitives which are built upon by this
	// logStorage.
	ts *treeStorage

	// Additional options applied to this logStorage
	opts LogStorageOptions
}

func (ls *logStorage) AddSequencedLeaves(ctx context.Context, tree *trillian.Tree, leaves []*trillian.LogLeaf, timestamp time.Time) ([]*trillian.QueuedLogLeaf, error) {
	return nil, ErrNotImplemented
}
func (ls *logStorage) CheckDatabaseAccessible(ctx context.Context) error {
	return checkDatabaseAccessible(ctx, ls.ts.client)
}
func (ls *logStorage) QueueLeaves(ctx context.Context, tree *trillian.Tree, leaves []*trillian.LogLeaf, qTimestamp time.Time) ([]*trillian.QueuedLogLeaf, error) {
	results := make([]*trillian.QueuedLogLeaf, len(leaves))
	//TODO(dazwilkin) Is it not permissible to do this as a batch insert?
	var wg sync.WaitGroup
	for i, l := range leaves {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// The insert of the leafdata and the unsequenced work item must happen atomically
			if err := ins.Put(ctx, leafData); err != nil {
				// Handle error
			}
			if err := ins.Put(ctx, unseqTable); err != nil {
				// Handle error
			}
		}()
	}
	// Wait for all the mutations to apply (or fail)
	wg.Wait()

	// Read back any leaves that failed with an already exists error when we tried inserting them
	err = ls.readDupeLeaves(ctx, tree.TreeId, writeDupes, results)
	if err != nil {
		return nil, err
	}
	return results, nil
}
func (ls *logStorage) ReadWriteTransaction(ctx context.Context, tree *trillian.Tree, f storage.LogTXFunc) error {
}
func (ls *logStorage) Snapshot(ctx context.Context) (storage.ReadOnlyLogTX, error) {
}
func (ls *logStorage) SnapshotForTree(ctx context.Context, tree *trillian.Tree) (storage.ReadOnlyLogTreeTX, error) {
}
