// Copyright 2016 Google Inc. All Rights Reserved.
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

package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"
	"encoding/json"
	"github.com/lib/pq"
	"github.com/golang/glog"
	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/cache"
	"github.com/google/trillian/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
        valuesPlaceholder5 = "($1,$2,$3,$4,$5)"
        insertLeafDataSQL      ="select ignore_duplicates($1,$2,$3,$4,$5)"; 
        insertSequencedLeafSQL = "INSERT INTO sequenced_leaf_data(tree_id,leaf_identity_hash,merkle_leaf_hash,sequence_number,integrate_timestamp_nanos) VALUES"

        selectNonDeletedTreeIDByTypeAndStateSQL = `
                SELECT tree_id FROM trees WHERE tree_type in ($1,$2) AND tree_state in ($3,$4) AND (deleted IS NULL OR deleted = false)`

        selectSequencedLeafCountSQL   = "SELECT COUNT(*) FROM sequenced_leaf_data WHERE tree_id=?"
        selectUnsequencedLeafCountSQL = "SELECT tree_id, COUNT(1) FROM unsequenced GROUP BY tree_id"
        selectLatestSignedLogRootSQL  = `SELECT tree_head_timestamp,tree_size,root_hash,tree_revision,root_signature
                        FROM tree_head WHERE tree_id=$1
                        ORDER BY tree_head_timestamp DESC LIMIT 1`

        selectLeavesByRangeSQL = `SELECT s.merkle_leaf_hash,l.leaf_identity_hash,l.leaf_value,s.sequence_number,l.extra_data,l.queue_timestamp_nanos,s.integrate_timestamp_nanos
                        FROM leaf_data l,sequenced_leaf_data s
                        WHERE l.leaf_identity_hash = s.leaf_identity_hash
                        AND s.sequence_number >= $1 AND s.sequence_number < $2 AND l.tree_id = $3 AND s.tree_id = l.tree_id` + orderBysequence_numberSQL

        // These statements need to be expanded to provide the correct number of parameter placeholders.
        selectLeavesByIndexSQL = `SELECT s.merkle_leaf_hash,l.leaf_identity_hash,l.leaf_value,s.sequence_number,l.extra_data,l.queue_timestamp_nanos,s.integrate_timestamp_nanos
                        FROM leaf_data l,sequenced_leaf_data s
                        WHERE l.leaf_identity_hash = s.leaf_identity_hash
                        AND s.sequence_number IN (` + placeholderSQL + `) AND l.tree_id = <param> AND s.tree_id = l.tree_id`
        selectLeavesByMerkleHashSQL = `SELECT s.merkle_leaf_hash,l.leaf_identity_hash,l.leaf_value,s.sequence_number,l.extra_data,l.queue_timestamp_nanos,s.integrate_timestamp_nanos
                        FROM leaf_data l,sequenced_leaf_data s
                        WHERE l.leaf_identity_hash = s.leaf_identity_hash
                        AND s.merkle_leaf_hash IN (` + placeholderSQL + `) AND l.tree_id = <param> AND s.tree_id = l.tree_id`
        // TODO(drysdale): rework the code so the dummy hash isn't needed (e.g. this assumes hash size is 32)
        dummymerkle_leaf_hash = "00000000000000000000000000000000"
        // This statement returns a dummy Merkle leaf hash value (which must be
        // of the right size) so that its signature matches that of the other
        // leaf-selection statements.
        selectLeavesByLeafIdentityHashSQL = `SELECT '` + dummymerkle_leaf_hash + `',l.leaf_identity_hash,l.leaf_value,-1,l.extra_data,l.queue_timestamp_nanos,s.integrate_timestamp_nanos
                        FROM leaf_data l LEFT JOIN sequenced_leaf_data s ON (l.leaf_identity_hash = s.leaf_identity_hash AND l.tree_id = s.tree_id)
                        WHERE l.leaf_identity_hash IN (` + placeholderSQL + `) AND l.tree_id = <param>`

        // Same as above except with leaves ordered by sequence so we only incur this cost when necessary
        orderBysequence_numberSQL                     = " ORDER BY s.sequence_number"
        selectLeavesByMerkleHashOrderedBySequenceSQL = selectLeavesByMerkleHashSQL + orderBysequence_numberSQL

        // Error code returned by driver when inserting a duplicate row
        errNumDuplicate = "23505" //this is thrown because of the unique key constraint.  Effective check for duplicates

        logIDLabel = "logid"
)

