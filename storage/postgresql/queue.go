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
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/google/trillian"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/klog/v2"
)

const (
	// If this statement ORDER BY clause is changed refer to the comment in removeSequencedLeaves
	selectQueuedLeavesSQL = "SELECT LeafIdentityHash,MerkleLeafHash,QueueTimestampNanos,QueueID " +
		"FROM Unsequenced " +
		"WHERE TreeId=$1" +
		" AND Bucket=0" +
		" AND QueueTimestampNanos<=$2 " +
		"ORDER BY QueueTimestampNanos,LeafIdentityHash " +
		"LIMIT $3"
	insertUnsequencedEntrySQL = "INSERT INTO Unsequenced(TreeId,Bucket,LeafIdentityHash,MerkleLeafHash,QueueTimestampNanos,QueueID) VALUES($1,0,$2,$3,$4,$5)"
	deleteUnsequencedSQL      = "DELETE FROM Unsequenced WHERE QueueID=ANY($1)"
)

type dequeuedLeaf []byte

func dequeueInfo(_ []byte, queueID []byte) dequeuedLeaf {
	return dequeuedLeaf(queueID)
}

func (t *logTreeTX) dequeueLeaf(rows pgx.Rows) (*trillian.LogLeaf, dequeuedLeaf, error) {
	var leafIDHash []byte
	var merkleHash []byte
	var queueTimestamp int64
	var queueID []byte

	err := rows.Scan(&leafIDHash, &merkleHash, &queueTimestamp, &queueID)
	if err != nil {
		klog.Warningf("Error scanning work rows: %s", err)
		return nil, nil, err
	}

	// Note: the LeafData and ExtraData being nil here is OK as this is only used by the
	// sequencer. The sequencer only writes to the SequencedLeafData table and the client
	// supplied data was already written to LeafData as part of queueing the leaf.
	queueTimestampProto := timestamppb.New(time.Unix(0, queueTimestamp))
	if err := queueTimestampProto.CheckValid(); err != nil {
		return nil, dequeuedLeaf{}, fmt.Errorf("got invalid queue timestamp: %w", err)
	}
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
	rows := make([][]interface{}, 0, len(leaves))
	dequeuedLeaves := make([]dequeuedLeaf, 0, len(leaves))
	for _, leaf := range leaves {
		// This should fail on insert but catch it early
		if len(leaf.LeafIdentityHash) != t.hashSizeBytes {
			return errors.New("sequenced leaf has incorrect hash size")
		}

		if err := leaf.IntegrateTimestamp.CheckValid(); err != nil {
			return fmt.Errorf("got invalid integrate timestamp: %w", err)
		}
		iTimestamp := leaf.IntegrateTimestamp.AsTime()
		rows = append(rows, []interface{}{t.treeID, leaf.LeafIdentityHash, leaf.MerkleLeafHash, leaf.LeafIndex, iTimestamp.UnixNano()})
		qe, ok := t.dequeued[string(leaf.LeafIdentityHash)]
		if !ok {
			return fmt.Errorf("attempting to update leaf that wasn't dequeued. IdentityHash: %x", leaf.LeafIdentityHash)
		}
		dequeuedLeaves = append(dequeuedLeaves, qe)
	}

	// Copy sequenced leaves to SequencedLeafData table.
	n, err := t.tx.CopyFrom(
		ctx,
		pgx.Identifier{"sequencedleafdata"},
		[]string{"treeid", "leafidentityhash", "merkleleafhash", "sequencenumber", "integratetimestampnanos"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		klog.Warningf("Failed to copy sequenced leaves: %s", err)
	}
	if err := checkResultOkAndCopyCountIs(n, err, int64(len(leaves))); err != nil {
		return err
	}

	return t.removeSequencedLeaves(ctx, dequeuedLeaves)
}

// removeSequencedLeaves removes the passed in leaves slice (which may be
// modified as part of the operation).
func (t *logTreeTX) removeSequencedLeaves(ctx context.Context, queueIDs []dequeuedLeaf) error {
	start := time.Now()
	// Don't need to re-sort because the query ordered by leaf hash. If that changes because
	// the query is expensive then the sort will need to be done here. See comment in
	// QueueLeaves.
	result, err := t.tx.Exec(ctx, deleteUnsequencedSQL, queueIDs)
	if err != nil {
		// Error is handled by checkResultOkAndRowCountIs() below
		klog.Warningf("Failed to delete sequenced work: %s", err)
	}

	err = checkResultOkAndRowCountIs(result, err, int64(len(queueIDs)))
	if err != nil {
		return err
	}

	observe(dequeueRemoveLatency, time.Since(start), labelForTX(t))
	return nil
}
