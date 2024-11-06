// Copyright 2024 Trillian Authors. All Rights Reserved.
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

package postgresql

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/cache"
	"github.com/google/trillian/storage/tree"
	"github.com/google/trillian/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/transparency-dev/merkle/compact"
	"github.com/transparency-dev/merkle/rfc6962"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/klog/v2"
)

const (
	createTempQueueLeavesTable = "CREATE TEMP TABLE TempQueueLeaves (" +
		" TreeId BIGINT," +
		" LeafIdentityHash BYTEA," +
		" LeafValue BYTEA," +
		" ExtraData BYTEA," +
		" MerkleLeafHash BYTEA," +
		" QueueTimestampNanos BIGINT," +
		" QueueID BYTEA," +
		" IsDuplicate BOOLEAN DEFAULT FALSE" +
		") ON COMMIT DROP"
	queueLeavesSQL = "SELECT * FROM queue_leaves()"

	createTempAddSequencedLeavesTable = "CREATE TEMP TABLE TempAddSequencedLeaves (" +
		" TreeId BIGINT," +
		" LeafIdentityHash BYTEA," +
		" LeafValue BYTEA," +
		" ExtraData BYTEA," +
		" MerkleLeafHash BYTEA," +
		" QueueTimestampNanos BIGINT," +
		" SequenceNumber BIGINT," +
		" IsDuplicateLeafData BOOLEAN DEFAULT FALSE," +
		" IsDuplicateSequencedLeafData BOOLEAN DEFAULT FALSE" +
		") ON COMMIT DROP"
	addSequencedLeavesSQL = "SELECT * FROM add_sequenced_leaves()"

	selectNonDeletedTreeIDByTypeAndStateSQL = "SELECT TreeId " +
		"FROM Trees " +
		"WHERE TreeType IN($1,$2)" +
		" AND TreeState IN($3,$4)" +
		" AND (Deleted IS NULL OR Deleted='false')"

	selectLatestSignedLogRootSQL = "SELECT TreeHeadTimestamp,TreeSize,RootHash,RootSignature " +
		"FROM TreeHead " +
		"WHERE TreeId=$1 " +
		"ORDER BY TreeHeadTimestamp DESC " +
		"LIMIT 1"

	selectLeavesByRangeSQL = "SELECT s.MerkleLeafHash,l.LeafIdentityHash,l.LeafValue,s.SequenceNumber,l.ExtraData,l.QueueTimestampNanos,s.IntegrateTimestampNanos " +
		"FROM SequencedLeafData s" +
		" INNER JOIN LeafData l ON (s.LeafIdentityHash=l.LeafIdentityHash AND s.TreeId=l.TreeId) " +
		"WHERE s.SequenceNumber>=$1" +
		" AND s.SequenceNumber<$2" +
		" AND l.TreeId=$3" + orderBySequenceNumberSQL

	selectLeavesByMerkleHashSQL = "SELECT s.MerkleLeafHash,l.LeafIdentityHash,l.LeafValue,s.SequenceNumber,l.ExtraData,l.QueueTimestampNanos,s.IntegrateTimestampNanos " +
		"FROM SequencedLeafData s" +
		" INNER JOIN LeafData l ON (s.LeafIdentityHash=l.LeafIdentityHash AND s.TreeId=l.TreeId) " +
		"WHERE s.MerkleLeafHash=ANY($1)" +
		" AND l.TreeId=$2"
	// TODO(robstradling): Per #1548, rework the code so the dummy hash isn't needed (e.g. this assumes hash size is 32)
	dummyMerkleLeafHash = "00000000000000000000000000000000"
	// This statement returns a dummy Merkle leaf hash value (which must be
	// of the right size) so that its signature matches that of the other
	// leaf-selection statements.
	selectLeavesByLeafIdentityHashSQL = "SELECT decode('" + dummyMerkleLeafHash + "','escape'),l.LeafIdentityHash,l.LeafValue,-1,l.ExtraData,l.QueueTimestampNanos,s.IntegrateTimestampNanos " +
		"FROM LeafData l" +
		" LEFT JOIN SequencedLeafData s ON (l.LeafIdentityHash=s.LeafIdentityHash AND l.TreeId=s.TreeId) " +
		"WHERE l.LeafIdentityHash=ANY($1)" +
		" AND l.TreeId=$2"

	// Same as above except with leaves ordered by sequence so we only incur this cost when necessary
	orderBySequenceNumberSQL                     = " ORDER BY s.SequenceNumber"
	selectLeavesByMerkleHashOrderedBySequenceSQL = selectLeavesByMerkleHashSQL + orderBySequenceNumberSQL

	logIDLabel = "logid"
)

