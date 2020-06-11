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

package cloudspanner

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/glog"
	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/cache"
	"github.com/google/trillian/storage/cloudspanner/spannerpb"
	"github.com/google/trillian/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	leafDataTbl            = "LeafData"
	seqDataByMerkleHashIdx = "SequenceByMerkleHash"
	seqDataTbl             = "SequencedLeafData"
	unseqTable             = "Unsequenced"

	unsequencedCountSQL = "SELECT Unsequenced.TreeID, COUNT(1) FROM Unsequenced GROUP BY TreeID"

	// t.TreeType: 1 = Log, 3 = PreorderedLog.
	// t.TreeState: 1 = Active, 5 = Draining.
	getActiveLogIDsSQL = `SELECT t.TreeID FROM TreeRoots t
													WHERE (t.TreeType = 1 OR t.TreeType = 3)
													AND (t.TreeState = 1 OR t.TreeState = 5)
													AND t.Deleted=false`
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

var (
	// Spanner DB columns:
	colExtraData               = "ExtraData"
	colLeafValue               = "LeafValue"
	colLeafIdentityHash        = "LeafIdentityHash"
	colMerkleLeafHash          = "MerkleLeafHash"
	colSequenceNumber          = "SequenceNumber"
	colQueueTimestampNanos     = "QueueTimestampNanos"
	colIntegrateTimestampNanos = "IntegrateTimestampNanos"
)

// NewLogStorage initialises and returns a new LogStorage.
func NewLogStorage(client *spanner.Client) storage.LogStorage {
	return NewLogStorageWithOpts(client, LogStorageOptions{})
}

// NewLogStorageWithOpts initialises and returns a new LogStorage.
// The opts parameter can be used to enable custom workarounds.
func NewLogStorageWithOpts(client *spanner.Client, opts LogStorageOptions) storage.LogStorage {
	if opts.DequeueAcrossMerkleBucketsRangeFraction <= 0 || opts.DequeueAcrossMerkleBucketsRangeFraction > 1.0 {
		opts.DequeueAcrossMerkleBucketsRangeFraction = 1.0
	}
	ret := &logStorage{
		ts:   newTreeStorageWithOpts(client, opts.TreeStorageOptions),
		opts: opts,
	}

	return ret
}

// logStorage provides a Cloud Spanner backed trillian.LogStorage implementation.
// See third_party/golang/trillian/storage/log_storage.go for more details.
type logStorage struct {
	// ts provides the merkle-tree level primitives which are built upon by this
	// logStorage.
	ts *treeStorage

	// Additional options applied to this logStorage
	opts LogStorageOptions
}

func (ls *logStorage) CheckDatabaseAccessible(ctx context.Context) error {
	return checkDatabaseAccessible(ctx, ls.ts.client)
}

func (ls *logStorage) Snapshot(ctx context.Context) (storage.ReadOnlyLogTX, error) {
	var staleness spanner.TimestampBound
	if ls.opts.ReadOnlyStaleness > 0 {
		staleness = spanner.ExactStaleness(ls.opts.ReadOnlyStaleness)
	} else {
		staleness = spanner.StrongRead()
	}

	snapshotTX := &snapshotTX{
		client: ls.ts.client,
		stx:    ls.ts.client.ReadOnlyTransaction().WithTimestampBound(staleness),
		ls:     ls,
	}
	return &readOnlyLogTX{snapshotTX}, nil
}

func newLogCache(tree *trillian.Tree) (*cache.SubtreeCache, error) {
	hasher, err := hashers.NewLogHasher(tree.HashStrategy)
	if err != nil {
		return nil, err
	}
	return cache.NewLogSubtreeCache(defLogStrata, hasher), nil
}

func (ls *logStorage) begin(ctx context.Context, tree *trillian.Tree, readonly bool, stx spanRead) (*logTX, error) {
	tx, err := ls.ts.begin(ctx, tree, newLogCache, stx)
	if err != nil {
		return nil, err
	}

	ltx := &logTX{
		ls:       ls,
		dequeued: make(map[string]*QueuedEntry),
		treeTX:   tx,
	}

	// Needed to generate ErrTreeNeedsInit in SnapshotForTree and other methods.
	if err := ltx.getLatestRoot(ctx); err == storage.ErrTreeNeedsInit {
		return ltx, err
	} else if err != nil {
		tx.Rollback()
		return nil, err
	}
	return ltx, nil
}