var (
	defaultLogStrata = []int{8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8}

	once             sync.Once
	queuedCounter    monitoring.Counter
	queuedDupCounter monitoring.Counter
	dequeuedCounter  monitoring.Counter

	queueLatency            monitoring.Histogram
	queueInsertLatency      monitoring.Histogram
	queueReadLatency        monitoring.Histogram
	queueInsertLeafLatency  monitoring.Histogram
	queueInsertEntryLatency monitoring.Histogram
	dequeueLatency          monitoring.Histogram
	dequeueSelectLatency    monitoring.Histogram
	dequeueRemoveLatency    monitoring.Histogram
)

func createMetrics(mf monitoring.MetricFactory) {
	queuedCounter = mf.NewCounter("postgres_queued_leaves", "Number of leaves queued", logIDLabel)
	queuedDupCounter = mf.NewCounter("postgres_queued_dup_leaves", "Number of duplicate leaves queued", logIDLabel)
	dequeuedCounter = mf.NewCounter("postgres_dequeued_leaves", "Number of leaves dequeued", logIDLabel)

	queueLatency = mf.NewHistogram("postgres_queue_leaves_latency", "Latency of queue leaves operation in seconds", logIDLabel)
	queueInsertLatency = mf.NewHistogram("postgres_queue_leaves_latency_insert", "Latency of insertion part of queue leaves operation in seconds", logIDLabel)
	queueReadLatency = mf.NewHistogram("postgres_queue_leaves_latency_read_dups", "Latency of read-duplicates part of queue leaves operation in seconds", logIDLabel)
	queueInsertLeafLatency = mf.NewHistogram("postgres_queue_leaf_latency_leaf", "Latency of insert-leaf part of queue (single) leaf operation in seconds", logIDLabel)
	queueInsertEntryLatency = mf.NewHistogram("postgres_queue_leaf_latency_entry", "Latency of insert-entry part of queue (single) leaf operation in seconds", logIDLabel)

	dequeueLatency = mf.NewHistogram("postgres_dequeue_leaves_latency", "Latency of dequeue leaves operation in seconds", logIDLabel)
	dequeueSelectLatency = mf.NewHistogram("postgres_dequeue_leaves_latency_select", "Latency of selection part of dequeue leaves operation in seconds", logIDLabel)
	dequeueRemoveLatency = mf.NewHistogram("postgres_dequeue_leaves_latency_remove", "Latency of removal part of dequeue leaves operation in seconds", logIDLabel)
}

func labelForTX(t *logTreeTX) string {
	return strconv.FormatInt(t.treeID, 10)
}

func observe(hist monitoring.Histogram, duration time.Duration, label string) {
	hist.Observe(duration.Seconds(), label)
}

type postgresLogStorage struct {
	*pgTreeStorage
	admin         storage.AdminStorage
	metricFactory monitoring.MetricFactory
}

// NewLogStorage creates a storage.LogStorage instance for the specified MySQL URL.
// It assumes storage.AdminStorage is backed by the same MySQL database as well.
func NewLogStorage(db *sql.DB, mf monitoring.MetricFactory) storage.LogStorage {
	if mf == nil {
		mf = monitoring.InertMetricFactory{}
	}
	return &postgresLogStorage{
		admin:            NewAdminStorage(db),
		pgTreeStorage: newTreeStorage(db),
		metricFactory:    mf,
	}
}

func (m *postgresLogStorage) CheckDatabaseAccessible(ctx context.Context) error {
	return m.db.PingContext(ctx)
}

func (m *postgresLogStorage) getLeavesByIndexStmt(ctx context.Context, num int) (*sql.Stmt, error) {
	stmt := &statementSkeleton{ 
		sql:			selectLeavesByIndexSQL,
		firstInsertion:		"%s",
		firstPlaceholders: 	1,
		restInsertion:		"%s",
		restPlaceholders:	1,
		num:			num,
	}
	return m.getStmt(ctx, stmt)
}

func (m *postgresLogStorage) getLeavesByMerkleHashStmt(ctx context.Context, num int, orderBySequence bool) (*sql.Stmt, error) {
	if orderBySequence {

	        orderByStmt := &statementSkeleton{
        	        sql:                    selectLeavesByMerkleHashOrderedBySequenceSQL,
                	firstInsertion:         "%s",
	                firstPlaceholders:      1,
        	        restInsertion:          "%s",
                	restPlaceholders:       1,
	                num:                    num,
	        }

		return m.getStmt(ctx, orderByStmt)
	}
	
        merkleHashStmt := &statementSkeleton{
                sql:                  	selectLeavesByMerkleHashSQL,
                firstInsertion:         "%s",
                firstPlaceholders:      1,
                restInsertion:          "%s",
                restPlaceholders:       1,
                num:                    num,
        }

	return m.getStmt(ctx, merkleHashStmt)
}