var (
	once             sync.Once
	queuedCounter    monitoring.Counter
	queuedDupCounter monitoring.Counter
	dequeuedCounter  monitoring.Counter

	dequeueLatency       monitoring.Histogram
	dequeueSelectLatency monitoring.Histogram
	dequeueRemoveLatency monitoring.Histogram
)

func createMetrics(mf monitoring.MetricFactory) {
	queuedCounter = mf.NewCounter("postgresql_queued_leaves", "Number of leaves queued", logIDLabel)
	queuedDupCounter = mf.NewCounter("postgresql_queued_dup_leaves", "Number of duplicate leaves queued", logIDLabel)
	dequeuedCounter = mf.NewCounter("postgresql_dequeued_leaves", "Number of leaves dequeued", logIDLabel)

	dequeueLatency = mf.NewHistogram("postgresql_dequeue_leaves_latency", "Latency of dequeue leaves operation in seconds", logIDLabel)
	dequeueSelectLatency = mf.NewHistogram("postgresql_dequeue_leaves_latency_select", "Latency of selection part of dequeue leaves operation in seconds", logIDLabel)
	dequeueRemoveLatency = mf.NewHistogram("postgresql_dequeue_leaves_latency_remove", "Latency of removal part of dequeue leaves operation in seconds", logIDLabel)
}

func labelForTX(t *logTreeTX) string {
	return strconv.FormatInt(t.treeID, 10)
}

func observe(hist monitoring.Histogram, duration time.Duration, label string) {
	hist.Observe(duration.Seconds(), label)
}

type postgreSQLLogStorage struct {
	*postgreSQLTreeStorage
	admin         storage.AdminStorage
	metricFactory monitoring.MetricFactory
}

// NewLogStorage creates a storage.LogStorage instance for the specified PostgreSQL URL.
// It assumes storage.AdminStorage is backed by the same PostgreSQL database as well.
func NewLogStorage(db *pgxpool.Pool, mf monitoring.MetricFactory) storage.LogStorage {
	if mf == nil {
		mf = monitoring.InertMetricFactory{}
	}
	return &postgreSQLLogStorage{
		admin:                 NewAdminStorage(db),
		postgreSQLTreeStorage: newTreeStorage(db),
		metricFactory:         mf,
	}
}

func (m *postgreSQLLogStorage) CheckDatabaseAccessible(ctx context.Context) error {
	return m.db.Ping(ctx)
}

