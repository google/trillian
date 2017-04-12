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

package coresql

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	spb "github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/monitoring/metric"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/cache"
	"github.com/google/trillian/trees"
	"github.com/google/trillian/storage/sql/coresql/wrapper"
)

var (
	defaultLogStrata = []int{8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8}

	queuedCounter   = metric.NewCounter("sql_queued_leaves")
	dequeuedCounter = metric.NewCounter("sql_dequeued_leaves")
)

type mySQLLogStorage struct {
	*mySQLTreeStorage
	admin storage.AdminStorage
}

// NewLogStorage creates a mySQLLogStorage instance for the specified MySQL URL.
// It assumes storage.AdminStorage is backed by the same MySQL database as well.
func NewLogStorage(wrapper wrapper.DBWrapper) storage.LogStorage {
	return &mySQLLogStorage{
		admin:            NewAdminStorage(wrapper),
		mySQLTreeStorage: newTreeStorage(wrapper),
	}
}

func (m *mySQLLogStorage) CheckDatabaseAccessible(ctx context.Context) error {
	return m.wrap.CheckDatabaseAccessible(ctx)
}

func getActiveLogIDsInternal(ctx context.Context, stmt *sql.Stmt) ([]int64, error) {
	rows, err := stmt.QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logIDs := make([]int64, 0)
	for rows.Next() {
		var treeID int64
		if err := rows.Scan(&treeID); err != nil {
			return nil, err
		}
		logIDs = append(logIDs, treeID)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return logIDs, nil
}

func getActiveLogIDs(ctx context.Context, t *readOnlyLogTX) ([]int64, error) {
	return getActiveLogIDsInternal(ctx, t.wrap.GetActiveLogsStmt())
}

func getActiveLogIDsWithPendingWork(ctx context.Context, t *readOnlyLogTX) ([]int64, error) {
	return getActiveLogIDsInternal(ctx, t.wrap.GetActiveLogsWithWorkStmt())
}

// readOnlyLogTX implements storage.ReadOnlyLogTX
type readOnlyLogTX struct {
	tx   *sql.Tx
	wrap wrapper.DBWrapper
}

func (m *mySQLLogStorage) Snapshot(ctx context.Context) (storage.ReadOnlyLogTX, error) {
	tx, err := m.wrap.DB().Begin()
	if err != nil {
		glog.Warningf("Could not start ReadOnlyLogTX: %s", err)
		return nil, err
	}
	return &readOnlyLogTX{tx: tx, wrap: m.wrap}, nil
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
	return getActiveLogIDs(ctx, t)
}

func (t *readOnlyLogTX) GetActiveLogIDsWithPendingWork(ctx context.Context) ([]int64, error) {
	return getActiveLogIDsWithPendingWork(ctx, t.wrap.GetActiveLogsWithWorkStmt())
}

func (m *mySQLLogStorage) beginInternal(ctx context.Context, treeID int64, readonly bool) (storage.LogTreeTX, error) {
	tree, err := trees.GetTree(
		ctx,
		m.admin,
		treeID,
		trees.GetOpts{TreeType: trillian.TreeType_LOG, Readonly: readonly})
	if err != nil {
		return nil, err
	}
	hasher, err := trees.Hasher(tree)
	if err != nil {
		return nil, err
	}

	ttx, err := m.beginTreeTx(ctx, treeID, hasher.Size(), defaultLogStrata, cache.PopulateLogSubtreeNodes(hasher), cache.PrepareLogSubtreeWrite())
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
	stx, err := t.ls.wrap.GetQueuedLeavesStmt(t.tx)
	if err != nil {
		glog.Warningf("Failed to prepare dequeue select: %s", err)
		return nil, err
	}
	defer stx.Close()

	leaves := make([]*trillian.LogLeaf, 0, limit)
	rows, err := stx.QueryContext(ctx, t.treeID, cutoffTime.UnixNano(), limit)

	if err != nil {
		glog.Warningf("Failed to select rows for work: %s", err)
		return nil, err
	}

	defer rows.Close()

	for rows.Next() {
		var leafIDHash []byte
		var merkleHash []byte

		err := rows.Scan(&leafIDHash, &merkleHash)

		if err != nil {
			glog.Warningf("Error scanning work rows: %s", err)
			return nil, err
		}

		if len(leafIDHash) != t.hashSizeBytes {
			return nil, errors.New("Dequeued a leaf with incorrect hash size")
		}

		// Note: the LeafData and ExtraData being nil here is OK as this is only used by the
		// sequencer. The sequencer only writes to the SequencedLeafData table and the client
		// supplied data was already written to LeafData as part of queueing the leaf.
		leaf := &trillian.LogLeaf{
			LeafIdentityHash: leafIDHash,
			MerkleLeafHash:   merkleHash,
		}
		leaves = append(leaves, leaf)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	// The convention is that if leaf processing succeeds (by committing this tx)
	// then the unsequenced entries for them are removed
	if len(leaves) > 0 {
		err = t.removeSequencedLeaves(ctx, leaves)
	}

	if err != nil {
		return nil, err
	}

	dequeuedCounter.Add(int64(len(leaves)))

	return leaves, nil
}

func (t *logTreeTX) QueueLeaves(ctx context.Context, leaves []*trillian.LogLeaf, queueTimestamp time.Time) ([]*trillian.LogLeaf, error) {
	// Don't accept batches if any of the leaves are invalid.
	for _, leaf := range leaves {
		if len(leaf.LeafIdentityHash) != t.hashSizeBytes {
			return nil, fmt.Errorf("queued leaf must have a leaf ID hash of length %d", t.hashSizeBytes)
		}
	}

	// Insert in order of the hash values in the leaves, but track original position for return value.
	orderedLeaves := make([]leafAndPosition, len(leaves))
	for i, leaf := range leaves {
		orderedLeaves[i] = leafAndPosition{leaf: leaf, idx: i}
	}
	sort.Sort(byLeafIdentityHashWithPosition(orderedLeaves))
	existingCount := 0
	existingLeaves := make([]*trillian.LogLeaf, len(leaves))

	unseqEntryStmt, err := t.ls.wrap.InsertUnsequencedEntryStmt(t.tx)
	if err != nil {
		return nil, err
	}
	defer unseqEntryStmt.Close()
	unseqLeafStmt, err := t.ls.wrap.InsertUnsequencedLeafStmt(t.tx)
	if err != nil {
		return nil, err
	}
	defer unseqLeafStmt.Close()

	for i, leafPos := range orderedLeaves {
		leaf := leafPos.leaf
		_, err := unseqLeafStmt.Exec(t.treeID, leaf.LeafIdentityHash, leaf.LeafValue, leaf.ExtraData)
		if t.ls.wrap.IsDuplicateErr(err) {
			// Remember the duplicate leaf, using the requested leaf for now.
			existingLeaves[leafPos.idx] = leaf
			existingCount++
			continue
		}
		if err != nil {
			glog.Warningf("Error inserting %d into LeafData: %s", i, err)
			return nil, err
		}

		// Create the work queue entry
		// Message ids only need to guard against duplicates for the time that entries are
		// in the unsequenced queue, which should be short, but we'll still use a strong hash.
		// TODO(alcutter): get this from somewhere else
		hasher := sha256.New()
		binary.Write(hasher, binary.LittleEndian, t.treeID)
		hasher.Write(leaf.LeafIdentityHash)
		messageID := hasher.Sum(nil)

		_, err = unseqEntryStmt.ExecContext(
			ctx,
			t.treeID,
			leaf.LeafIdentityHash,
			leaf.MerkleLeafHash,
			messageID,
			queueTimestamp.UnixNano())
		if err != nil {
			glog.Warningf("Error inserting into Unsequenced: %s", err)
			return nil, fmt.Errorf("Unsequenced: %v", err)
		}
	}

	if existingCount == 0 {
		queuedCounter.Add(int64(len(leaves)))
		return existingLeaves, nil
	}

	// For existing leaves, we need to retrieve the contents.  First collate the desired LeafIdentityHash values.
	var toRetrieve [][]byte
	for _, existing := range existingLeaves {
		if existing != nil {
			toRetrieve = append(toRetrieve, existing.LeafIdentityHash)
		}
	}
	results, err := t.GetLeafDataByIdentityHash(ctx, toRetrieve)
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
			if bytes.Compare(result.LeafIdentityHash, requested.LeafIdentityHash) == 0 {
				existingLeaves[i] = result
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("failed to find existing leaf for hash %x", requested.LeafIdentityHash)
		}
	}

	queuedCounter.Add(int64(len(leaves)))
	return existingLeaves, nil
}

func (t *logTreeTX) GetSequencedLeafCount(ctx context.Context) (int64, error) {
	var sequencedLeafCount int64

	stmt, err := t.ls.wrap.GetSequencedLeafCountStmt(t.tx)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	err = stmt.QueryRowContext(ctx, t.treeID).Scan(&sequencedLeafCount)

	if err != nil {
		glog.Warningf("Error getting sequenced leaf count: %s", err)
	}

	return sequencedLeafCount, err
}

func (t *logTreeTX) GetLeavesByIndex(ctx context.Context, leaves []int64) ([]*trillian.LogLeaf, error) {
	stmt, err := t.ls.wrap.GetLeavesByIndexStmt(t.tx, len(leaves))
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	var args []interface{}
	for _, nodeID := range leaves {
		args = append(args, interface{}(int64(nodeID)))
	}
	args = append(args, interface{}(t.treeID))
	rows, err := stmt.QueryContext(ctx, args...)
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
	stmt, err := t.ls.wrap.GetLeavesByMerkleHashStmt(t.tx, len(leafHashes), orderBySequence)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	return t.getLeavesByHashInternal(ctx, leafHashes, stmt, "merkle")
}

// GetLeafDataByIdentityHash retrieves leaf data by LeafIdentityHash, returned
// as a slice of LogLeaf objects for convenience.  However, note that the
// returned LogLeaf objects will not have a valid MerkleLeafHash or LeafIndex.
func (t *logTreeTX) GetLeafDataByIdentityHash(ctx context.Context, leafHashes [][]byte) ([]*trillian.LogLeaf, error) {
	stmt, err := t.ls.wrap.GetLeavesByLeafIdentityHashStmt(t.tx, len(leafHashes))
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	return t.getLeavesByHashInternal(ctx, leafHashes, stmt, "leaf-identity")
}

func (t *logTreeTX) LatestSignedLogRoot(ctx context.Context) (trillian.SignedLogRoot, error) {
	return t.root, nil
}

// fetchLatestRoot reads the latest SignedLogRoot from the DB and returns it.
func (t *logTreeTX) fetchLatestRoot(ctx context.Context) (trillian.SignedLogRoot, error) {
	var timestamp, treeSize, treeRevision int64
	var rootHash, rootSignatureBytes []byte
	var rootSignature spb.DigitallySigned

	stmt, err := t.ls.wrap.GetLatestSignedLogRootStmt(t.tx)
	if err != nil {
		return trillian.SignedLogRoot{}, nil
	}
	defer stmt.Close()
	err = stmt.QueryRowContext(ctx, t.treeID).Scan(
		&timestamp, &treeSize, &rootHash, &treeRevision, &rootSignatureBytes)

	// It's possible there are no roots for this tree yet
	if err == sql.ErrNoRows {
		return trillian.SignedLogRoot{}, nil
	}
	if err != nil {
		return trillian.SignedLogRoot{}, err
	}

	if err := proto.Unmarshal(rootSignatureBytes, &rootSignature); err != nil {
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

	stmt, err := t.ls.wrap.InsertTreeHeadStmt(t.tx)
	if err != nil {
		return err
	}
	defer stmt.Close()
	res, err := stmt.ExecContext(
		ctx,
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

func (t *logTreeTX) UpdateSequencedLeaves(ctx context.Context, leaves []*trillian.LogLeaf) error {
	// TODO: In theory we can do this with CASE / WHEN in one SQL statement but it's more fiddly
	// and can be implemented later if necessary
	stmt, err := t.ls.wrap.InsertSequencedLeafStmt(t.tx)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, leaf := range leaves {
		// This should fail on insert but catch it early
		if len(leaf.LeafIdentityHash) != t.hashSizeBytes {
			return errors.New("Sequenced leaf has incorrect hash size")
		}

		_, err := stmt.ExecContext(
			ctx,
			t.treeID,
			leaf.LeafIdentityHash,
			leaf.MerkleLeafHash,
			leaf.LeafIndex)
		if err != nil {
			glog.Warningf("Failed to update sequenced leaves: %s", err)
			return err
		}
	}

	return nil
}

// removeSequencedLeaves removes the passed in leaves slice (which may be
// modified as part of the operation).
func (t *logTreeTX) removeSequencedLeaves(ctx context.Context, leaves []*trillian.LogLeaf) error {
	// Delete in order of the hash values in the leaves.
	sort.Sort(byLeafIdentityHash(leaves))

	stmt, err := t.ls.wrap.DeleteUnsequencedStmt(t.tx, len(leaves))
	if err != nil {
		glog.Warningf("Failed to get delete statement for sequenced work: %s", err)
		return err
	}
	defer stmt.Close()
	var args []interface{}
	for _, leaf := range leaves {
		args = append(args, interface{}(leaf.LeafIdentityHash))
	}
	args = append(args, interface{}(t.treeID))
	result, err := stmt.ExecContext(ctx, args...)

	if err != nil {
		// Error is handled by checkResultOkAndRowCountIs() below
		glog.Warningf("Failed to delete sequenced work: %s", err)
	}

	err = checkResultOkAndRowCountIs(result, err, int64(len(leaves)))

	if err != nil {
		return err
	}

	return nil
}

func (t *logTreeTX) getLeavesByHashInternal(ctx context.Context, leafHashes [][]byte, tmpl *sql.Stmt, desc string) ([]*trillian.LogLeaf, error) {
	stx := t.tx.StmtContext(ctx, tmpl)
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

	// The tree could include duplicates so we don't know how many results will be returned
	var ret []*trillian.LogLeaf

	defer rows.Close()
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

// GetActiveLogIDs returns a list of the IDs of all configured logs
func (t *logTreeTX) GetActiveLogIDs(ctx context.Context) ([]int64, error) {
	return getActiveLogIDs(ctx, t.tx)
}

// GetActiveLogIDsWithPendingWork returns a list of the IDs of all configured logs
// that have queued unsequenced leaves that need to be integrated
func (t *logTreeTX) GetActiveLogIDsWithPendingWork(ctx context.Context) ([]int64, error) {
	return getActiveLogIDsWithPendingWork(ctx, t.tx)
}

// byLeafIdentityHash allows sorting of leaves by their identity hash, so DB
// operations always happen in a consistent order.
type byLeafIdentityHash []*trillian.LogLeaf

func (l byLeafIdentityHash) Len() int {
	return len(l)
}
func (l byLeafIdentityHash) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}
func (l byLeafIdentityHash) Less(i, j int) bool {
	return bytes.Compare(l[i].LeafIdentityHash, l[j].LeafIdentityHash) == -1
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