func (m *postgresLogStorage) getLeavesByLeafIdentityHashStmt(ctx context.Context, num int) (*sql.Stmt, error) {
        identityHashStmt := &statementSkeleton{
                sql:                    selectLeavesByLeafIdentityHashSQL,
                firstInsertion:         "%s",
                firstPlaceholders:      1,
                restInsertion:          "%s",
                restPlaceholders:       1,
                num:                    num,
        }

	return m.getStmt(ctx, identityHashStmt)
}

// readOnlyLogTX implements storage.ReadOnlyLogTX
type readOnlyLogTX struct {
	ls *postgresLogStorage
	tx *sql.Tx
}

func (m *postgresLogStorage) Snapshot(ctx context.Context) (storage.ReadOnlyLogTX, error) {
	tx, err := m.db.BeginTx(ctx, nil /* opts */)
	if err != nil {
		glog.Warningf("Could not start ReadOnlyLogTX: %s", err)
		return nil, err
	}
	return &readOnlyLogTX{m, tx}, nil
}

func (t *readOnlyLogTX) Commit() error {
	return t.tx.Commit()
}

func (t *readOnlyLogTX) Rollback() error {
	return t.tx.Rollback()
}

func (t *readOnlyLogTX) Close() error {
	if err := t.Rollback(); err != nil && err != sql.ErrTxDone {
		glog.Warningf("Rollback error on Close(): %v", err)
		return err
	}
	return nil
}

func (t *readOnlyLogTX) GetActiveLogIDs(ctx context.Context) ([]int64, error) {
	// Include logs that are DRAINING in the active list as we're still
	// integrating leaves into them.
	rows, err := t.tx.QueryContext(
		ctx, selectNonDeletedTreeIDByTypeAndStateSQL,
		trillian.TreeType_LOG.String(), trillian.TreeType_PREORDERED_LOG.String(),
		trillian.TreeState_ACTIVE.String(), trillian.TreeState_DRAINING.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var treeID int64
		if err := rows.Scan(&treeID); err != nil {
			return nil, err
		}
		ids = append(ids, treeID)
	}
	return ids, rows.Err()
}

func (m *postgresLogStorage) beginInternal(ctx context.Context, tree *trillian.Tree) (storage.LogTreeTX, error) {
	once.Do(func() {
		createMetrics(m.metricFactory)
	})
	hasher, err := hashers.NewLogHasher(tree.HashStrategy)
	if err != nil {
		return nil, err
	}

	stCache := cache.NewLogSubtreeCache(defaultLogStrata, hasher)
	ttx, err := m.beginTreeTx(ctx, tree, hasher.Size(), stCache)
	if err != nil && err != storage.ErrTreeNeedsInit {
		return nil, err
	}

	ltx := &logTreeTX{
		treeTX: ttx,
		ls:     m,
	}
	ltx.slr, err = ltx.fetchLatestRoot(ctx)
	if err == storage.ErrTreeNeedsInit {
		return ltx, err
	} else if err != nil {
		ttx.Rollback()
		return nil, err
	}
	if err := ltx.root.UnmarshalBinary(ltx.slr.LogRoot); err != nil {
		ttx.Rollback()
		return nil, err
	}

	ltx.treeTX.writeRevision = int64(ltx.root.Revision) + 1
	return ltx, nil
}