func (ls *logStorage) BeginForTree(ctx context.Context, treeID int64) (storage.LogTreeTX, error) {
	return nil, ErrNotImplemented
}

func (ls *logStorage) ReadWriteTransaction(ctx context.Context, tree *trillian.Tree, f storage.LogTXFunc) error {
	_, err := ls.ts.client.ReadWriteTransaction(ctx, func(ctx context.Context, stx *spanner.ReadWriteTransaction) error {
		tx, err := ls.begin(ctx, tree, false /* readonly */, stx)
		if err != nil && err != storage.ErrTreeNeedsInit {
			return err
		}
		if err := f(ctx, tx); err != nil {
			return err
		}
		return tx.flushSubtrees(ctx)
	})
	return err
}

func (ls *logStorage) SnapshotForTree(ctx context.Context, tree *trillian.Tree) (storage.ReadOnlyLogTreeTX, error) {
	return ls.begin(ctx, tree, true /* readonly */, ls.ts.client.ReadOnlyTransaction())
}

func (ls *logStorage) QueueLeaves(ctx context.Context, tree *trillian.Tree, leaves []*trillian.LogLeaf, qTimestamp time.Time) ([]*trillian.QueuedLogLeaf, error) {
	_, treeConfig, err := ls.ts.getTreeAndConfig(ctx, tree)
	if err != nil {
		return nil, err
	}
	config, ok := treeConfig.(*spannerpb.LogStorageConfig)
	if !ok {
		return nil, status.Errorf(codes.Internal, "got unexpected config type for Log operation: %T", treeConfig)
	}

	now := time.Now().UTC().Unix()
	bucketPrefix := (now % config.NumUnseqBuckets) << 8

	results := make([]*trillian.QueuedLogLeaf, len(leaves))
	writeDupes := make(map[string][]int)

	qTS := qTimestamp.UnixNano()
	var wg sync.WaitGroup
	for i, l := range leaves {
		wg.Add(1)
		// Capture values of i and l for later reference in the MutationResultFunc below.
		i := i
		l := l
		go func() {
			defer wg.Done()

			// The insert of the leafdata and the unsequenced work item must happen
			// atomically.
			m1 := spanner.Insert(
				leafDataTbl,
				[]string{colTreeID, colLeafIdentityHash, colLeafValue, colExtraData, colQueueTimestampNanos},
				[]interface{}{tree.TreeId, l.LeafIdentityHash, l.LeafValue, l.ExtraData, qTS})
			b := bucketPrefix | int64(l.MerkleLeafHash[0])
			m2 := spanner.Insert(
				unseqTable,
				[]string{colTreeID, colBucket, colQueueTimestampNanos, colMerkleLeafHash, colLeafIdentityHash},
				[]interface{}{tree.TreeId, b, qTS, l.MerkleLeafHash, l.LeafIdentityHash})

			_, err = ls.ts.client.Apply(ctx, []*spanner.Mutation{m1, m2})
			if spanner.ErrCode(err) == codes.AlreadyExists {
				k := string(l.LeafIdentityHash)
				writeDupes[k] = append(writeDupes[k], i)
			} else if err != nil {
				s, _ := status.FromError(err)
				results[i] = &trillian.QueuedLogLeaf{Status: s.Proto()}
			} else {
				results[i] = &trillian.QueuedLogLeaf{Leaf: l} // implicit OK status
			}
		}()
	}

	// Wait for all of our mutations to apply (or fail):
	wg.Wait()

	// Finally, read back any leaves which failed with an already exists error
	// when we tried to insert them:
	err = ls.readDupeLeaves(ctx, tree.TreeId, writeDupes, results)
	if err != nil {
		return nil, err
	}
	return results, nil
}

func (ls *logStorage) AddSequencedLeaves(ctx context.Context, tree *trillian.Tree, leaves []*trillian.LogLeaf, timestamp time.Time) ([]*trillian.QueuedLogLeaf, error) {
	return nil, ErrNotImplemented
}

