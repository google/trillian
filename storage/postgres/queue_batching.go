// +build batched_queue

// Copyright 2017 Google Inc. All Rights Reserved.
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
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian"
)

const (
	// If this statement ORDER BY clause is changed refer to the comment in removeSequencedLeaves
	selectQueuedLeavesSQL = `SELECT leaf_identity_hash,merkle_leaf_hash,queue_timestamp_nanos,queue_id
                        FROM unsequenced
                        WHERE tree_id=$1
                        AND Bucket=0
                        AND queue_timestamp_nanos<=$2
                        ORDER BY queue_timestamp_nanos,leaf_identity_hash ASC LIMIT $3`
	insertUnsequencedEntrySQL = `INSERT INTO unsequenced(tree_id,Bucket,leaf_identity_hash,merkle_leaf_hash,queue_timestamp_nanos,queue_id) VALUES($1,0,$2,$3,$4,$5)`
	deleteUnsequencedSQL      = "DELETE FROM unsequenced WHERE queue_id IN (<placeholder>)"
)

type dequeuedLeaf []byte

func dequeueInfo(_ []byte, queueID []byte) dequeuedLeaf {
	return dequeuedLeaf(queueID)
}

func (t *logTreeTX) dequeueLeaf(rows *sql.Rows) (*trillian.LogLeaf, dequeuedLeaf, error) {
	var leafIDHash []byte
	var merkleHash []byte
	var queueTimestamp int64
	var queueID []byte

	err := rows.Scan(&leafIDHash, &merkleHash, &queueTimestamp, &queueID)
	if err != nil {
		glog.Warningf("Error scanning work rows: %s", err)
		return nil, nil, err
	}

	queueTimestampProto, err := ptypes.TimestampProto(time.Unix(0, queueTimestamp))
	if err != nil {
		return nil, dequeuedLeaf{}, fmt.Errorf("got invalid queue timestamp: %v", err)
	}
	// Note: the LeafData and ExtraData being nil here is OK as this is only used by the
	// sequencer. The sequencer only writes to the SequencedLeafData table and the client
	// supplied data was already written to LeafData as part of queueing the leaf.
	leaf := &trillian.LogLeaf{
		LeafIdentityHash: leafIDHash,
		MerkleLeafHash:   merkleHash,
		QueueTimestamp:   queueTimestampProto,
	}
	return leaf, dequeueInfo(leafIDHash, queueID), nil
}

func generateQueueID(treeID int64, leafIdentityHash []byte, timestamp int64) []byte {
	h := sha256.New()
	b := make([]byte, 10)
	binary.PutVarint(b, treeID)
	h.Write(b)
	b = make([]byte, 10)
	binary.PutVarint(b, timestamp)
	h.Write(b)
	h.Write(leafIdentityHash)
	return h.Sum(nil)
}

func queueArgs(treeID int64, identityHash []byte, queueTimestamp time.Time) []interface{} {
	timestamp := queueTimestamp.UnixNano()
	return []interface{}{timestamp, generateQueueID(treeID, identityHash, timestamp)}
}

func (t *logTreeTX) UpdateSequencedLeaves(ctx context.Context, leaves []*trillian.LogLeaf) error {
	querySuffix := []string{}
	args := []interface{}{}
	for _, leaf := range leaves {
		iTimestamp, err := ptypes.Timestamp(leaf.IntegrateTimestamp)
		if err != nil {
			return fmt.Errorf("got invalid integrate timestamp: %v", err)
		}
		querySuffix = append(querySuffix, valuesPlaceholder5)
		args = append(args, t.treeID, leaf.LeafIdentityHash, leaf.MerkleLeafHash, leaf.LeafIndex, iTimestamp.UnixNano())
	}
	result, err := t.tx.ExecContext(ctx, insertSequencedLeafSQL+strings.Join(querySuffix, ","), args...)
	if err != nil {
		glog.Warningf("Failed to update sequenced leaves: %s", err)
	}
	return checkResultOkAndRowCountIs(result, err, int64(len(leaves)))
}

func (m *postgresLogStorage) getDeleteUnsequencedStmt(ctx context.Context, num int) (*sql.Stmt, error) {
	stmt := &statementSkeleton{
		sql:               deleteUnsequencedSQL,
		firstInsertion:    "%s",
		firstPlaceholders: 1,
		restInsertion:     "%s",
		restPlaceholders:  1,
		num:               num,
	}
	return m.getStmt(ctx, stmt)
}

// removeSequencedLeaves removes the passed in leaves slice (which may be
// modified as part of the operation).
func (t *logTreeTX) removeSequencedLeaves(ctx context.Context, queueIDs []dequeuedLeaf) error {
	// Don't need to re-sort because the query ordered by leaf hash. If that changes because
	// the query is expensive then the sort will need to be done here. See comment in
	// QueueLeaves.
	tmpl, err := t.ls.getDeleteUnsequencedStmt(ctx, len(queueIDs))
	if err != nil {
		glog.Warningf("Failed to get delete statement for sequenced work: %s", err)
		return err
	}
	stx := t.tx.StmtContext(ctx, tmpl)
	args := make([]interface{}, len(queueIDs))
	for i, q := range queueIDs {
		args[i] = []byte(q)
	}
	result, err := stx.ExecContext(ctx, args...)
	if err != nil {
		// Error is handled by checkResultOkAndRowCountIs() below
		glog.Warningf("Failed to delete sequenced work: %s", err)
	}
	return checkResultOkAndRowCountIs(result, err, int64(len(queueIDs)))
}
