package mysql

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/cache"
)

const getTreePropertiesSQL string = "SELECT AllowsDuplicateLeaves FROM Trees WHERE TreeId=?"
const getTreeParametersSQL string = "SELECT ReadOnlyRequests From TreeControl WHERE TreeID=?"
const selectQueuedLeavesSQL string = `SELECT LeafValueHash,Payload
		 FROM Unsequenced
		 WHERE TreeID=?
		 AND QueueTimestampNanos<=?
		 ORDER BY QueueTimestampNanos DESC,LeafValueHash ASC LIMIT ?`
const insertUnsequencedLeafSQL string = `INSERT INTO LeafData(TreeId,LeafValueHash,LeafValue)
		 VALUES(?,?,?) ON DUPLICATE KEY UPDATE LeafValueHash=LeafValueHash`
const insertUnsequencedLeafSQLNoDuplicates string = `INSERT INTO LeafData(TreeId,LeafValueHash,LeafValue)
		 VALUES(?,?,?)`
const insertUnsequencedEntrySQL string = `INSERT INTO Unsequenced(TreeId,LeafValueHash,MessageId,Payload,QueueTimestampNanos)
     VALUES(?,?,?,?,?)`
const insertSequencedLeafSQL string = `INSERT INTO SequencedLeafData(TreeId,LeafValueHash,MerkleLeafHash,SequenceNumber)
		 VALUES(?,?,?,?)`
const selectSequencedLeafCountSQL string = "SELECT COUNT(*) FROM SequencedLeafData WHERE TreeId=?"
const selectLatestSignedLogRootSQL string = `SELECT TreeHeadTimestamp,TreeSize,RootHash,TreeRevision,RootSignature
		 FROM TreeHead WHERE TreeId=?
		 ORDER BY TreeHeadTimestamp DESC LIMIT 1`

// These statements need to be expanded to provide the correct number of parameter placeholders
// for a particular case
const deleteUnsequencedSQL string = "DELETE FROM Unsequenced WHERE LeafValueHash IN (<placeholder>) AND TreeId = ?"
const selectLeavesByIndexSQL string = `SELECT s.MerkleLeafHash,l.LeafValueHash,l.LeafValue,s.SequenceNumber
		     FROM LeafData l,SequencedLeafData s
		     WHERE l.LeafValueHash = s.LeafValueHash
		     AND s.SequenceNumber IN (` + placeholderSQL + `) AND l.TreeId = ? AND s.TreeId = l.TreeId`
const selectLeavesByHashSQL string = `SELECT s.MerkleLeafHash,l.LeafValueHash,l.LeafValue,s.SequenceNumber
		     FROM LeafData l,SequencedLeafData s
		     WHERE l.LeafValueHash = s.LeafValueHash
		     AND s.MerkleLeafHash IN (` + placeholderSQL + `) AND l.TreeId = ? AND s.TreeId = l.TreeId`

// Same as above except with leaves ordered by sequence so we only incur this cost when necessary
const selectLeavesByHashOrderedBySequenceSQL string = selectLeavesByHashSQL + " ORDER BY s.SequenceNumber"

var defaultLogStrata = []int{8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8}

type mySQLLogStorage struct {
	*mySQLTreeStorage

	// These options can only sensibly be set when storage is initialized
	logID           int64
	allowDuplicates bool

	// These options can reasonably be changed during operation
	readOnly        bool
}

// NewLogStorage creates a mySQLLogStorage instance for the specified MySQL URL.
func NewLogStorage(id int64, dbURL string) (storage.LogStorage, error) {
	// TODO(al): pass this through/configure from DB
	th := merkle.NewRFC6962TreeHasher(crypto.NewSHA256())
	ts, err := newTreeStorage(id, dbURL, th.Size(), defaultLogStrata, cache.PopulateLogSubtreeNodes(th))
	if err != nil {
		glog.Warningf("Couldn't create a new treeStorage: %s", err)
		return nil, err
	}

	s := mySQLLogStorage{
		mySQLTreeStorage: ts,
		logID:            id,
	}

	// TODO: This should not default but it would currently complicate testing and can be
	// implemented later when the create tree API has been defined.
	if err := s.db.QueryRow(getTreePropertiesSQL, id).Scan(&s.allowDuplicates); err == sql.ErrNoRows {
		s.allowDuplicates = false
	} else if err != nil {
		glog.Warningf("Failed to get trees row for id %v: %s", id, err)
		return nil, err
	}

	err = s.db.QueryRow(getTreeParametersSQL, id).Scan(&s.readOnly)

	// TODO(Martin2112): It's probably not ok for the log to have no parameters set. Enforce this when
	// we have an admin API and / or we're further along.
	if err == sql.ErrNoRows {
		glog.Warningf("*** Opening storage for log: %v but it has no params configured ***", id)
	}

	return &s, nil
}