func (m *postgresLogStorage) ReadWriteTransaction(ctx context.Context, tree *trillian.Tree, f storage.LogTXFunc) error {
	tx, err := m.beginInternal(ctx, tree)
	if err != nil && err != storage.ErrTreeNeedsInit {
		return err
	}
	defer tx.Close()
	if err := f(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (m *postgresLogStorage) AddSequencedLeaves(ctx context.Context, tree *trillian.Tree, leaves []*trillian.LogLeaf, timestamp time.Time) ([]*trillian.QueuedLogLeaf, error) {
	tx, err := m.beginInternal(ctx, tree)
	if err != nil {
		return nil, err
	}
	res, err := tx.AddSequencedLeaves(ctx, leaves, timestamp)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return res, nil
}

func (m *postgresLogStorage) SnapshotForTree(ctx context.Context, tree *trillian.Tree) (storage.ReadOnlyLogTreeTX, error) {
	tx, err := m.beginInternal(ctx, tree)
	if err != nil && err != storage.ErrTreeNeedsInit {
		return nil, err
	}
	return tx, err
}

func (m *postgresLogStorage) QueueLeaves(ctx context.Context, tree *trillian.Tree, leaves []*trillian.LogLeaf, queueTimestamp time.Time) ([]*trillian.QueuedLogLeaf, error) {
	tx, err := m.beginInternal(ctx, tree)
	if err != nil {
		return nil, err
	}
	existing, err := tx.QueueLeaves(ctx, leaves, queueTimestamp)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
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
	ls   *postgresLogStorage
	root types.LogRootV1
	slr  trillian.SignedLogRoot
}

func (t *logTreeTX) ReadRevision(ctx context.Context) (int64, error) {
	return int64(t.root.Revision), nil
}

func (t *logTreeTX) WriteRevision(ctx context.Context) (int64, error) {
	if t.treeTX.writeRevision < 0 {
		return t.treeTX.writeRevision, errors.New("logTreeTX write revision not populated")
	}
	return t.treeTX.writeRevision, nil
}

func (t *logTreeTX) DequeueLeaves(ctx context.Context, limit int, cutoffTime time.Time) ([]*trillian.LogLeaf, error) {
	if t.treeType == trillian.TreeType_PREORDERED_LOG {
		// TODO(pavelkalinnikov): Optimize this by fetching only the required
		// fields of LogLeaf. We can avoid joining with LeafData table here.
		return t.GetLeavesByRange(ctx, int64(t.root.TreeSize), int64(limit))
	}

	start := time.Now()
	stx, err := t.tx.PrepareContext(ctx, selectQueuedLeavesSQL)
	if err != nil {
		glog.Warningf("Failed to prepare dequeue select: %s", err)
		return nil, err
	}
	defer stx.Close()

	leaves := make([]*trillian.LogLeaf, 0, limit)
	dq := make([]dequeuedLeaf, 0, limit)
	rows, err := stx.QueryContext(ctx, t.treeID, cutoffTime.UnixNano(), limit)
	if err != nil {
		glog.Warningf("Failed to select rows for work: %s", err)
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		leaf, dqInfo, err := t.dequeueLeaf(rows)
		if err != nil {
			glog.Warningf("Error dequeuing leaf: %v %v", err, selectQueuedLeavesSQL)
			return nil, err
		}

		if len(leaf.LeafIdentityHash) != t.hashSizeBytes {
			return nil, errors.New("dequeued a leaf with incorrect hash size")
		}

		leaves = append(leaves, leaf)
		dq = append(dq, dqInfo)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}
	label := labelForTX(t)
	selectDuration := time.Since(start)
	observe(dequeueSelectLatency, selectDuration, label)

	// The convention is that if leaf processing succeeds (by committing this tx)
	// then the unsequenced entries for them are removed
	if len(leaves) > 0 {
		err = t.removeSequencedLeaves(ctx, dq)
	}

	if err != nil {
		return nil, err
	}

	totalDuration := time.Since(start)
	removeDuration := totalDuration - selectDuration
	observe(dequeueRemoveLatency, removeDuration, label)
	observe(dequeueLatency, totalDuration, label)
	dequeuedCounter.Add(float64(len(leaves)), label)

	return leaves, nil
}

// sortLeavesForInsert returns a slice containing the passed in leaves sorted
// by LeafIdentityHash, and paired with their original positions.
// QueueLeaves and AddSequencedLeaves use this to make the order that LeafData
// row locks are acquired deterministic and reduce the chance of deadlocks.
func sortLeavesForInsert(leaves []*trillian.LogLeaf) []leafAndPosition {
	ordLeaves := make([]leafAndPosition, len(leaves))
	for i, leaf := range leaves {
		ordLeaves[i] = leafAndPosition{leaf: leaf, idx: i}
	}
	sort.Sort(byLeafIdentityHashWithPosition(ordLeaves))
	return ordLeaves
}

func (t *logTreeTX) QueueLeaves(ctx context.Context, leaves []*trillian.LogLeaf, queueTimestamp time.Time) ([]*trillian.LogLeaf, error) {
	// Don't accept batches if any of the leaves are invalid.
	for _, leaf := range leaves {
		if len(leaf.LeafIdentityHash) != t.hashSizeBytes {
			return nil, fmt.Errorf("queued leaf must have a leaf ID hash of length %d", t.hashSizeBytes)
		}
		var err error
		leaf.QueueTimestamp, err = ptypes.TimestampProto(queueTimestamp)
		if err != nil {
			return nil, fmt.Errorf("got invalid queue timestamp: %v", err)
		}
	}
	start := time.Now()
	label := labelForTX(t)

	ordLeaves := sortLeavesForInsert(leaves)
	existingCount := 0
	existingLeaves := make([]*trillian.LogLeaf, len(leaves))

	for _, ol := range ordLeaves {
		i, leaf := ol.idx, ol.leaf

		leafStart := time.Now()
		qTimestamp, err := ptypes.Timestamp(leaf.QueueTimestamp)
		if err != nil {
			return nil, fmt.Errorf("got invalid queue timestamp: %v", err)
		}
		dupCheckRow, err := t.tx.QueryContext(ctx, insertLeafDataSQL, t.treeID, leaf.LeafIdentityHash, leaf.LeafValue, leaf.ExtraData, qTimestamp.UnixNano())
		insertDuration := time.Since(leafStart)
		observe(queueInsertLeafLatency, insertDuration, label)
	        var resultData bool
		resultData = true
		dupCheckRow.Scan(&resultData)
		dupCheckRow.Close()
		if resultData == false {
			// Remember the duplicate leaf, using the requested leaf for now.
			existingLeaves[i] = leaf
			existingCount++
			queuedDupCounter.Inc(label)
			glog.Warningf("Found duplicate %v %v", t.treeID, leaf)
			continue
		}
		if err != nil {
			glog.Warningf("Error inserting %d into LeafData: %s", i, err)
			return nil, err
		}

		// Create the work queue entry
		args := []interface{}{
			t.treeID,
			leaf.LeafIdentityHash,
			leaf.MerkleLeafHash,
		}
		queueTimestamp, err := ptypes.Timestamp(leaf.QueueTimestamp)
		if err != nil {
			return nil, fmt.Errorf("got invalid queue timestamp: %v", err)
		}
		args = append(args, queueArgs(t.treeID, leaf.LeafIdentityHash, queueTimestamp)...)
		_, err = t.tx.ExecContext(
			ctx,
			insertUnsequencedEntrySQL,
			args...,
		)
		if err != nil {
			glog.Warningf("Error inserting into Unsequenced: %s query %v arguements: %v", err,insertUnsequencedEntrySQL, args)
			return nil, fmt.Errorf("Unsequenced: %v -- %v", err,args)
		}
		leafDuration := time.Since(leafStart)
		observe(queueInsertEntryLatency, (leafDuration - insertDuration), label)
	}
	insertDuration := time.Since(start)
	observe(queueInsertLatency, insertDuration, label)
	queuedCounter.Add(float64(len(leaves)), label)

	if existingCount == 0 {
		return existingLeaves, nil
	}

	// For existing leaves, we need to retrieve the contents.  First collate the desired LeafIdentityHash values.
	var toRetrieve [][]byte
	for _, existing := range existingLeaves {
		if existing != nil {
			toRetrieve = append(toRetrieve, existing.LeafIdentityHash)
		}
	}
	results, err := t.getLeafDataByIdentityHash(ctx, toRetrieve)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve existing leaves: %v %v", err,toRetrieve)
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
	totalDuration := time.Since(start)
	readDuration := totalDuration - insertDuration
	observe(queueReadLatency, readDuration, label)
	observe(queueLatency, totalDuration, label)

	return existingLeaves, nil
}

func (t *logTreeTX) AddSequencedLeaves(ctx context.Context, leaves []*trillian.LogLeaf, timestamp time.Time) ([]*trillian.QueuedLogLeaf, error) {
	res := make([]*trillian.QueuedLogLeaf, len(leaves))
	ok := status.New(codes.OK, "OK").Proto()

	// Leaves in this transaction are inserted in two tables. For each leaf, if
	// one of the two inserts fails, we remove the side effect by rolling back to
	// a savepoint installed before the first insert of the two.
	const savepoint = "SAVEPOINT AddSequencedLeaves"
	if _, err := t.tx.ExecContext(ctx, savepoint); err != nil {
		glog.Errorf("Error adding savepoint: %s", err)
		return nil, err
	}
	// TODO(pavelkalinnikov): Consider performance implication of executing this
	// extra SAVEPOINT, especially for 1-entry batches. Optimize if necessary.

	// Note: LeafData inserts are presumably protected from deadlocks due to
	// sorting, but the order of the corresponding SequencedLeafData inserts
	// becomes indeterministic. However, in a typical case when leaves are
	// supplied in contiguous non-intersecting batches, the chance of having
	// circular dependencies between transactions is significantly lower.
	ordLeaves := sortLeavesForInsert(leaves)
	for _, ol := range ordLeaves {
		i, leaf := ol.idx, ol.leaf

		// This should fail on insert, but catch it early.
		if got, want := len(leaf.LeafIdentityHash), t.hashSizeBytes; got != want {
			return nil, status.Errorf(codes.FailedPrecondition, "leaves[%d] has incorrect hash size %d, want %d", i, got, want)
		}

		if _, err := t.tx.ExecContext(ctx, savepoint); err != nil {
			glog.Errorf("Error updating savepoint: %s", err)
			return nil, err
		}

		res[i] = &trillian.QueuedLogLeaf{Status: ok}

		// TODO(pavelkalinnikov): Measure latencies.
		_, err := t.tx.ExecContext(ctx, insertLeafDataSQL,
			t.treeID, leaf.LeafIdentityHash, leaf.LeafValue, leaf.ExtraData, timestamp.UnixNano())
		// TODO(pavelkalinnikov): Detach PREORDERED_LOG integration latency metric.

		if err != nil {
			glog.Errorf("Error inserting leaves[%d] into LeafData: %s", i, err)
			return nil, err
		}

		_, err = t.tx.ExecContext(ctx, insertSequencedLeafSQL+valuesPlaceholder5,
			t.treeID, leaf.LeafIdentityHash, leaf.MerkleLeafHash, leaf.LeafIndex, 0)
		// TODO(pavelkalinnikov): Update IntegrateTimestamp on integrating the leaf.

		if isDuplicateErr(err) {
			res[i].Status = status.New(codes.FailedPrecondition, "conflicting LeafIndex").Proto()
			if _, err := t.tx.ExecContext(ctx, "ROLLBACK TO "+savepoint); err != nil {
				glog.Errorf("Error rolling back to savepoint: %s", err)
				return nil, err
			}
		} else if err != nil {
			glog.Errorf("Error inserting leaves[%d] into SequencedLeafData: %s", i, err)
			return nil, err
		}

		// TODO(pavelkalinnikov): Load LeafData for conflicting entries.
	}

	if _, err := t.tx.ExecContext(ctx, "RELEASE "+savepoint); err != nil {
		glog.Errorf("Error releasing savepoint: %s", err)
		return nil, err
	}

	return res, nil
}

func (t *logTreeTX) GetSequencedLeafCount(ctx context.Context) (int64, error) {
	var sequencedLeafCount int64

	err := t.tx.QueryRowContext(ctx, selectSequencedLeafCountSQL, t.treeID).Scan(&sequencedLeafCount)
	if err != nil {
		glog.Warningf("Error getting sequenced leaf count: %s", err)
	}

	return sequencedLeafCount, err
}

func (t *logTreeTX) GetLeavesByIndex(ctx context.Context, leaves []int64) ([]*trillian.LogLeaf, error) {
	if t.treeType == trillian.TreeType_LOG {
		treeSize := int64(t.root.TreeSize)
		for _, leaf := range leaves {
			if leaf < 0 {
				return nil, status.Errorf(codes.InvalidArgument, "index %d is < 0", leaf)
			}
			if leaf >= treeSize {
				return nil, status.Errorf(codes.OutOfRange, "invalid leaf index %d, want < TreeSize(%d)", leaf, treeSize)
			}
		}
	}
	tmpl, err := t.ls.getLeavesByIndexStmt(ctx, len(leaves))
	if err != nil {
		return nil, err
	}
	stx := t.tx.StmtContext(ctx, tmpl)
	defer stx.Close()

	var args []interface{}
	for _, nodeID := range leaves {
		args = append(args, interface{}(int64(nodeID)))
	}
	args = append(args, interface{}(t.treeID))
	rows, err := stx.QueryContext(ctx, args...)
	if err != nil {
		glog.Warningf("Failed to get leaves by idx: %s", err)
		return nil, err
	}
	defer rows.Close()

	ret := make([]*trillian.LogLeaf, 0, len(leaves))
	for rows.Next() {
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
			glog.Warningf("Failed to scan merkle leaves: %s", err)
			return nil, err
		}
		var err error
		leaf.QueueTimestamp, err = ptypes.TimestampProto(time.Unix(0, qTimestamp))
		if err != nil {
			return nil, fmt.Errorf("got invalid queue timestamp: %v", err)
		}
		leaf.IntegrateTimestamp, err = ptypes.TimestampProto(time.Unix(0, iTimestamp))
		if err != nil {
			return nil, fmt.Errorf("got invalid integrate timestamp: %v", err)
		}
		ret = append(ret, leaf)
	}

	if got, want := len(ret), len(leaves); got != want {
		return nil, status.Errorf(codes.Internal, "len(ret): %d, want %d", got, want)
	}
	return ret, nil
}

func (t *logTreeTX) GetLeavesByRange(ctx context.Context, start, count int64) ([]*trillian.LogLeaf, error) {
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
	// TODO(pavelkalinnikov): Further clip `count` to a safe upper bound like 64k.

	args := []interface{}{start, start + count, t.treeID}
	rows, err := t.tx.QueryContext(ctx, selectLeavesByRangeSQL, args...)
	if err != nil {
		glog.Warningf("Failed to get leaves by range: %s", err)
		return nil, err
	}
	defer rows.Close()

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
			glog.Warningf("Failed to scan merkle leaves: %s", err)
			return nil, err
		}
		if leaf.LeafIndex != wantIndex {
			if wantIndex < int64(t.root.TreeSize) {
				return nil, fmt.Errorf("got unexpected index %d, want %d", leaf.LeafIndex, wantIndex)
			}
			break
		}
		var err error
		leaf.QueueTimestamp, err = ptypes.TimestampProto(time.Unix(0, qTimestamp))
		if err != nil {
			return nil, fmt.Errorf("got invalid queue timestamp: %v", err)
		}
		leaf.IntegrateTimestamp, err = ptypes.TimestampProto(time.Unix(0, iTimestamp))
		if err != nil {
			return nil, fmt.Errorf("got invalid integrate timestamp: %v", err)
		}
		ret = append(ret, leaf)
	}

	return ret, nil
}