// readDupeLeaves reads the leaves whose ids are passed as keys in the dupes map,
// and stores them in results.
func (ls *logStorage) readDupeLeaves(ctx context.Context, logID int64, dupes map[string][]int, results []*trillian.QueuedLogLeaf) error {
	numDupes := len(dupes)
	if numDupes == 0 {
		return nil
	}
	glog.V(2).Infof("dupe rowsToRead: %v", numDupes)

	ids := make([][]byte, 0, numDupes)
	for k := range dupes {
		ids = append(ids, []byte(k))
	}
	dupesRead := 0
	tx := ls.ts.client.Single()
	err := readLeaves(ctx, tx, logID, ids, func(l *trillian.LogLeaf) {
		glog.V(2).Infof("Found already exists dupe: %v", l)
		dupesRead++

		indices := dupes[string(l.LeafIdentityHash)]
		glog.V(2).Infof("Indices %v", indices)
		if len(indices) == 0 {
			glog.Warningf("Logic error: Spanner returned a leaf %x, but it matched no requested index", l.LeafIdentityHash)
			return
		}
		for _, i := range indices {
			leaf := l
			results[i] = &trillian.QueuedLogLeaf{
				Leaf:   leaf,
				Status: status.Newf(codes.AlreadyExists, "leaf already exists: %v", l.LeafIdentityHash).Proto(),
			}
		}
	})
	tx.Close()
	if err != nil {
		return err
	}
	if got, want := dupesRead, numDupes; got != want {
		return fmt.Errorf("read unexpected number of dupe rows %d, want %d", got, want)
	}
	return nil
}

// logTX is a concrete implementation of the Trillian storage.LogStorage
// interface.
type logTX struct {
	// treeTX embeds the merkle-tree level transactional actions.
	*treeTX

	// logStorage is the logStorage which begat this logTX.
	ls *logStorage

	// numSequenced holds the number of leaves sequenced by this transaction.
	numSequenced int64

	// dequeued is a map of LeafIdentityHash to QueuedEntry containing entries for
	// everything dequeued by this transaction.
	// This is required to recover the primary key for the unsequenced entry in
	// UpdateSequencedLeaves.
	dequeued map[string]*QueuedEntry
}

func (tx *logTX) getLogStorageConfig() *spannerpb.LogStorageConfig {
	return tx.config.(*spannerpb.LogStorageConfig)
}

// LatestSignedLogRoot returns the freshest SignedLogRoot for this log at the
// time the transaction was started.
func (tx *logTX) LatestSignedLogRoot(ctx context.Context) (*trillian.SignedLogRoot, error) {
	currentSTH, err := tx.currentSTH(ctx)
	if err != nil {
		return nil, err
	}
	writeRev, err := tx.writeRev(ctx)
	if err != nil {
		return nil, err
	}

	if got, want := currentSTH.TreeRevision+1, writeRev; got != want {
		return nil, fmt.Errorf("inconsistency: currentSTH.TreeRevision+1 (%d) != writeRev (%d)", got, want)
	}

	// Put logRoot back together. Fortunately LogRoot has a deterministic serialization.
	logRoot, err := (&types.LogRootV1{
		TimestampNanos: uint64(currentSTH.TsNanos),
		RootHash:       currentSTH.RootHash,
		TreeSize:       uint64(currentSTH.TreeSize),
		Revision:       uint64(currentSTH.TreeRevision),
		Metadata:       currentSTH.Metadata,
	}).MarshalBinary()
	if err != nil {
		return nil, err
	}

	// We already read the latest root as part of starting the transaction (in
	// order to calculate the writeRevision), so we just return that data here:
	return &trillian.SignedLogRoot{
		KeyHint:          types.SerializeKeyHint(tx.treeID),
		LogRoot:          logRoot,
		LogRootSignature: currentSTH.Signature,
	}, nil
}

// StoreSignedLogRoot stores the provided root.
// This method will return an error if the caller attempts to store more than
// one root per log for a given tree size.
func (tx *logTX) StoreSignedLogRoot(ctx context.Context, root *trillian.SignedLogRoot) error {
	writeRev, err := tx.writeRev(ctx)
	if err == storage.ErrTreeNeedsInit {
		writeRev = 0
	} else if err != nil {
		return err
	}

	var logRoot types.LogRootV1
	if err := logRoot.UnmarshalBinary(root.LogRoot); err != nil {
		glog.Warningf("Failed to parse log root: %x %v", root.LogRoot, err)
		return err
	}

	m := spanner.Insert(
		"TreeHeads",
		[]string{
			"TreeID",
			"TimestampNanos",
			"TreeSize",
			"RootHash",
			"RootSignature",
			"TreeRevision",
			"TreeMetadata",
		},
		[]interface{}{
			int64(tx.treeID),
			int64(logRoot.TimestampNanos),
			int64(logRoot.TreeSize),
			logRoot.RootHash,
			root.LogRootSignature,
			writeRev,
			logRoot.Metadata,
		})

	stx, ok := tx.stx.(*spanner.ReadWriteTransaction)
	if !ok {
		return ErrWrongTXType
	}
	return stx.BufferWrite([]*spanner.Mutation{m})
}

