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

package mysql

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

	"github.com/go-sql-driver/mysql"
	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/cache"
	"github.com/google/trillian/trees"
	"github.com/mattn/go-sqlite3"

	spb "github.com/google/trillian/crypto/sigpb"
)

const (
	insertUnsequencedLeafSQL = `INSERT INTO LeafData(TreeId,LeafIdentityHash,LeafValue,ExtraData)
			VALUES(?,?,?,?)`
	selectSequencedLeafCountSQL   = "SELECT COUNT(*) FROM SequencedLeafData WHERE TreeId=?"
	selectUnsequencedLeafCountSQL = "SELECT TreeId, COUNT(1) FROM Unsequenced GROUP BY TreeId"
	selectLatestSignedLogRootSQL  = `SELECT TreeHeadTimestamp,TreeSize,RootHash,TreeRevision,RootSignature
			FROM TreeHead WHERE TreeId=?
			ORDER BY TreeHeadTimestamp DESC LIMIT 1`

	// These statements need to be expanded to provide the correct number of parameter placeholders.
	selectLeavesByIndexSQL = `SELECT s.MerkleLeafHash,l.LeafIdentityHash,l.LeafValue,s.SequenceNumber,l.ExtraData
			FROM LeafData l,SequencedLeafData s
			WHERE l.LeafIdentityHash = s.LeafIdentityHash
			AND s.SequenceNumber IN (` + placeholderSQL + `) AND l.TreeId = ? AND s.TreeId = l.TreeId`
	selectLeavesByMerkleHashSQL = `SELECT s.MerkleLeafHash,l.LeafIdentityHash,l.LeafValue,s.SequenceNumber,l.ExtraData
			FROM LeafData l,SequencedLeafData s
			WHERE l.LeafIdentityHash = s.LeafIdentityHash
			AND s.MerkleLeafHash IN (` + placeholderSQL + `) AND l.TreeId = ? AND s.TreeId = l.TreeId`
	// TODO(drysdale): rework the code so the dummy hash isn't needed (e.g. this assumes hash size is 32)
	dummyMerkleLeafHash = "00000000000000000000000000000000"
	// This statement returns a dummy Merkle leaf hash value (which must be
	// of the right size) so that its signature matches that of the other
	// leaf-selection statements.
	selectLeavesByLeafIdentityHashSQL = `SELECT '` + dummyMerkleLeafHash + `',l.LeafIdentityHash,l.LeafValue,-1,l.ExtraData
			FROM LeafData l
			WHERE l.LeafIdentityHash IN (` + placeholderSQL + `) AND l.TreeId = ?`

	// Same as above except with leaves ordered by sequence so we only incur this cost when necessary
	orderBySequenceNumberSQL                     = " ORDER BY s.SequenceNumber"
	selectLeavesByMerkleHashOrderedBySequenceSQL = selectLeavesByMerkleHashSQL + orderBySequenceNumberSQL

	// Error code returned by driver when inserting a duplicate row
	errNumDuplicate = 1062

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
	queuedCounter = mf.NewCounter("mysql_queued_leaves", "Number of leaves queued", logIDLabel)
	queuedDupCounter = mf.NewCounter("mysql_queued_dup_leaves", "Number of duplicate leaves queued", logIDLabel)
	dequeuedCounter = mf.NewCounter("mysql_dequeued_leaves", "Number of leaves dequeued", logIDLabel)

	queueLatency = mf.NewHistogram("mysql_queue_leaves_latency", "Latency of queue leaves operation in seconds", logIDLabel)
	queueInsertLatency = mf.NewHistogram("mysql_queue_leaves_latency_insert", "Latency of insertion part of queue leaves operation in seconds", logIDLabel)
	queueReadLatency = mf.NewHistogram("mysql_queue_leaves_latency_read_dups", "Latency of read-duplicates part of queue leaves operation in seconds", logIDLabel)
	queueInsertLeafLatency = mf.NewHistogram("mysql_queue_leaf_latency_leaf", "Latency of insert-leaf part of queue (single) leaf operation in seconds", logIDLabel)
	queueInsertEntryLatency = mf.NewHistogram("mysql_queue_leaf_latency_entry", "Latency of insert-entry part of queue (single) leaf operation in seconds", logIDLabel)

	dequeueLatency = mf.NewHistogram("mysql_dequeue_leaves_latency", "Latency of dequeue leaves operation in seconds", logIDLabel)
	dequeueSelectLatency = mf.NewHistogram("mysql_dequeue_leaves_latency_select", "Latency of selection part of dequeue leaves operation in seconds", logIDLabel)
	dequeueRemoveLatency = mf.NewHistogram("mysql_dequeue_leaves_latency_remove", "Latency of removal part of dequeue leaves operation in seconds", logIDLabel)
}