func (t *logTreeTX) GetLeavesByHash(ctx context.Context, leafHashes [][]byte, orderBySequence bool) ([]*trillian.LogLeaf, error) {
	tmpl, err := t.ls.getLeavesByMerkleHashStmt(ctx, len(leafHashes), orderBySequence)
	if err != nil {
		return nil, err
	}

	return t.getLeavesByHashInternal(ctx, leafHashes, tmpl, "merkle")
}

// getLeafDataByIdentityHash retrieves leaf data by LeafIdentityHash, returned
// as a slice of LogLeaf objects for convenience.  However, note that the
// returned LogLeaf objects will not have a valid MerkleLeafHash, LeafIndex, or IntegrateTimestamp.
func (t *logTreeTX) getLeafDataByIdentityHash(ctx context.Context, leafHashes [][]byte) ([]*trillian.LogLeaf, error) {
	tmpl, err := t.ls.getLeavesByLeafIdentityHashStmt(ctx, len(leafHashes))
	if err != nil {
		return nil, err
	}
	return t.getLeavesByHashInternal(ctx, leafHashes, tmpl, "leaf-identity")
}

func (t *logTreeTX) LatestSignedLogRoot(ctx context.Context) (trillian.SignedLogRoot, error) {
	return t.slr, nil
}