func readLeaves(ctx context.Context, stx *spanner.ReadOnlyTransaction, logID int64, ids [][]byte, f func(*trillian.LogLeaf)) error {
	leafTable := leafDataTbl
	cols := []string{colLeafIdentityHash, colLeafValue, colExtraData, colQueueTimestampNanos}
	keys := make([]spanner.KeySet, 0)
	for _, l := range ids {
		keys = append(keys, spanner.Key{logID, l})
	}

	rows := stx.Read(ctx, leafTable, spanner.KeySets(keys...), cols)
	return rows.Do(func(r *spanner.Row) error {
		var l trillian.LogLeaf
		var qTimestamp int64
		if err := r.Columns(&l.LeafIdentityHash, &l.LeafValue, &l.ExtraData, &qTimestamp); err != nil {
			return err
		}
		var err error
		l.QueueTimestamp, err = ptypes.TimestampProto(time.Unix(0, qTimestamp))
		if err != nil {
			return fmt.Errorf("got invalid queue timestamp: %v", err)
		}
		f(&l)
		return nil
	})
}

func (tx *logTX) QueueLeaves(ctx context.Context, leaves []*trillian.LogLeaf, ts time.Time) ([]*trillian.LogLeaf, error) {
	return nil, ErrNotImplemented
}

func (tx *logTX) AddSequencedLeaves(ctx context.Context, leaves []*trillian.LogLeaf, timestamp time.Time) ([]*trillian.QueuedLogLeaf, error) {
	return nil, ErrNotImplemented
}

// DequeueLeaves removes [0, limit) leaves from the to-be-sequenced queue.
// The leaves returned are not guaranteed to be in any particular order.
// The caller should assign sequence numbers and pass the updated leaves as
// arguments to the UpdateSequencedLeaves method.
//
// The LogLeaf structs returned by this method will not be fully populated;
// only the LeafIdentityHash and MerkleLeafHash fields will contain data, this
// should be sufficient for assigning sequence numbers with this storage impl.
//
// TODO(al): cutoff is currently ignored.
func (tx *logTX) DequeueLeaves(ctx context.Context, limit int, cutoff time.Time) ([]*trillian.LogLeaf, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit should be > 0, got %d", limit)
	}

	// Decide which bucket(s) to dequeue from.
	// The high 8 bits of the bucket key is a time based ring - at any given
	// moment, FEs queueing entries will be adding them to different buckets
	// than we're dequeuing from here - the low 8 bits are the first byte of the
	// merkle hash of the entry.
	now := time.Now().UTC()
	cfg := tx.getLogStorageConfig()
	timeBucket := int64(((now.Unix() + cfg.NumUnseqBuckets/2) % cfg.NumUnseqBuckets) << 8)

	// Choose a starting point in the merkle prefix range, and calculate the
	// start/limit of the merkle range we'll dequeue from.
	// It seems to be much better to tune for keeping this range small, and allow
	// the signer to run multiple times per second than try to dequeue a large batch
	// which spans a large number of merkle prefixes.
	merklePrefix := rand.Int63n(256)
	startBucket := timeBucket | merklePrefix
	numMerkleBuckets := int64(256 * tx.ls.opts.DequeueAcrossMerkleBucketsRangeFraction)
	merkleLimit := merklePrefix + numMerkleBuckets
	if merkleLimit > 0xff {
		merkleLimit = 0xff
	}
	limitBucket := timeBucket | merkleLimit

	stmt := spanner.NewStatement(`
			SELECT Bucket, QueueTimestampNanos, MerkleLeafHash, LeafIdentityHash
			FROM Unsequenced u
			WHERE u.TreeID = @tree_id
			AND u.Bucket >= @start_bucket
			AND u.Bucket <= @limit_bucket
			LIMIT @max_num
			`)
	stmt.Params["tree_id"] = tx.treeID
	stmt.Params["start_bucket"] = startBucket
	stmt.Params["limit_bucket"] = limitBucket
	stmt.Params["max_num"] = limit

	ret := make([]*trillian.LogLeaf, 0, limit)
	rows := tx.stx.Query(ctx, stmt)
	if err := rows.Do(func(r *spanner.Row) error {
		var l trillian.LogLeaf
		var qe QueuedEntry
		if err := r.Columns(&qe.bucket, &qe.timestamp, &l.MerkleLeafHash, &l.LeafIdentityHash); err != nil {
			return err
		}

		var err error
		l.QueueTimestamp, err = ptypes.TimestampProto(time.Unix(0, qe.timestamp))
		if err != nil {
			return fmt.Errorf("got invalid queue timestamp: %v", err)
		}
		k := string(l.LeafIdentityHash)
		if tx.dequeued[k] != nil {
			// dupe, user probably called DequeueLeaves more than once.
			return nil
		}

		ret = append(ret, &l)
		qe.leaf = &l
		tx.dequeued[k] = &qe
		return nil
	}); err != nil {
		return nil, err
	}

	return ret, nil
}

