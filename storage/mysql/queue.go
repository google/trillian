// +build !batched_queue

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

package mysql

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
)

const (
	// If this statement ORDER BY clause is changed refer to the comment in removeSequencedLeaves
	selectQueuedLeavesSQL = `SELECT LeafIdentityHash,MerkleLeafHash,QueueTimestampNanos
			FROM Unsequenced
			WHERE TreeID=?
			AND Bucket=0
			AND QueueTimestampNanos<=?
			ORDER BY QueueTimestampNanos,LeafIdentityHash ASC LIMIT ?`
	insertUnsequencedEntrySQL = `INSERT INTO Unsequenced(TreeId,Bucket,LeafIdentityHash,MerkleLeafHash,QueueTimestampNanos)
			VALUES(?,0,?,?,?)`
	insertSequencedLeafSQL = `INSERT INTO SequencedLeafData(TreeId,LeafIdentityHash,MerkleLeafHash,SequenceNumber)
			VALUES(?,?,?,?)`
	deleteUnsequencedSQL = "DELETE FROM Unsequenced WHERE TreeId=? AND Bucket=0 AND QueueTimestampNanos=? AND LeafIdentityHash=?"
)

// dequeuedLeaf is used internally and contains some data that is not part of the client API.
type dequeuedLeaf struct {
	queueTimestampNanos int64
	leafIdentityHash    []byte
}

func (t *logTreeTX) dequeueLeaf(rows *sql.Rows) (*trillian.LogLeaf, interface{}, error) {
	var leafIDHash []byte
	var merkleHash []byte
	var queueTimeNanos int64

	err := rows.Scan(&leafIDHash, &merkleHash, &queueTimeNanos)
	if err != nil {
		glog.Warningf("Error scanning work rows: %s", err)
		return nil, nil, err
	}

	// Note: the LeafData and ExtraData being nil here is OK as this is only used by the
	// sequencer. The sequencer only writes to the SequencedLeafData table and the client
	// supplied data was already written to LeafData as part of queueing the leaf.
	leaf := &trillian.LogLeaf{
		LeafIdentityHash: leafIDHash,
		MerkleLeafHash:   merkleHash,
	}
	return leaf, &dequeuedLeaf{queueTimestampNanos: queueTimeNanos, leafIdentityHash: leafIDHash}, nil
}

func queueArgs(treeID int64, identityHash []byte, queueTimestamp time.Time) []interface{} {
	return []interface{}{queueTimestamp.UnixNano()}
}

func (t *logTreeTX) UpdateSequencedLeaves(ctx context.Context, leaves []*trillian.LogLeaf) error {
	// TODO: In theory we can do this with CASE / WHEN in one SQL statement but it's more fiddly
	// and can be implemented later if necessary
	for _, leaf := range leaves {
		// This should fail on insert but catch it early
		if len(leaf.LeafIdentityHash) != t.hashSizeBytes {
			return errors.New("Sequenced leaf has incorrect hash size")
		}

		_, err := t.tx.ExecContext(
			ctx,
			insertSequencedLeafSQL,
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
func (t *logTreeTX) removeSequencedLeaves(ctx context.Context, leaves []interface{}) error {
	// Don't need to re-sort because the query ordered by leaf hash. If that changes because
	// the query is expensive then the sort will need to be done here. See comment in
	// QueueLeaves.
	stx, err := t.tx.PrepareContext(ctx, deleteUnsequencedSQL)
	if err != nil {
		glog.Warningf("Failed to prep delete statement for sequenced work: %v", err)
		return err
	}
	for _, obj := range leaves {
		dql, ok := obj.(*dequeuedLeaf)
		if !ok {
			return errors.New("non-dequeuedLeaf passed to removeSequencedLeaves")
		}
		result, err := stx.ExecContext(ctx, t.treeID, dql.queueTimestampNanos, dql.leafIdentityHash)
		err = checkResultOkAndRowCountIs(result, err, int64(1))
		if err != nil {
			return err
		}
	}

	return nil
}