func labelForTX(t *logTreeTX) string {
	return strconv.FormatInt(t.treeID, 10)
}

func observe(hist monitoring.Histogram, duration time.Duration, label string) {
	hist.Observe(duration.Seconds(), label)
}

type mySQLLogStorage struct {
	*mySQLTreeStorage
	admin         storage.AdminStorage
	metricFactory monitoring.MetricFactory
}

// NewLogStorage creates a storage.LogStorage instance for the specified MySQL URL.
// It assumes storage.AdminStorage is backed by the same MySQL database as well.
func NewLogStorage(db *sql.DB, mf monitoring.MetricFactory) storage.LogStorage {
	if mf == nil {
		mf = monitoring.InertMetricFactory{}
	}
	return &mySQLLogStorage{
		admin:            NewAdminStorage(db),
		mySQLTreeStorage: newTreeStorage(db),
		metricFactory:    mf,
	}
}

func (m *mySQLLogStorage) CheckDatabaseAccessible(ctx context.Context) error {
	return checkDatabaseAccessible(ctx, m.db)
}

func (m *mySQLLogStorage) getLeavesByIndexStmt(ctx context.Context, num int) (*sql.Stmt, error) {
	return m.getStmt(ctx, selectLeavesByIndexSQL, num, "?", "?")
}

func (m *mySQLLogStorage) getLeavesByMerkleHashStmt(ctx context.Context, num int, orderBySequence bool) (*sql.Stmt, error) {
	if orderBySequence {
		return m.getStmt(ctx, selectLeavesByMerkleHashOrderedBySequenceSQL, num, "?", "?")
	}

	return m.getStmt(ctx, selectLeavesByMerkleHashSQL, num, "?", "?")
}

func (m *mySQLLogStorage) getLeavesByLeafIdentityHashStmt(ctx context.Context, num int) (*sql.Stmt, error) {
	return m.getStmt(ctx, selectLeavesByLeafIdentityHashSQL, num, "?", "?")
}

// readOnlyLogTX implements storage.ReadOnlyLogTX
type readOnlyLogTX struct {
	tx *sql.Tx
}