// UpdateSequencedLeaves stores the sequence numbers assigned to the leaves,
// and integrates them into the tree.
func (tx *logTX) UpdateSequencedLeaves(ctx context.Context, leaves []*trillian.LogLeaf) error {
	stx, ok := tx.stx.(*spanner.ReadWriteTransaction)
	if !ok {
		return ErrWrongTXType
	}
	// We need the latest root to know what the next sequence number to use below is.
	currentSTH, err := tx.currentSTH(ctx)
	if err != nil {
		return err
	}

	for _, l := range leaves {
		if got, want := l.LeafIndex, currentSTH.TreeSize+tx.numSequenced; got != want {
			return fmt.Errorf("attempting to assign non-sequential leaf with sequence %d, want %d", got, want)
		}

		qe, ok := tx.dequeued[string(l.LeafIdentityHash)]
		if !ok {
			return fmt.Errorf("attempting to assign unknown merkleleafhash %v", l.MerkleLeafHash)
		}

		iTimestamp, err := ptypes.Timestamp(l.IntegrateTimestamp)
		if err != nil {
			return fmt.Errorf("got invalid integrate timestamp: %v", err)
		}

		// Add the sequence mapping...
		m1 := spanner.Insert(seqDataTbl,
			[]string{colTreeID, colSequenceNumber, colLeafIdentityHash, colMerkleLeafHash, colIntegrateTimestampNanos},
			[]interface{}{tx.treeID, l.LeafIndex, l.LeafIdentityHash, l.MerkleLeafHash, iTimestamp.UnixNano()})

		m2 := spanner.Delete(unseqTable, spanner.Key{tx.treeID, qe.bucket, qe.timestamp, l.MerkleLeafHash})

		tx.numSequenced++
		if err := stx.BufferWrite([]*spanner.Mutation{m1, m2}); err != nil {
			return fmt.Errorf("bufferwrite(): %v", err)
		}
	}

	return nil
}

// GetSequencedLeafCount returns the number of leaves integrated into the tree
// at the time the transaction was started.
func (tx *logTX) GetSequencedLeafCount(ctx context.Context) (int64, error) {
	currentSTH, err := tx.currentSTH(ctx)
	if err != nil {
		return -1, err
	}

	return currentSTH.TreeSize, nil
}

// leafmap is a map of LogLeaf by sequence number which knows how to populate
// itself directly from Spanner Rows.
type leafmap map[int64]*trillian.LogLeaf

