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
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/golang/protobuf/ptypes"
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
	deleteUnsequencedSQL = "DELETE FROM Unsequenced WHERE TreeId=? AND Bucket=0 AND QueueTimestampNanos=? AND LeafIdentityHash=?"
)

type dequeuedLeaf struct {
	queueTimestampNanos int64
	leafIdentityHash    []byte
}

func dequeueInfo(leafIDHash []byte, queueTimestamp int64) dequeuedLeaf {
	return dequeuedLeaf{queueTimestampNanos: queueTimestamp, leafIdentityHash: leafIDHash}
}

func (t *logTreeTX) dequeueLeaf(rows *sql.Rows) (*trillian.LogLeaf, dequeuedLeaf, error) {
	var leafIDHash []byte
	var merkleHash []byte
	var queueTimestamp int64

	err := rows.Scan(&leafIDHash, &merkleHash, &queueTimestamp)
	if err != nil {
		glog.Warningf("Error scanning work rows: %s", err)
		return nil, dequeuedLeaf{}, err
	}

	// Note: the LeafData and ExtraData being nil here is OK as this is only used by the
	// sequencer. The sequencer only writes to the SequencedLeafData table and the client
	// supplied data was already written to LeafData as part of queueing the leaf.
	queueTimestampProto, err := ptypes.TimestampProto(time.Unix(0, queueTimestamp))
	if err != nil {
		return nil, dequeuedLeaf{}, fmt.Errorf("got invalid queue timestamp: %v", err)
	}
	leaf := &trillian.LogLeaf{
		LeafIdentityHash: leafIDHash,
		MerkleLeafHash:   merkleHash,
		QueueTimestamp:   queueTimestampProto,
	}
	return leaf, dequeueInfo(leafIDHash, queueTimestamp), nil
}

func queueArgs(treeID int64, identityHash []byte, queueTimestamp time.Time) []interface{} {
	return []interface{}{queueTimestamp.UnixNano()}
}

func (t *logTreeTX) UpdateSequencedLeaves(ctx context.Context, leaves []*trillian.LogLeaf) error {
	for _, leaf := range leaves {
		// This should fail on insert but catch it early
		if len(leaf.LeafIdentityHash) != t.hashSizeBytes {
			return errors.New("sequenced leaf has incorrect hash size")
		}

		iTimestamp, err := ptypes.Timestamp(leaf.IntegrateTimestamp)
		if err != nil {
			return fmt.Errorf("got invalid integrate timestamp: %v", err)
		}
		_, err = t.tx.ExecContext(
			ctx,
			insertSequencedLeafSQL+valuesPlaceholder5,
			t.treeID,
			leaf.LeafIdentityHash,
			leaf.MerkleLeafHash,
			leaf.LeafIndex,
			iTimestamp.UnixNano())
		if err != nil {
			glog.Warningf("Failed to update sequenced leaves: %s", err)
			return err
		}
	}

	return nil
}

// removeSequencedLeaves removes the passed in leaves slice (which may be
// modified as part of the operation).
func (t *logTreeTX) removeSequencedLeaves(ctx context.Context, leaves []dequeuedLeaf) error {
	// Don't need to re-sort because the query ordered by leaf hash. If that changes because
	// the query is expensive then the sort will need to be done here. See comment in
	// QueueLeaves.
	stx, err := t.tx.PrepareContext(ctx, deleteUnsequencedSQL)
	if err != nil {
		glog.Warningf("Failed to prep delete statement for sequenced work: %v", err)
		return err
	}
	for _, dql := range leaves {
		result, err := stx.ExecContext(ctx, t.treeID, dql.queueTimestampNanos, dql.leafIdentityHash)
		err = checkResultOkAndRowCountIs(result, err, int64(1))
		if err != nil {
			return err
		}
	}

	return nil
}