func (m *postgreSQLLogStorage) GetActiveLogIDs(ctx context.Context) ([]int64, error) {
	// Include logs that are DRAINING in the active list as we're still
	// integrating leaves into them.
	rows, err := m.db.Query(
		ctx, selectNonDeletedTreeIDByTypeAndStateSQL,
		trillian.TreeType_LOG.String(), trillian.TreeType_PREORDERED_LOG.String(),
		trillian.TreeState_ACTIVE.String(), trillian.TreeState_DRAINING.String())
	if err != nil {
		return nil, err
	}
	defer func() {
		rows.Close()
		if err := rows.Err(); err != nil {
			klog.Errorf("rows.Err(): %v", err)
		}
	}()
	ids := []int64{}
	for rows.Next() {
		var treeID int64
		if err := rows.Scan(&treeID); err != nil {
			return nil, err
		}
		ids = append(ids, treeID)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func (m *postgreSQLLogStorage) beginInternal(ctx context.Context, tree *trillian.Tree) (*logTreeTX, error) {
	once.Do(func() {
		createMetrics(m.metricFactory)
	})

	stCache := cache.NewLogSubtreeCache(rfc6962.DefaultHasher)
	ttx, err := m.beginTreeTx(ctx, tree, rfc6962.DefaultHasher.Size(), stCache)
	if err != nil && err != storage.ErrTreeNeedsInit {
		return nil, err
	}

	ltx := &logTreeTX{
		treeTX:   ttx,
		ls:       m,
		dequeued: make(map[string]dequeuedLeaf),
	}
	ltx.slr, err = ltx.fetchLatestRoot(ctx)
	if err == storage.ErrTreeNeedsInit {
		return ltx, err
	} else if err != nil {
		if err := ttx.Close(); err != nil {
			klog.Errorf("ttx.Close(): %v", err)
		}
		return nil, err
	}

	if err := ltx.root.UnmarshalBinary(ltx.slr.LogRoot); err != nil {
		if err := ttx.Close(); err != nil {
			klog.Errorf("ttx.Close(): %v", err)
		}
		return nil, err
	}

	return ltx, nil
}

// TODO(robstradling): This and many other methods of this storage
// implementation can leak a specific sql.ErrTxDone all the way to the client,
// if the transaction is rolled back as a result of a canceled context. It must
// return "generic" errors, and only log the specific ones for debugging.
func (m *postgreSQLLogStorage) ReadWriteTransaction(ctx context.Context, tree *trillian.Tree, f storage.LogTXFunc) error {
	tx, err := m.beginInternal(ctx, tree)
	if err != nil && err != storage.ErrTreeNeedsInit {
		return err
	}
	defer func() {
		if err := tx.Close(); err != nil {
			klog.Errorf("tx.Close(): %v", err)
		}
	}()
	if err := f(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (m *postgreSQLLogStorage) AddSequencedLeaves(ctx context.Context, tree *trillian.Tree, leaves []*trillian.LogLeaf, timestamp time.Time) ([]*trillian.QueuedLogLeaf, error) {
	tx, err := m.beginInternal(ctx, tree)
	if tx != nil {
		// Ensure we don't leak the transaction. For example if we get an
		// ErrTreeNeedsInit from beginInternal() or if AddSequencedLeaves fails
		// below.
		defer func() {
			if err := tx.Close(); err != nil {
				klog.Errorf("tx.Close(): %v", err)
			}
		}()
	}
	if err != nil {
		return nil, err
	}
	res, err := tx.AddSequencedLeaves(ctx, leaves, timestamp)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return res, nil
}

func (m *postgreSQLLogStorage) SnapshotForTree(ctx context.Context, tree *trillian.Tree) (storage.ReadOnlyLogTreeTX, error) {
	tx, err := m.beginInternal(ctx, tree)
	if err != nil && err != storage.ErrTreeNeedsInit {
		return nil, err
	}
	return tx, err
}

func (m *postgreSQLLogStorage) QueueLeaves(ctx context.Context, tree *trillian.Tree, leaves []*trillian.LogLeaf, queueTimestamp time.Time) ([]*trillian.QueuedLogLeaf, error) {
	tx, err := m.beginInternal(ctx, tree)
	if tx != nil {
		// Ensure we don't leak the transaction. For example if we get an
		// ErrTreeNeedsInit from beginInternal() or if QueueLeaves fails
		// below.
		defer func() {
			if err := tx.Close(); err != nil {
				klog.Errorf("tx.Close(): %v", err)
			}
		}()
	}
	if err != nil {
		return nil, err
	}
	existing, err := tx.QueueLeaves(ctx, leaves, queueTimestamp)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	ret := make([]*trillian.QueuedLogLeaf, len(leaves))
	for i, e := range existing {
		if e != nil {
			ret[i] = &trillian.QueuedLogLeaf{
				Leaf:   e,
				Status: status.Newf(codes.AlreadyExists, "leaf already exists: %v", e.LeafIdentityHash).Proto(),
			}
			continue
		}
		ret[i] = &trillian.QueuedLogLeaf{Leaf: leaves[i]}
	}
	return ret, nil
}

type logTreeTX struct {
	treeTX
	ls       *postgreSQLLogStorage
	root     types.LogRootV1
	slr      *trillian.SignedLogRoot
	dequeued map[string]dequeuedLeaf
}

// GetMerkleNodes returns the requested nodes.
func (t *logTreeTX) GetMerkleNodes(ctx context.Context, ids []compact.NodeID) ([]tree.Node, error) {
	t.treeTX.mu.Lock()
	defer t.treeTX.mu.Unlock()
	return t.subtreeCache.GetNodes(ids, t.getSubtreesFunc(ctx))
}

func (t *logTreeTX) DequeueLeaves(ctx context.Context, limit int, cutoffTime time.Time) ([]*trillian.LogLeaf, error) {
	t.treeTX.mu.Lock()
	defer t.treeTX.mu.Unlock()

	if t.treeType == trillian.TreeType_PREORDERED_LOG {
		// TODO(robstradling): Optimize this by fetching only the required
		// fields of LogLeaf. We can avoid joining with LeafData table here.
		return t.getLeavesByRangeInternal(ctx, int64(t.root.TreeSize), int64(limit))
	}

	start := time.Now()

	leaves := make([]*trillian.LogLeaf, 0, limit)
	rows, err := t.tx.Query(ctx, selectQueuedLeavesSQL, t.treeID, cutoffTime.UnixNano(), limit)
	if err != nil {
		klog.Warningf("Failed to select rows for work: %s", err)
		return nil, err
	}
	defer func() {
		rows.Close()
		if err := rows.Err(); err != nil {
			klog.Errorf("rows.Err(): %v", err)
		}
	}()

	for rows.Next() {
		leaf, dqInfo, err := t.dequeueLeaf(rows)
		if err != nil {
			klog.Warningf("Error dequeuing leaf: %v", err)
			return nil, err
		}

		if len(leaf.LeafIdentityHash) != t.hashSizeBytes {
			return nil, errors.New("dequeued a leaf with incorrect hash size")
		}

		k := string(leaf.LeafIdentityHash)
		if _, ok := t.dequeued[k]; ok {
			// dupe, user probably called DequeueLeaves more than once.
			continue
		}
		t.dequeued[k] = dqInfo
		leaves = append(leaves, leaf)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	label := labelForTX(t)
	observe(dequeueSelectLatency, time.Since(start), label)
	observe(dequeueLatency, time.Since(start), label)
	dequeuedCounter.Add(float64(len(leaves)), label)

	return leaves, nil
}

func (t *logTreeTX) QueueLeaves(ctx context.Context, leaves []*trillian.LogLeaf, queueTimestamp time.Time) ([]*trillian.LogLeaf, error) {
	t.treeTX.mu.Lock()
	defer t.treeTX.mu.Unlock()

	// Prepare rows to copy, but don't accept batches if any of the leaves are invalid.
	leafMap := make(map[string]int)
	copyRows := make([][]interface{}, 0, len(leaves))
	for i, leaf := range leaves {
		if len(leaf.LeafIdentityHash) != t.hashSizeBytes {
			return nil, fmt.Errorf("queued leaf must have a leaf ID hash of length %d", t.hashSizeBytes)
		}
		leaf.QueueTimestamp = timestamppb.New(queueTimestamp)
		if err := leaf.QueueTimestamp.CheckValid(); err != nil {
			return nil, fmt.Errorf("got invalid queue timestamp: %w", err)
		}
		qTimestamp := leaf.QueueTimestamp.AsTime()
		args := queueArgs(t.treeID, leaf.LeafIdentityHash, qTimestamp)
		copyRows = append(copyRows, []interface{}{t.treeID, leaf.LeafIdentityHash, leaf.LeafValue, leaf.ExtraData, leaf.MerkleLeafHash, args[0], args[1]})
		leafMap[hex.EncodeToString(leaf.LeafIdentityHash)] = i
	}
	label := labelForTX(t)

	// Create temporary table.
	_, err := t.tx.Exec(ctx, createTempQueueLeavesTable)
	if err != nil {
		klog.Warningf("Failed to create tempqueueleaves table: %s", err)
		return nil, postgresqlToGRPC(err)
	}

	// Copy rows to temporary table.
	_, err = t.tx.CopyFrom(
		ctx,
		pgx.Identifier{"tempqueueleaves"},
		[]string{"treeid", "leafidentityhash", "leafvalue", "extradata", "merkleleafhash", "queuetimestampnanos", "queueid"},
		pgx.CopyFromRows(copyRows),
	)
	if err != nil {
		klog.Warningf("Failed to copy queued leaves: %s", err)
		return nil, postgresqlToGRPC(err)
	}

	// Create the leaf data records, work queue entries, and obtain a deduplicated list of existing leaves.
	existingCount := 0
	existingLeaves := make([]*trillian.LogLeaf, len(leaves))
	var toRetrieve [][]byte
	var leafIdentityHash []byte
	if rows, err := t.tx.Query(ctx, queueLeavesSQL); err != nil {
		klog.Warningf("Failed to queue leaves: %s", err)
		return nil, postgresqlToGRPC(err)
	} else {
		defer rows.Close()
		for rows.Next() {
			if err = rows.Scan(&leafIdentityHash); err != nil {
				klog.Warningf("Failed to scan row: %s", err)
				return nil, postgresqlToGRPC(err)
			}

			if i, ok := leafMap[hex.EncodeToString(leafIdentityHash)]; !ok {
				klog.Warningf("Unexpected leafIdentityHash: %s", hex.EncodeToString(leafIdentityHash))
				return nil, postgresqlToGRPC(err)
			} else {
				// Remember the duplicate leaf, using the requested leaf for now.
				existingLeaves[i] = leaves[i]
				existingCount++
				queuedDupCounter.Inc(label)
			}
			toRetrieve = append(toRetrieve, leafIdentityHash)
		}
		if rows.Err() != nil {
			klog.Errorf("Failed processing rows: %s", err)
			return nil, postgresqlToGRPC(err)
		}
	}
	queuedCounter.Add(float64(len(leaves)), label)

	if existingCount == 0 {
		return existingLeaves, nil
	}

	results, err := t.getLeafDataByIdentityHash(ctx, toRetrieve)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve existing leaves: %v", err)
	}
	if len(results) != len(toRetrieve) {
		return nil, fmt.Errorf("failed to retrieve all existing leaves: got %d, want %d", len(results), len(toRetrieve))
	}
	// Replace the requested leaves with the actual leaves.
	for i, requested := range existingLeaves {
		if requested == nil {
			continue
		}
		found := false
		for _, result := range results {
			if bytes.Equal(result.LeafIdentityHash, requested.LeafIdentityHash) {
				existingLeaves[i] = result
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("failed to find existing leaf for hash %x", requested.LeafIdentityHash)
		}
	}

	return existingLeaves, nil
}

func (t *logTreeTX) AddSequencedLeaves(ctx context.Context, leaves []*trillian.LogLeaf, timestamp time.Time) ([]*trillian.QueuedLogLeaf, error) {
	t.treeTX.mu.Lock()
	defer t.treeTX.mu.Unlock()

	res := make([]*trillian.QueuedLogLeaf, len(leaves))
	ok := status.New(codes.OK, "OK").Proto()

	// Prepare rows to copy.
	leafMap := make(map[string]int)
	copyRows := make([][]interface{}, 0, len(leaves))
	for i, leaf := range leaves {
		// This should fail on insert, but catch it early.
		if got, want := len(leaf.LeafIdentityHash), t.hashSizeBytes; got != want {
			return nil, status.Errorf(codes.FailedPrecondition, "leaves[%d] has incorrect hash size %d, want %d", i, got, want)
		}

		copyRows = append(copyRows, []interface{}{t.treeID, leaf.LeafIdentityHash, leaf.LeafValue, leaf.ExtraData, leaf.MerkleLeafHash, timestamp.UnixNano(), leaf.LeafIndex})
		leafMap[hex.EncodeToString(leaf.LeafIdentityHash)] = i
		res[i] = &trillian.QueuedLogLeaf{Status: ok}
	}

	// Create temporary table.
	_, err := t.tx.Exec(ctx, createTempAddSequencedLeavesTable)
	if err != nil {
		klog.Warningf("Failed to create tempaddsequencedleaves table: %s", err)
		return nil, postgresqlToGRPC(err)
	}

	// Copy rows to temporary table.
	_, err = t.tx.CopyFrom(
		ctx,
		pgx.Identifier{"tempaddsequencedleaves"},
		[]string{"treeid", "leafidentityhash", "leafvalue", "extradata", "merkleleafhash", "queuetimestampnanos", "sequencenumber"},
		pgx.CopyFromRows(copyRows),
	)
	if err != nil {
		klog.Warningf("Failed to copy sequenced leaves: %s", err)
		return nil, postgresqlToGRPC(err)
	}

	// Create the leaf data records and sequenced leaf data records, returning details of which records already existed.
	if rows, err := t.tx.Query(ctx, addSequencedLeavesSQL); err != nil {
		klog.Warningf("Failed to add sequenced leaves: %s", err)
		return nil, postgresqlToGRPC(err)
	} else {
		defer rows.Close()
		for rows.Next() {
			var leafIdentityHash []byte
			var isDuplicateLeafData, isDuplicateSequencedLeafData bool
			if err = rows.Scan(&leafIdentityHash, &isDuplicateLeafData, &isDuplicateSequencedLeafData); err != nil {
				klog.Warningf("Failed to scan row: %s", err)
				return nil, postgresqlToGRPC(err)
			}

			if i, ok := leafMap[hex.EncodeToString(leafIdentityHash)]; !ok {
				klog.Warningf("Unexpected leafIdentityHash: %s", hex.EncodeToString(leafIdentityHash))
				return nil, postgresqlToGRPC(err)
			} else if isDuplicateLeafData {
				res[i].Status = status.New(codes.FailedPrecondition, "conflicting LeafIdentityHash").Proto()
			} else if isDuplicateSequencedLeafData {
				res[i].Status = status.New(codes.FailedPrecondition, "conflicting LeafIndex").Proto()
			}
		}
		if rows.Err() != nil {
			klog.Errorf("Error processing rows: %s", err)
			return nil, postgresqlToGRPC(err)
		}
	}

	// TODO(robstradling): Support opting out from duplicates detection.
	// TODO(robstradling): Update IntegrateTimestamp on integrating the leaf.
	// TODO(robstradling): Load LeafData for conflicting entries.

	return res, nil
}

func (t *logTreeTX) GetLeavesByRange(ctx context.Context, start, count int64) ([]*trillian.LogLeaf, error) {
	t.treeTX.mu.Lock()
	defer t.treeTX.mu.Unlock()
	return t.getLeavesByRangeInternal(ctx, start, count)
}

func (t *logTreeTX) getLeavesByRangeInternal(ctx context.Context, start, count int64) ([]*trillian.LogLeaf, error) {
	if count <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "invalid count %d, want > 0", count)
	}
	if start < 0 {
		return nil, status.Errorf(codes.InvalidArgument, "invalid start %d, want >= 0", start)
	}

	if t.treeType == trillian.TreeType_LOG {
		treeSize := int64(t.root.TreeSize)
		if treeSize <= 0 {
			return nil, status.Errorf(codes.OutOfRange, "empty tree")
		} else if start >= treeSize {
			return nil, status.Errorf(codes.OutOfRange, "invalid start %d, want < TreeSize(%d)", start, treeSize)
		}
		// Ensure no entries queried/returned beyond the tree.
		if maxCount := treeSize - start; count > maxCount {
			count = maxCount
		}
	}
	// TODO(robstradling): Further clip `count` to a safe upper bound like 64k.

	rows, err := t.tx.Query(ctx, selectLeavesByRangeSQL, start, start+count, t.treeID)
	if err != nil {
		klog.Warningf("Failed to get leaves by range: %s", err)
		return nil, err
	}
	defer func() {
		rows.Close()
		if err := rows.Err(); err != nil {
			klog.Errorf("rows.Err(): %v", err)
		}
	}()

	ret := make([]*trillian.LogLeaf, 0, count)
	for wantIndex := start; rows.Next(); wantIndex++ {
		leaf := &trillian.LogLeaf{}
		var qTimestamp, iTimestamp int64
		if err := rows.Scan(
			&leaf.MerkleLeafHash,
			&leaf.LeafIdentityHash,
			&leaf.LeafValue,
			&leaf.LeafIndex,
			&leaf.ExtraData,
			&qTimestamp,
			&iTimestamp); err != nil {
			klog.Warningf("Failed to scan merkle leaves: %s", err)
			return nil, err
		}
		if leaf.LeafIndex != wantIndex {
			if wantIndex < int64(t.root.TreeSize) {
				return nil, fmt.Errorf("got unexpected index %d, want %d", leaf.LeafIndex, wantIndex)
			}
			break
		}
		leaf.QueueTimestamp = timestamppb.New(time.Unix(0, qTimestamp))
		if err := leaf.QueueTimestamp.CheckValid(); err != nil {
			return nil, fmt.Errorf("got invalid queue timestamp: %w", err)
		}
		leaf.IntegrateTimestamp = timestamppb.New(time.Unix(0, iTimestamp))
		if err := leaf.IntegrateTimestamp.CheckValid(); err != nil {
			return nil, fmt.Errorf("got invalid integrate timestamp: %w", err)
		}
		ret = append(ret, leaf)
	}
	if err := rows.Err(); err != nil {
		klog.Warningf("Failed to read returned leaves: %s", err)
		return nil, err
	}

	return ret, nil
}

func (t *logTreeTX) GetLeavesByHash(ctx context.Context, leafHashes [][]byte, orderBySequence bool) ([]*trillian.LogLeaf, error) {
	t.treeTX.mu.Lock()
	defer t.treeTX.mu.Unlock()

	var query string
	if orderBySequence {
		query = selectLeavesByMerkleHashOrderedBySequenceSQL
	} else {
		query = selectLeavesByMerkleHashSQL
	}

	return t.getLeavesByHashInternal(ctx, leafHashes, query, "merkle")
}

// getLeafDataByIdentityHash retrieves leaf data by LeafIdentityHash, returned
// as a slice of LogLeaf objects for convenience.  However, note that the
// returned LogLeaf objects will not have a valid MerkleLeafHash, LeafIndex, or IntegrateTimestamp.
func (t *logTreeTX) getLeafDataByIdentityHash(ctx context.Context, leafHashes [][]byte) ([]*trillian.LogLeaf, error) {
	return t.getLeavesByHashInternal(ctx, leafHashes, selectLeavesByLeafIdentityHashSQL, "leaf-identity")
}

func (t *logTreeTX) LatestSignedLogRoot(ctx context.Context) (*trillian.SignedLogRoot, error) {
	t.treeTX.mu.Lock()
	defer t.treeTX.mu.Unlock()

	if t.slr == nil {
		return nil, storage.ErrTreeNeedsInit
	}

	return t.slr, nil
}

// fetchLatestRoot reads the latest root from the DB.
func (t *logTreeTX) fetchLatestRoot(ctx context.Context) (*trillian.SignedLogRoot, error) {
	var timestamp, treeSize int64
	var rootHash, rootSignatureBytes []byte
	if err := t.tx.QueryRow(
		ctx, selectLatestSignedLogRootSQL, t.treeID).Scan(
		&timestamp, &treeSize, &rootHash, &rootSignatureBytes,
	); err == pgx.ErrNoRows {
		// It's possible there are no roots for this tree yet
		return nil, storage.ErrTreeNeedsInit
	}

	// Put logRoot back together. Fortunately LogRoot has a deterministic serialization.
	logRoot, err := (&types.LogRootV1{
		RootHash:       rootHash,
		TimestampNanos: uint64(timestamp),
		TreeSize:       uint64(treeSize),
	}).MarshalBinary()
	if err != nil {
		return nil, err
	}

	return &trillian.SignedLogRoot{LogRoot: logRoot}, nil
}

func (t *logTreeTX) StoreSignedLogRoot(ctx context.Context, root *trillian.SignedLogRoot) error {
	t.treeTX.mu.Lock()
	defer t.treeTX.mu.Unlock()

	var logRoot types.LogRootV1
	if err := logRoot.UnmarshalBinary(root.LogRoot); err != nil {
		klog.Warningf("Failed to parse log root: %x %v", root.LogRoot, err)
		return err
	}
	if len(logRoot.Metadata) != 0 {
		return fmt.Errorf("unimplemented: postgresql storage does not support log root metadata")
	}

	res, err := t.tx.Exec(
		ctx,
		insertTreeHeadSQL,
		t.treeID,
		logRoot.TimestampNanos,
		logRoot.TreeSize,
		logRoot.RootHash,
		[]byte{})
	if err != nil {
		klog.Warningf("Failed to store signed root: %s", err)
	}

	return checkResultOkAndRowCountIs(res, err, 1)
}

func (t *logTreeTX) getLeavesByHashInternal(ctx context.Context, leafHashes [][]byte, query string, desc string) ([]*trillian.LogLeaf, error) {
	rows, err := t.tx.Query(ctx, query, leafHashes, t.treeID)
	if err != nil {
		klog.Warningf("Query() %s hash = %v", desc, err)
		return nil, err
	}
	defer func() {
		rows.Close()
		if err := rows.Err(); err != nil {
			klog.Errorf("rows.Err(): %v", err)
		}
	}()

	// The tree could include duplicates so we don't know how many results will be returned
	var ret []*trillian.LogLeaf
	for rows.Next() {
		leaf := &trillian.LogLeaf{}
		// We might be using a LEFT JOIN in our statement, so leaves which are
		// queued but not yet integrated will have a NULL IntegrateTimestamp
		// when there's no corresponding entry in SequencedLeafData, even though
		// the table definition forbids that, so we use a nullable type here and
		// check its validity below.
		var integrateTS sql.NullInt64
		var queueTS int64

		if err := rows.Scan(&leaf.MerkleLeafHash, &leaf.LeafIdentityHash, &leaf.LeafValue, &leaf.LeafIndex, &leaf.ExtraData, &queueTS, &integrateTS); err != nil {
			klog.Warningf("LogID: %d Scan() %s = %s", t.treeID, desc, err)
			return nil, err
		}
		leaf.QueueTimestamp = timestamppb.New(time.Unix(0, queueTS))
		if err := leaf.QueueTimestamp.CheckValid(); err != nil {
			return nil, fmt.Errorf("got invalid queue timestamp: %w", err)
		}
		if integrateTS.Valid {
			leaf.IntegrateTimestamp = timestamppb.New(time.Unix(0, integrateTS.Int64))
			if err := leaf.IntegrateTimestamp.CheckValid(); err != nil {
				return nil, fmt.Errorf("got invalid integrate timestamp: %w", err)
			}
		}

		if got, want := len(leaf.MerkleLeafHash), t.hashSizeBytes; got != want {
			return nil, fmt.Errorf("LogID: %d Scanned leaf %s does not have hash length %d, got %d", t.treeID, desc, want, got)
		}

		ret = append(ret, leaf)
	}
	if err := rows.Err(); err != nil {
		klog.Warningf("Failed to read returned leaves: %s", err)
		return nil, err
	}

	return ret, nil
}