func (m *mySQLLogStorage) Snapshot(ctx context.Context) (storage.ReadOnlyLogTX, error) {
	tx, err := m.db.BeginTx(ctx, nil /* opts */)
	if err != nil {
		glog.Warningf("Could not start ReadOnlyLogTX: %s", err)
		return nil, err
	}
	return &readOnlyLogTX{tx}, nil
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
	rows, err := t.tx.QueryContext(
		ctx, selectNonDeletedTreeIDByTypeAndStateSQL, trillian.TreeType_LOG.String(), trillian.TreeState_ACTIVE.String())
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

func (m *mySQLLogStorage) beginInternal(ctx context.Context, treeID int64, readonly bool) (storage.LogTreeTX, error) {
	once.Do(func() {
		createMetrics(m.metricFactory)
	})
	tree, err := trees.GetTree(
		ctx,
		m.admin,
		treeID,
		trees.GetOpts{TreeType: trillian.TreeType_LOG, Readonly: readonly})
	if err != nil {
		return nil, err
	}
	hasher, err := hashers.NewLogHasher(tree.HashStrategy)
	if err != nil {
		return nil, err
	}

	stCache := cache.NewLogSubtreeCache(defaultLogStrata, hasher)
	ttx, err := m.beginTreeTx(ctx, treeID, hasher.Size(), stCache)
	if err != nil {
		return nil, err
	}

	ltx := &logTreeTX{
		treeTX: ttx,
		ls:     m,
	}

	ltx.root, err = ltx.fetchLatestRoot(ctx)
	if err != nil {
		ttx.Rollback()
		return nil, err
	}
	ltx.treeTX.writeRevision = ltx.root.TreeRevision + 1

	return ltx, nil
}

func (m *mySQLLogStorage) BeginForTree(ctx context.Context, treeID int64) (storage.LogTreeTX, error) {
	return m.beginInternal(ctx, treeID, false /* readonly */)
}

func (m *mySQLLogStorage) SnapshotForTree(ctx context.Context, treeID int64) (storage.ReadOnlyLogTreeTX, error) {
	tx, err := m.beginInternal(ctx, treeID, true /* readonly */)
	if err != nil {
		return nil, err
	}
	return tx.(storage.ReadOnlyLogTreeTX), err
}

type logTreeTX struct {
	treeTX
	ls   *mySQLLogStorage
	root trillian.SignedLogRoot
}

func (t *logTreeTX) ReadRevision() int64 {
	return t.root.TreeRevision
}

func (t *logTreeTX) WriteRevision() int64 {
	return t.treeTX.writeRevision
}

func (t *logTreeTX) DequeueLeaves(ctx context.Context, limit int, cutoffTime time.Time) ([]*trillian.LogLeaf, error) {
	start := time.Now()
	stx, err := t.tx.PrepareContext(ctx, selectQueuedLeavesSQL)

	if err != nil {
		glog.Warningf("Failed to prepare dequeue select: %s", err)
		return nil, err
	}

	leaves := make([]*trillian.LogLeaf, 0, limit)
	dq := make([]dequeuedLeaf, 0, limit)
	rows, err := stx.QueryContext(ctx, t.treeID, cutoffTime.UnixNano(), limit)

	if err != nil {
		glog.Warningf("Failed to select rows for work: %s", err)
		return nil, err
	}

	defer rows.Close()

	for rows.Next() {
		var leafIDHash []byte
		var merkleHash []byte
		var meta dequeueMeta

		err := rows.Scan(&leafIDHash, &merkleHash, &meta)
		if err != nil {
			glog.Warningf("Error scanning work rows: %s", err)
			return nil, err
		}

		// Note: the LeafData and ExtraData being nil here is OK as this is only used by the
		// sequencer. The sequencer only writes to the SequencedLeafData table and the client
		// supplied data was already written to LeafData as part of queueing the leaf.
		leaf := &trillian.LogLeaf{
			LeafIdentityHash: leafIDHash,
			MerkleLeafHash:   merkleHash,
		}

		if len(leaf.LeafIdentityHash) != t.hashSizeBytes {
			return nil, errors.New("dequeued a leaf with incorrect hash size")
		}

		leaves = append(leaves, leaf)
		dq = append(dq, dequeueInfo(leafIDHash, meta))
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

func (t *logTreeTX) QueueLeaves(ctx context.Context, leaves []*trillian.LogLeaf, queueTimestamp time.Time) ([]*trillian.LogLeaf, error) {
	// Don't accept batches if any of the leaves are invalid.
	for _, leaf := range leaves {
		if len(leaf.LeafIdentityHash) != t.hashSizeBytes {
			return nil, fmt.Errorf("queued leaf must have a leaf ID hash of length %d", t.hashSizeBytes)
		}
	}
	start := time.Now()
	label := labelForTX(t)

	// Insert in order of the hash values in the leaves, but track original position for return value.
	// This is to make the order that row locks are acquired deterministic and helps to reduce
	// the chance of deadlocks.
	orderedLeaves := make([]leafAndPosition, len(leaves))
	for i, leaf := range leaves {
		orderedLeaves[i] = leafAndPosition{leaf: leaf, idx: i}
	}
	sort.Sort(byLeafIdentityHashWithPosition(orderedLeaves))
	existingCount := 0
	existingLeaves := make([]*trillian.LogLeaf, len(leaves))

	for i, leafPos := range orderedLeaves {
		leafStart := time.Now()
		leaf := leafPos.leaf
		_, err := t.tx.ExecContext(ctx, insertUnsequencedLeafSQL, t.treeID, leaf.LeafIdentityHash, leaf.LeafValue, leaf.ExtraData)
		insertDuration := time.Since(leafStart)
		observe(queueInsertLeafLatency, insertDuration, label)
		if isDuplicateErr(err) {
			// Remember the duplicate leaf, using the requested leaf for now.
			existingLeaves[leafPos.idx] = leaf
			existingCount++
			queuedDupCounter.Inc(label)
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
		args = append(args, queueArgs(t.treeID, leaf.LeafIdentityHash, queueTimestamp)...)
		_, err = t.tx.ExecContext(
			ctx,
			insertUnsequencedEntrySQL,
			args...,
		)
		if err != nil {
			glog.Warningf("Error inserting into Unsequenced: %s", err)
			return nil, fmt.Errorf("Unsequenced: %v", err)
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
	totalDuration := time.Since(start)
	readDuration := totalDuration - insertDuration
	observe(queueReadLatency, readDuration, label)
	observe(queueLatency, totalDuration, label)

	return existingLeaves, nil
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
	tmpl, err := t.ls.getLeavesByIndexStmt(ctx, len(leaves))
	if err != nil {
		return nil, err
	}
	stx := t.tx.StmtContext(ctx, tmpl)
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

	ret := make([]*trillian.LogLeaf, 0, len(leaves))
	defer rows.Close()
	for rows.Next() {
		leaf := &trillian.LogLeaf{}
		if err := rows.Scan(
			&leaf.MerkleLeafHash,
			&leaf.LeafIdentityHash,
			&leaf.LeafValue,
			&leaf.LeafIndex,
			&leaf.ExtraData); err != nil {
			glog.Warningf("Failed to scan merkle leaves: %s", err)
			return nil, err
		}
		ret = append(ret, leaf)
	}

	if got, want := len(ret), len(leaves); got != want {
		return nil, fmt.Errorf("len(ret): %d, want %d", got, want)
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
// returned LogLeaf objects will not have a valid MerkleLeafHash or LeafIndex.
func (t *logTreeTX) getLeafDataByIdentityHash(ctx context.Context, leafHashes [][]byte) ([]*trillian.LogLeaf, error) {
	tmpl, err := t.ls.getLeavesByLeafIdentityHashStmt(ctx, len(leafHashes))
	if err != nil {
		return nil, err
	}
	return t.getLeavesByHashInternal(ctx, leafHashes, tmpl, "leaf-identity")
}

func (t *logTreeTX) LatestSignedLogRoot(ctx context.Context) (trillian.SignedLogRoot, error) {
	return t.root, nil
}

// fetchLatestRoot reads the latest SignedLogRoot from the DB and returns it.
func (t *logTreeTX) fetchLatestRoot(ctx context.Context) (trillian.SignedLogRoot, error) {
	var timestamp, treeSize, treeRevision int64
	var rootHash, rootSignatureBytes []byte
	var rootSignature spb.DigitallySigned

	err := t.tx.QueryRowContext(
		ctx, selectLatestSignedLogRootSQL, t.treeID).Scan(
		&timestamp, &treeSize, &rootHash, &treeRevision, &rootSignatureBytes)

	// It's possible there are no roots for this tree yet
	if err == sql.ErrNoRows {
		return trillian.SignedLogRoot{}, nil
	}

	err = proto.Unmarshal(rootSignatureBytes, &rootSignature)

	if err != nil {
		glog.Warningf("Failed to unmarshall root signature: %v", err)
		return trillian.SignedLogRoot{}, err
	}

	return trillian.SignedLogRoot{
		RootHash:       rootHash,
		TimestampNanos: timestamp,
		TreeRevision:   treeRevision,
		Signature:      &rootSignature,
		LogId:          t.treeID,
		TreeSize:       treeSize,
	}, nil
}

func (t *logTreeTX) StoreSignedLogRoot(ctx context.Context, root trillian.SignedLogRoot) error {
	signatureBytes, err := proto.Marshal(root.Signature)

	if err != nil {
		glog.Warningf("Failed to marshal root signature: %v %v", root.Signature, err)
		return err
	}

	res, err := t.tx.ExecContext(
		ctx,
		insertTreeHeadSQL,
		t.treeID,
		root.TimestampNanos,
		root.TreeSize,
		root.RootHash,
		root.TreeRevision,
		signatureBytes)
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

		if err := rows.Scan(&leaf.MerkleLeafHash, &leaf.LeafIdentityHash, &leaf.LeafValue, &leaf.LeafIndex, &leaf.ExtraData); err != nil {
			glog.Warningf("LogID: %d Scan() %s = %s", t.treeID, desc, err)
			return nil, err
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
	rows, err := stx.QueryContext(ctx)
	if err != nil {
		return nil, err
	}
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
	case *mysql.MySQLError:
		return err.Number == errNumDuplicate
	case sqlite3.Error:
		return err.Code == sqlite3.ErrConstraint && err.ExtendedCode == sqlite3.ErrConstraintPrimaryKey
	default:
		return false
	}
}