// addFullRow appends the leaf data in row to the array
func (l leafmap) addFullRow(r *spanner.Row) error {
	var (
		merkleLeafHash, leafValue, extraData []byte
		sequenceNumber                       int64
		leafIDHash                           []byte
		qTimestamp                           int64
		iTimestamp                           int64
	)

	//`SELECT sd.MerkleLeafHash, ld.LeafValue, ld.ExtraData, sd.SequenceNumber, ld.LeafIdentityHash, ld.QueueTimestampNanos, sd.IntegrateTimestampNanos
	if err := r.Columns(&merkleLeafHash, &leafValue, &extraData, &sequenceNumber, &leafIDHash, &qTimestamp, &iTimestamp); err != nil {
		return err
	}
	leaf := &trillian.LogLeaf{
		MerkleLeafHash:   merkleLeafHash,
		LeafValue:        leafValue,
		ExtraData:        extraData,
		LeafIndex:        sequenceNumber,
		LeafIdentityHash: leafIDHash,
	}
	var err error
	leaf.QueueTimestamp, err = ptypes.TimestampProto(time.Unix(0, qTimestamp))
	if err != nil {
		return fmt.Errorf("got invalid queue timestamp %v", err)
	}
	leaf.IntegrateTimestamp, err = ptypes.TimestampProto(time.Unix(0, iTimestamp))
	if err != nil {
		return fmt.Errorf("got invalid integrate timestamp %v", err)
	}

	l[sequenceNumber] = leaf
	return nil
}

// leavesByHash is a map of []LogLeaf (keyed by value hash) which knows how to
// populate itself from Spanner Rows.
type leavesByHash map[string][]*trillian.LogLeaf

// addRow adds the contents of the Spanner Row to this map.
func (b leavesByHash) addRow(r *spanner.Row) error {
	var h []byte
	var v []byte
	var ed []byte
	var qTimestamp int64
	if err := r.Columns(&h, &v, &ed, &qTimestamp); err != nil {
		return err
	}
	queueTimestamp, err := ptypes.TimestampProto(time.Unix(0, qTimestamp))
	if err != nil {
		return fmt.Errorf("got invalid queue timestamp: %v", err)
	}

	leaves, ok := b[string(h)]
	if !ok {
		return fmt.Errorf("inconsistency: unexpected leafValueHash %v", h)
	}
	for i := range leaves {
		if got, want := leaves[i].LeafIdentityHash, h; !bytes.Equal(got, want) {
			return fmt.Errorf("inconsistency: unexpected leafvaluehash %v, want %v", got, want)
		}
		leaves[i].LeafValue = v
		leaves[i].ExtraData = ed
		leaves[i].QueueTimestamp = queueTimestamp
	}
	return nil
}

// populateLeafData populates the partial LogLeaf structs held in the passed in
// map of LeafIdentityHash to []LogLeaf by reading the remaining LogLeaf data from
// Spanner.
// The value of byHash is an []LogLeaf because the underlying leaf data could
// be sequenced into multiple tree leaves if the log allows duplication.
func (tx *logTX) populateLeafData(ctx context.Context, byHash leavesByHash) error {
	keySet := make([]spanner.KeySet, 0, len(byHash))
	for k := range byHash {
		keySet = append(keySet, spanner.Key{tx.treeID, []byte(k)})
	}
	cols := []string{colLeafIdentityHash, colLeafValue, colExtraData, colQueueTimestampNanos}
	rows := tx.stx.Read(ctx, leafDataTbl, spanner.KeySets(keySet...), cols)
	return rows.Do(byHash.addRow)
}

// validateIndices ensures that all indices are between 0 and treeSize-1.
func validateIndices(indices []int64, treeSize int64) error {
	maxIndex := treeSize - 1
	for _, i := range indices {
		if i < 0 {
			return status.Errorf(codes.InvalidArgument, "index %d is < 0", i)
		}
		if i > maxIndex {
			return status.Errorf(codes.OutOfRange, "index %d is > highest index in current tree %d", i, maxIndex)
		}
	}
	return nil
}