// fetchLatestRoot reads the latest SignedLogRoot from the DB and returns it.
func (t *logTreeTX) fetchLatestRoot(ctx context.Context) (trillian.SignedLogRoot, error) {
	var rootSignatureBytes []byte
	var jsonObj []byte

        t.tx.QueryRowContext(
                ctx,
                "select current_tree_data,root_signature from trees where tree_id = $1",
                t.treeID).Scan(&jsonObj,&rootSignatureBytes)
	if jsonObj == nil { //this fixes the createtree workflow
		return trillian.SignedLogRoot{}, storage.ErrTreeNeedsInit
	}
	var logRoot types.LogRootV1
	json.Unmarshal(jsonObj,&logRoot)
	newRoot, _ := logRoot.MarshalBinary()
	return trillian.SignedLogRoot{
		KeyHint:          types.SerializeKeyHint(t.treeID),
		LogRoot:          newRoot,
		LogRootSignature: rootSignatureBytes,
	}, nil
}

func (t *logTreeTX) StoreSignedLogRoot(ctx context.Context, root trillian.SignedLogRoot) error {
	var logRoot types.LogRootV1
	if err := logRoot.UnmarshalBinary(root.LogRoot); err != nil {
		glog.Warningf("Failed to parse log root: %x %v", root.LogRoot, err)
		return err
	}
	if len(logRoot.Metadata) != 0 {
		return fmt.Errorf("unimplemented: postgres storage does not support log root metadata")

	}
	//get a json copy of the tree_head
        data,_ := json.Marshal(logRoot)
	t.tx.ExecContext(
		ctx,
		"update trees set current_tree_data = $1,root_signature = $2 where tree_id = $3",
		data,
		root.LogRootSignature,
		t.treeID)
	res, err := t.tx.ExecContext(
		ctx,
		insertTreeHeadSQL,
		t.treeID,
		logRoot.TimestampNanos,
		logRoot.TreeSize,
		logRoot.RootHash,
		logRoot.Revision,
		root.LogRootSignature)
	if err != nil {
		glog.Warningf("Failed to store signed root: %s", err)
	}

	return checkResultOkAndRowCountIs(res, err, 1)
}