func (m *mySQLLogStorage) getLeavesByIndexStmt(num int) (*sql.Stmt, error) {
	return m.getStmt(selectLeavesByIndexSQL, num, "?", "?")
}

func (m *mySQLLogStorage) getLeavesByHashStmt(num int, orderBySequence bool) (*sql.Stmt, error) {
	if orderBySequence {
		return m.getStmt(selectLeavesByHashOrderedBySequenceSQL, num, "?", "?")
	}

	return m.getStmt(selectLeavesByHashSQL, num, "?", "?")
}

func (m *mySQLLogStorage) getDeleteUnsequencedStmt(num int) (*sql.Stmt, error) {
	return m.getStmt(deleteUnsequencedSQL, num, "?", "?")
}

func (m *mySQLLogStorage) LatestSVignedLogRoot() (trillian.SignedLogRoot, error) {
	t, err := m.Begin()

	if err != nil {
		return trillian.SignedLogRoot{}, err
	}

	defer t.Commit()
	return t.LatestSignedLogRoot()
}

func (m *mySQLLogStorage) GetSequencedLeafCount() (int64, error) {
	t, err := m.Begin()

	if err != nil {
		return 0, err
	}

	defer t.Commit()
	return t.GetSequencedLeafCount()
}

func (m *mySQLLogStorage) GetLeavesByIndex(leaves []int64) ([]trillian.LogLeaf, error) {
	t, err := m.Begin()

	if err != nil {
		return []trillian.LogLeaf{}, err
	}
	defer t.Commit()
	return t.GetLeavesByIndex(leaves)
}

func (m *mySQLLogStorage) GetLeavesByHash(leafHashes [][]byte, orderBySequence bool) ([]trillian.LogLeaf, error) {
	t, err := m.Begin()

	if err != nil {
		return []trillian.LogLeaf{}, err
	}
	defer t.Commit()
	return t.GetLeavesByHash(leafHashes, orderBySequence)
}

func (m *mySQLLogStorage) beginInternal() (storage.LogTX, error) {
	ttx, err := m.beginTreeTx()
	if err != nil {
		return nil, err
	}
	ret := &logTX{
		treeTX: ttx,
		ls:     m,
	}

	root, err := ret.LatestSignedLogRoot()
	if err != nil {
		ttx.Rollback()
		return nil, err
	}

	ret.treeTX.writeRevision = root.TreeRevision + 1

	return ret, nil
}

func (m *mySQLLogStorage) Begin() (storage.LogTX, error) {
	// Reject attempts to start a writable transaction in read only mode. Anything that
	// doesn't write is a part of Snapshot so is still available via that API.
	if m.readOnly {
		return nil, storage.ErrReadOnly
	}

	return m.beginInternal()
}

func (m *mySQLLogStorage) Snapshot() (storage.ReadOnlyLogTX, error) {
	tx, err := m.beginInternal()
	if err != nil {
		return nil, err
	}
	return tx.(storage.ReadOnlyLogTX), err
}

type logTX struct {
	treeTX
	ls *mySQLLogStorage
}

func (t *logTX) WriteRevision() int64 {
	return t.treeTX.writeRevision
}