// GetLeavesByIndex returns the leaves corresponding to the given indices.
func (tx *logTX) GetLeavesByIndex(ctx context.Context, indices []int64) ([]*trillian.LogLeaf, error) {
	// We need the latest root to validate the indices are within range.
	currentSTH, err := tx.currentSTH(ctx)
	if err != nil {
		return nil, err
	}

	if err := validateIndices(indices, currentSTH.TreeSize); err != nil {
		return nil, err
	}

	leaves := make(leafmap)
	stmt := spanner.NewStatement(
		`SELECT sd.MerkleLeafHash, ld.LeafValue, ld.ExtraData, sd.SequenceNumber, ld.LeafIdentityHash, ld.QueueTimestampNanos, sd.IntegrateTimestampNanos
FROM SequencedLeafData as sd
INNER JOIN LeafData as ld
ON sd.TreeID = ld.TreeID AND sd.LeafIdentityHash = ld.LeafIdentityHash
WHERE sd.TreeID = @tree_id and sd.SequenceNumber IN UNNEST(@seq_nums)`)
	stmt.Params["tree_id"] = tx.treeID
	stmt.Params["seq_nums"] = indices

	rows := tx.stx.Query(ctx, stmt)
	if err := rows.Do(leaves.addFullRow); err != nil {
		return nil, err
	}

	// Sanity check that we got everything we wanted
	if got, want := len(leaves), len(indices); got != want {
		return nil, fmt.Errorf("inconsistency: got %d leaves, want %d", got, want)
	}

	// Sort the leaves so they are in the same order as the indices.
	ret := make([]*trillian.LogLeaf, 0, len(indices))
	for _, i := range indices {
		l, ok := leaves[i]
		if !ok {
			return nil, fmt.Errorf("inconsistency: missing data for index %d", i)
		}
		ret = append(ret, l)
	}

	return ret, nil
}

func validateRange(start, count, treeSize int64) error {
	if count <= 0 {
		return status.Errorf(codes.InvalidArgument, "invalid count %d", count)
	}
	if start < 0 {
		return status.Errorf(codes.InvalidArgument, "invalid start %d", start)
	}
	if start >= treeSize {
		return status.Errorf(codes.OutOfRange, "start index %d beyond tree size %d", start, treeSize)
	}
	return nil
}

// GetLeavesByRange returns the leaves corresponding to the given index range.
func (tx *logTX) GetLeavesByRange(ctx context.Context, start, count int64) ([]*trillian.LogLeaf, error) {
	// We need the latest root to validate the indices are within range.
	currentSTH, err := tx.currentSTH(ctx)
	if err != nil {
		return nil, err
	}

	if err := validateRange(start, count, currentSTH.TreeSize); err != nil {
		return nil, err
	}

	stmt := spanner.NewStatement(
		`SELECT sd.MerkleLeafHash, ld.LeafValue, ld.ExtraData, sd.SequenceNumber, ld.LeafIdentityHash, ld.QueueTimestampNanos, sd.IntegrateTimestampNanos
FROM SequencedLeafData as sd
INNER JOIN LeafData as ld
ON sd.TreeID = ld.TreeID AND sd.LeafIdentityHash = ld.LeafIdentityHash
WHERE sd.TreeID = @tree_id AND sd.SequenceNumber >= @start AND sd.SequenceNumber < @xend`)
	stmt.Params["tree_id"] = tx.treeID
	stmt.Params["start"] = start
	xend := start + count
	if xend > currentSTH.TreeSize {
		xend = currentSTH.TreeSize
		count = xend - start
	}
	stmt.Params["xend"] = xend

	// Results need to be returned in order [start, end), all of which should be
	// available (as we restricted xend/count to TreeSize).
	leaves := make(leafmap)
	rows := tx.stx.Query(ctx, stmt)
	if err := rows.Do(leaves.addFullRow); err != nil {
		return nil, err
	}
	ret := make([]*trillian.LogLeaf, 0, count)
	for i := start; i < (start + count); i++ {
		l, ok := leaves[i]
		if !ok {
			return nil, fmt.Errorf("inconsistency: missing data for index %d", i)
		}
		ret = append(ret, l)
		delete(leaves, i)
	}
	if len(leaves) > 0 {
		return nil, fmt.Errorf("inconsistency: unexpected extra data outside range %d, +%d", start, count)
	}

	return ret, nil
}

// leafSlice is a slice of LogLeaf which knows how to populate itself from
// Spanner Rows.
type leafSlice []*trillian.LogLeaf

// addRow appends the leaf data in Row to the array.
func (l *leafSlice) addRow(r *spanner.Row) error {
	var (
		s      int64
		mh, lh []byte
	)

	if err := r.Columns(&s, &mh, &lh); err != nil {
		return err
	}
	leaf := trillian.LogLeaf{
		LeafIndex:        s,
		MerkleLeafHash:   mh,
		LeafIdentityHash: lh,
	}
	*l = append(*l, &leaf)
	return nil
}