func (t *logTreeTX) getLeavesByHashInternal(ctx context.Context, leafHashes [][]byte, tmpl *sql.Stmt, desc string) ([]*trillian.LogLeaf, error) {
	stx := t.tx.StmtContext(ctx, tmpl)
	defer stx.Close()

	var args []interface{}
	for _, hash := range leafHashes {
		args = append(args, interface{}([]byte(hash)))
	}
	args = append(args, interface{}(t.treeID))
	rows, err := stx.QueryContext(ctx, args...)
	if err != nil {
		glog.Warningf("Query() %s hash = %v", desc, err)
		return nil, err
	}
	defer rows.Close()

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
			glog.Warningf("LogID: %d Scan() %s = %s", t.treeID, desc, err)
			return nil, err
		}
		var err error
		leaf.QueueTimestamp, err = ptypes.TimestampProto(time.Unix(0, queueTS))
		if err != nil {
			return nil, fmt.Errorf("got invalid queue timestamp: %v", err)
		}
		if integrateTS.Valid {
			leaf.IntegrateTimestamp, err = ptypes.TimestampProto(time.Unix(0, integrateTS.Int64))
			if err != nil {
				return nil, fmt.Errorf("got invalid integrate timestamp: %v", err)
			}
		}

		if got, want := len(leaf.MerkleLeafHash), t.hashSizeBytes; got != want {
			return nil, fmt.Errorf("LogID: %d Scanned leaf %s does not have hash length %d, got %d", t.treeID, desc, want, got)
		}

		ret = append(ret, leaf)
	}

	return ret, nil
}