func (t *logTX) DequeueLeaves(limit int, cutoffTime time.Time) ([]trillian.LogLeaf, error) {
	stx, err := t.tx.Prepare(selectQueuedLeavesSQL)

	if err != nil {
		glog.Warningf("Failed to prepare dequeue select: %s", err)
		return nil, err
	}

	leaves := make([]trillian.LogLeaf, 0, limit)
	rows, err := stx.Query(t.ls.logID, cutoffTime.UnixNano(), limit)

	if err != nil {
		glog.Warningf("Failed to select rows for work: %s", err)
		return nil, err
	}

	defer rows.Close()

	for rows.Next() {
		var leafHash []byte
		var payload []byte

		err := rows.Scan(&leafHash, &payload)

		if err != nil {
			glog.Warningf("Error scanning work rows: %s", err)
			return nil, err
		}

		if len(leafHash) != t.ts.hashSizeBytes {
			return nil, errors.New("Dequeued a leaf with incorrect hash size")
		}

		leaf := trillian.LogLeaf{
			LeafValueHash: leafHash,
			LeafValue:     payload,
			ExtraData:     nil,
		}
		leaves = append(leaves, leaf)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	// The convention is that if leaf processing succeeds (by committing this tx)
	// then the unsequenced entries for them are removed
	if len(leaves) > 0 {
		err = t.removeSequencedLeaves(leaves)
	}

	if err != nil {
		return nil, err
	}

	return leaves, nil
}

func (t *logTX) QueueLeaves(leaves []trillian.LogLeaf, queueTimestamp time.Time) error {
	// Don't accept batches if any of the leaves are invalid.
	for _, leaf := range leaves {
		if len(leaf.LeafValueHash) != t.ts.hashSizeBytes {
			return fmt.Errorf("queued leaf must have a hash of length %d", t.ts.hashSizeBytes)
		}

		// Validate the hash as a consistency check that the data was received OK. Note: at
		// this stage it is not a Merkle tree hash for the leaf.
		if got, want := crypto.NewSHA256().Digest(leaf.LeafValue), leaf.LeafValueHash; !bytes.Equal(got, want) {
			return fmt.Errorf("leaf value / data hash mismatch got: %v, want: %v", got, want)
		}
	}

	// If the log does not allow duplicates we prevent the insert of such a leaf from
	// succeeding. If duplicates are allowed multiple sequenced leaves will share the same
	// leaf data in the database.
	var insertSQL string

	if t.ls.allowDuplicates {
		insertSQL = insertUnsequencedLeafSQL
	} else {
		insertSQL = insertUnsequencedLeafSQLNoDuplicates
	}

	for i, leaf := range leaves {
		// Create the unsequenced leaf data entry. We don't use INSERT IGNORE because this
		// can suppress errors unrelated to key collisions. We don't use REPLACE because
		// if there's ever a hash collision it will do the wrong thing and it also
		// causes a DELETE / INSERT, which is undesirable.
		_, err := t.tx.Exec(insertSQL, t.ls.logID, leaf.LeafValueHash, leaf.LeafValue)

		if err != nil {
			glog.Warningf("Error inserting %d into LeafData: %s", i, err)
			return fmt.Errorf("LeafData: %d, %v", i, err)
		}

		// Create the work queue entry
		// Message ids only need to guard against duplicates for the time that entries are
		// in the unsequenced queue, which should be short, but we'll still use a strong hash.
		// TODO(alcutter): get this from somewhere else
		hasher := sha256.New()

		// We use a fixed zero message id if the log disallows duplicates otherwise a random one.
		// the fixed id will collide if dups submitted when not allowed so the insert won't succeed
		// and everything will get rolled back
		messageIDBytes := make([]byte, 8)

		if t.ls.allowDuplicates {
			_, err := rand.Read(messageIDBytes)

			if err != nil {
				glog.Warningf("Failed to get a random message id: %s", err)
				return err
			}
		}

		hasher.Write(messageIDBytes)
		binary.Write(hasher, binary.LittleEndian, t.ls.logID)
		hasher.Write(leaf.LeafValueHash)
		messageID := hasher.Sum(nil)

		_, err = t.tx.Exec(insertUnsequencedEntrySQL,
			t.ls.logID, leaf.LeafValueHash, messageID, leaf.LeafValue, queueTimestamp.UnixNano())

		if err != nil {
			glog.Warningf("Error inserting into Unsequenced: %s", err)
			return fmt.Errorf("Unsequenced: %v", err)
		}
	}

	return nil
}

func (t *logTX) GetSequencedLeafCount() (int64, error) {
	var sequencedLeafCount int64

	err := t.tx.QueryRow(selectSequencedLeafCountSQL, t.ls.logID).Scan(&sequencedLeafCount)

	if err != nil {
		glog.Warningf("Error getting sequenced leaf count: %s", err)
	}

	return sequencedLeafCount, err
}

func (t *logTX) GetLeavesByIndex(leaves []int64) ([]trillian.LogLeaf, error) {
	tmpl, err := t.ls.getLeavesByIndexStmt(len(leaves))
	if err != nil {
		return nil, err
	}
	stx := t.tx.Stmt(tmpl)
	args := make([]interface{}, 0)
	for _, nodeID := range leaves {
		args = append(args, interface{}(int64(nodeID)))
	}
	args = append(args, interface{}(t.ls.logID))
	rows, err := stx.Query(args...)
	if err != nil {
		glog.Warningf("Failed to get leaves by idx: %s", err)
		return nil, err
	}

	ret := make([]trillian.LogLeaf, len(leaves))
	num := 0

	defer rows.Close()
	for rows.Next() {
		if err := rows.Scan(&ret[num].MerkleLeafHash, &ret[num].LeafValueHash, &ret[num].LeafValue, &ret[num].LeafIndex); err != nil {
			glog.Warningf("Failed to scan merkle leaves: %s", err)
			return nil, err
		}

		if got, want := len(ret[num].MerkleLeafHash), t.ts.hashSizeBytes; got != want {
			return nil, fmt.Errorf("Scanned leaf does not have hash length %d, got %d", want, got)
		}

		num++
	}

	if num != len(leaves) {
		return nil, fmt.Errorf("expected %d leaves, but saw %d", len(leaves), num)
	}
	return ret, nil
}

func (t *logTX) GetLeavesByHash(leafHashes [][]byte, orderBySequence bool) ([]trillian.LogLeaf, error) {
	tmpl, err := t.ls.getLeavesByHashStmt(len(leafHashes), orderBySequence)

	if err != nil {
		return nil, err
	}
	stx := t.tx.Stmt(tmpl)
	args := make([]interface{}, 0)
	for _, hash := range leafHashes {
		args = append(args, interface{}([]byte(hash)))
	}
	args = append(args, interface{}(t.ls.logID))
	rows, err := stx.Query(args...)
	if err != nil {
		glog.Warningf("Failed to get leaves by hash: %s", err)
		return nil, err
	}

	// The tree could include duplicates so we don't know how many results will be returned
	ret := make([]trillian.LogLeaf, 0)

	defer rows.Close()
	for rows.Next() {
		leaf := trillian.LogLeaf{}

		if err := rows.Scan(&leaf.MerkleLeafHash, &leaf.LeafValueHash, &leaf.LeafValue, &leaf.LeafIndex); err != nil {
			glog.Warningf("Failed to scan merkle leaves: %s", err)
			return nil, err
		}

		if got, want := len(leaf.MerkleLeafHash), t.ls.hashSizeBytes; got != want {
			return nil, fmt.Errorf("Scanned leaf does not have hash length %d, got %d", want, got)
		}

		ret = append(ret, leaf)
	}

	return ret, nil
}

func (t *logTX) LatestSignedLogRoot() (trillian.SignedLogRoot, error) {
	var timestamp, treeSize, treeRevision int64
	var rootHash, rootSignatureBytes []byte
	var rootSignature trillian.DigitallySigned

	err := t.tx.QueryRow(
		selectLatestSignedLogRootSQL, t.ls.logID).Scan(
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
		LogId:          t.ls.logID,
		TreeSize:       treeSize,
	}, nil
}

func (t *logTX) StoreSignedLogRoot(root trillian.SignedLogRoot) error {
	signatureBytes, err := proto.Marshal(root.Signature)

	if err != nil {
		glog.Warningf("Failed to marshal root signature: %v %v", root.Signature, err)
		return err
	}

	res, err := t.tx.Exec(insertTreeHeadSQL, t.ls.logID, root.TimestampNanos, root.TreeSize,
		root.RootHash, root.TreeRevision, signatureBytes)

	if err != nil {
		glog.Warningf("Failed to store signed root: %s", err)
	}

	return checkResultOkAndRowCountIs(res, err, 1)
}

func (t *logTX) UpdateSequencedLeaves(leaves []trillian.LogLeaf) error {
	// TODO: In theory we can do this with CASE / WHEN in one SQL statement but it's more fiddly
	// and can be implemented later if necessary
	for _, leaf := range leaves {
		// This should fail on insert but catch it early
		if len(leaf.LeafValueHash) != t.ts.hashSizeBytes {
			return errors.New("Sequenced leaf has incorrect hash size")
		}

		_, err := t.tx.Exec(insertSequencedLeafSQL, t.ls.logID, leaf.LeafValueHash, leaf.MerkleLeafHash,
			leaf.LeafIndex)

		if err != nil {
			glog.Warningf("Failed to update sequenced leaves: %s", err)
			return err
		}
	}

	return nil
}

func (t *logTX) removeSequencedLeaves(leaves []trillian.LogLeaf) error {
	tmpl, err := t.ls.getDeleteUnsequencedStmt(len(leaves))
	if err != nil {
		glog.Warningf("Failed to get delete statement for sequenced work: %s", err)
		return err
	}
	stx := t.tx.Stmt(tmpl)
	args := make([]interface{}, 0)
	for _, leaf := range leaves {
		args = append(args, interface{}(leaf.LeafValueHash))
	}
	args = append(args, interface{}(t.ls.logID))
	result, err := stx.Exec(args...)

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

func (t *logTX) getActiveLogIDsInternal(sql string) ([]int64, error) {
	rows, err := t.tx.Query(sql)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	logIDs := make([]int64, 0, 0)

	for rows.Next() {
		var keyID []byte
		var treeID int64

		if err := rows.Scan(&treeID, &keyID); err != nil {
			return []int64{}, err
		}

		logIDs = append(logIDs, treeID)
	}

	if rows.Err() != nil {
		return []int64{}, rows.Err()
	}

	return logIDs, nil
}

// GetActiveLogIDs returns a list of the IDs of all configured logs
func (t *logTX) GetActiveLogIDs() ([]int64, error) {
	return t.getActiveLogIDsInternal(selectActiveLogsSQL)
}

// GetActiveLogIDsWithPendingWork returns a list of the IDs of all configured logs
// that have queued unsequenced leaves that need to be integrated
func (t *logTX) GetActiveLogIDsWithPendingWork() ([]int64, error) {
	return t.getActiveLogIDsInternal(selectActiveLogsWithUnsequencedSQL)
}