// getUsingIndex returns a slice containing the LogLeaf structs corresponding
// to the requested keys.
// The entries in key are used in constructing a primary key (treeID, keyElem)
// for the specified Spanner index.
// If bySeq is true, the returned slice will be order by LogLeaf.LeafIndex.
func (tx *logTX) getUsingIndex(ctx context.Context, idx string, keys [][]byte, bySeq bool) ([]*trillian.LogLeaf, error) {
	keySet := make([]spanner.KeySet, 0, len(keys))
	for _, k := range keys {
		keySet = append(keySet, spanner.Key{tx.treeID, k})
	}

	leaves := make(leafSlice, 0, len(keys))
	cols := []string{colSequenceNumber, colMerkleLeafHash, colLeafIdentityHash}
	rows := tx.stx.ReadUsingIndex(ctx, seqDataTbl, idx, spanner.KeySets(keySet...), cols)
	if err := rows.Do(leaves.addRow); err != nil {
		return nil, err
	}

	byHash := make(leavesByHash)
	for i := range leaves {
		k := string(leaves[i].LeafIdentityHash)
		byHash[k] = append(byHash[k], leaves[i])
	}

	// Now we can fetch & combine the actual leaf data:
	if err := tx.populateLeafData(ctx, byHash); err != nil {
		return nil, err
	}

	if bySeq {
		sort.Sort(byIndex(leaves))
	}

	return leaves, nil
}

// GetLeavesByHash returns the leaves corresponding to the given merkle hashes.
// Any unknown hashes will simply be ignored, and the caller should inspect the
// returned leaves to determine whether this has occurred.
// TODO(al): Currently, this method does not populate the IntegrateTimestamp
//   member of the returned leaves. We should convert this method to use SQL
//   rather than denormalising IntegrateTimestampNanos into the index too.
func (tx *logTX) GetLeavesByHash(ctx context.Context, hashes [][]byte, bySeq bool) ([]*trillian.LogLeaf, error) {
	return tx.getUsingIndex(ctx, seqDataByMerkleHashIdx, hashes, bySeq)
}

// QueuedEntry represents a leaf which was dequeued.
// It's used to store some extra info which is necessary for rebuilding the
// leaf's primary key when it's passed back in to UpdateSequencedLeaves.
type QueuedEntry struct {
	// leaf is partially populated with the Merkle and LeafValue hashes only.
	leaf      *trillian.LogLeaf
	bucket    int64
	timestamp int64
}

// readOnlyLogTX implements storage.ReadOnlyLogTX.
type readOnlyLogTX struct {
	*snapshotTX
}

func (tx *readOnlyLogTX) GetActiveLogIDs(ctx context.Context) ([]int64, error) {
	tx.mu.RLock()
	defer tx.mu.RUnlock()
	if tx.stx == nil {
		return nil, ErrTransactionClosed
	}

	ids := []int64{}
	// We have to use SQL as Read() doesn't work against an index.
	stmt := spanner.NewStatement(getActiveLogIDsSQL)
	rows := tx.stx.Query(ctx, stmt)
	if err := rows.Do(func(r *spanner.Row) error {
		var id int64
		if err := r.Columns(&id); err != nil {
			return err
		}
		ids = append(ids, id)
		return nil
	}); err != nil {
		glog.Warningf("GetActiveLogIDs: %v", err)
		return nil, fmt.Errorf("problem executing getActiveLogIDsSQL: %v", err)
	}
	return ids, nil
}

func (tx *readOnlyLogTX) GetUnsequencedCounts(ctx context.Context) (storage.CountByLogID, error) {
	stmt := spanner.NewStatement(unsequencedCountSQL)
	ret := make(storage.CountByLogID)
	rows := tx.stx.Query(ctx, stmt)
	if err := rows.Do(func(r *spanner.Row) error {
		var id, c int64
		if err := r.Columns(&id, &c); err != nil {
			return err
		}
		ret[id] = c
		return nil
	}); err != nil {
		return nil, fmt.Errorf("problem executing unsequencedCountSQL: %v", err)
	}
	return ret, nil
}

// LogLeaf sorting boilerplate below.

type byIndex []*trillian.LogLeaf

func (b byIndex) Len() int { return len(b) }

func (b byIndex) Swap(i, j int) { b[i], b[j] = b[j], b[i] }

func (b byIndex) Less(i, j int) bool { return b[i].LeafIndex < b[j].LeafIndex }