func (t *readOnlyLogTX) GetUnsequencedCounts(ctx context.Context) (storage.CountByLogID, error) {
	stx, err := t.tx.PrepareContext(ctx, selectUnsequencedLeafCountSQL)
	if err != nil {
		glog.Warningf("Failed to prep unsequenced leaf count statement: %v", err)
		return nil, err
	}
	defer stx.Close()

	rows, err := stx.QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ret := make(map[int64]int64)
	for rows.Next() {
		var logID, count int64
		if err := rows.Scan(&logID, &count); err != nil {
			return nil, fmt.Errorf("failed to scan row from unsequenced counts: %v", err)
		}
		ret[logID] = count
	}
	return ret, nil
}

// leafAndPosition records original position before sort.
type leafAndPosition struct {
	leaf *trillian.LogLeaf
	idx  int
}

// byLeafIdentityHashWithPosition allows sorting (as above), but where we need
// to remember the original position
type byLeafIdentityHashWithPosition []leafAndPosition

func (l byLeafIdentityHashWithPosition) Len() int {
	return len(l)
}
func (l byLeafIdentityHashWithPosition) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}
func (l byLeafIdentityHashWithPosition) Less(i, j int) bool {
	return bytes.Compare(l[i].leaf.LeafIdentityHash, l[j].leaf.LeafIdentityHash) == -1
}

func isDuplicateErr(err error) bool {
	switch err := err.(type) {
	case *pq.Error:
		glog.Infof("checking %v %v %v",err.Code,errNumDuplicate, err.Code==errNumDuplicate)
		return err.Code == errNumDuplicate
	default:
		return false
	}
}
// Copyright 2016 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
