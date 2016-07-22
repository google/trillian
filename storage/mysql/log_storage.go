package mysql

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
)

const getTreePropertiesSql string = "SELECT AllowsDuplicateLeaves FROM Trees WHERE TreeId=?"
const getTreeParametersSql string = "SELECT ReadOnlyRequests From TreeControl WHERE TreeID=?"
const selectQueuedLeavesSql string = `SELECT LeafHash,Payload,SignedEntryTimestamp
		 FROM Unsequenced
		 WHERE TreeID=?
		 ORDER BY QueueTimestamp DESC LIMIT ?`
const insertUnsequencedLeafSql string = `INSERT INTO LeafData(TreeId,LeafHash,TheData)
		 VALUES(?,?,?) ON DUPLICATE KEY UPDATE LeafHash=LeafHash`
const insertUnsequencedEntrySql string = `INSERT INTO Unsequenced(TreeId,LeafHash,MessageId,SignedEntryTimestamp,Payload)
     VALUES(?,?,?,?,?)`
const insertSequencedLeafSql string = `INSERT INTO SequencedLeafData(TreeId,LeafHash,SequenceNumber,SignedEntryTimestamp)
		 VALUES(?,?,?,?)`
const selectSequencedLeafCountSql string = "SELECT COUNT(*) FROM SequencedLeafData"
const selectLatestSignedLogRootSql string = `SELECT TreeHeadTimestamp,TreeSize,RootHash,TreeRevision,RootSignature
		 FROM TreeHead WHERE TreeId=?
		 ORDER BY TreeHeadTimestamp DESC LIMIT 1`

// These statements need to be expanded to provide the correct number of parameter placeholders
// for a particular case
const deleteUnsequencedSql string = "DELETE FROM Unsequenced WHERE LeafHash IN (<placeholder>) AND TreeId = ?"
const selectLeavesByIndexSql string = `SELECT l.LeafHash,l.TheData,s.SequenceNumber,s.SignedEntryTimestamp
		     FROM LeafData l,SequencedLeafData s
		     WHERE l.LeafHash = s.LeafHash
		     AND s.SequenceNumber IN (` + placeholderSql + `) AND l.TreeId = ? AND s.TreeId = l.TreeId`
const selectLeavesByHashSql string = `SELECT l.LeafHash,l.TheData,s.SequenceNumber,s.SignedEntryTimestamp
		     FROM LeafData l,SequencedLeafData s
		     WHERE l.LeafHash = s.LeafHash
		     AND l.LeafHash IN (` + placeholderSql + `) AND l.TreeId = ? AND s.TreeId = l.TreeId`

type mySQLLogStorage struct {
	mySQLTreeStorage

	logID           trillian.LogID
	allowDuplicates bool
	readOnly        bool
}

func NewLogStorage(id trillian.LogID, dbURL string) (storage.LogStorage, error) {
	ts, err := newTreeStorage(id.TreeID, dbURL, trillian.NewSHA256())
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
	if err := s.db.QueryRow(getTreePropertiesSql, id.TreeID).Scan(&s.allowDuplicates); err == sql.ErrNoRows {
		s.allowDuplicates = false
	} else if err != nil {
		glog.Warningf("Failed to get trees row for id %v: %s", id, err)
		return nil, err
	}

	err = s.db.QueryRow(getTreeParametersSql, id.TreeID).Scan(&s.readOnly)

	// TODO(Martin2112): It's probably not ok for the log to have no parameters set. Enforce this when
	// we have an admin API and / or we're further along.
	if err == sql.ErrNoRows {
		glog.Warningf("*** Opening storage for log: %v but it has no params configured ***", id)
	}

	if s.setSubtree, err = s.db.Prepare(insertSubtreeSql); err != nil {
		glog.Warningf("Failed to prepare node insert subtree statement: %s", err)
		return nil, err
	}

	return &s, nil
}

func (m *mySQLLogStorage) getLeavesByIndexStmt(num int) (*sql.Stmt, error) {
	return m.getStmt(selectLeavesByIndexSql, num)
}

func (m *mySQLLogStorage) getLeavesByHashStmt(num int) (*sql.Stmt, error) {
	return m.getStmt(selectLeavesByHashSql, num)
}

func (m *mySQLLogStorage) getDeleteUnsequencedStmt(num int) (*sql.Stmt, error) {
	return m.getStmt(deleteUnsequencedSql, num)
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

func (m *mySQLLogStorage) GetLeavesByHash(leafHashes []trillian.Hash) ([]trillian.LogLeaf, error) {
	t, err := m.Begin()

	if err != nil {
		return []trillian.LogLeaf{}, err
	}
	defer t.Commit()
	return t.GetLeavesByHash(leafHashes)
}

func (m *mySQLLogStorage) beginInternal() (storage.LogTX, error) {
	ttx, err := m.beginTreeTx()
	if err != nil {
		return nil, err
	}
	return &logTX{
		treeTX: ttx,
		ls:     m,
	}, nil
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

func (t *logTX) DequeueLeaves(limit int) ([]trillian.LogLeaf, error) {
	stx, err := t.tx.Prepare(selectQueuedLeavesSql)

	if err != nil {
		glog.Warningf("Failed to prepare dequeue select: %s", err)
		return nil, err
	}

	leaves := make([]trillian.LogLeaf, 0, limit)
	rows, err := stx.Query(t.ls.logID.TreeID, limit)

	if err != nil {
		glog.Warningf("Failed to select rows for work: %s", err)
		return nil, err
	}

	defer rows.Close()

	for rows.Next() {
		var leafHash []byte
		var payload []byte
		var signedEntryTimestampBytes []byte

		err := rows.Scan(&leafHash, &payload, &signedEntryTimestampBytes)

		if err != nil {
			glog.Warningf("Error scanning work rows: %s", err)
			return nil, err
		}

		if len(leafHash) != t.ts.hashSizeBytes {
			return nil, errors.New("Dequeued a leaf with incorrect hash size")
		}

		signedEntryTimestamp, err := decodeSignedTimestamp(signedEntryTimestampBytes)

		if err != nil {
			return nil, err
		}

		leaf := trillian.LogLeaf{
			Leaf: trillian.Leaf{
				LeafHash:  leafHash,
				LeafValue: payload,
				ExtraData: nil,
			},
			SignedEntryTimestamp: signedEntryTimestamp,
			SequenceNumber:       0,
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

func (t *logTX) QueueLeaves(leaves []trillian.LogLeaf) error {
	// Don't accept batches if any of the leaves are invalid.
	for _, leaf := range leaves {
		if len(leaf.LeafHash) != t.ts.hashSizeBytes {
			return fmt.Errorf("Queued leaf must have a hash of length %d", t.ts.hashSizeBytes)
		}

		if len(leaf.SignedEntryTimestamp.Signature.Signature) == 0 {
			return errors.New("Queued leaf cannot have an empty signature")
		}
	}

	for _, leaf := range leaves {
		// Create the unsequenced leaf data entry. We don't use INSERT IGNORE because this
		// can suppress errors unrelated to key collisions. We don't use REPLACE because
		// if there's ever a hash collision it will do the wrong thing and it also
		// causes a DELETE / INSERT, which is undesirable.
		_, err := t.tx.Exec(insertUnsequencedLeafSql, t.ls.logID.TreeID,
			[]byte(leaf.LeafHash), leaf.LeafValue)

		if err != nil {
			glog.Warningf("Error inserting into LeafData: %s", err)
			return err
		}

		// Create the work queue entry
		// Message ids only need to guard against duplicates for the time that entries are
		// in the unsequenced queue, which should be short, but we'll still use a strong hash.
		// TODO(alcutter): get this from somewhere else
		hasher := sha256.New()

		// We use a fixed zero message id if the log disallows duplicates otherwise a random one.
		// the fixed id will collide if dups submitted when not allowed so the insert won't succeed
		// and everything will get rolled back
		messageIdBytes := make([]byte, 8)

		if t.ls.allowDuplicates {
			_, err := rand.Read(messageIdBytes)

			if err != nil {
				glog.Warningf("Failed to get a random message id: %s", err)
				return err
			}
		}

		hasher.Write(messageIdBytes)
		hasher.Write(t.ls.logID.LogID)
		hasher.Write(leaf.LeafHash)
		messageId := hasher.Sum(nil)

		signedTimestampBytes, err := EncodeSignedTimestamp(leaf.SignedEntryTimestamp)

		if err != nil {
			return err
		}

		// TODO: We shouldn't really need both payload and signed timestamp fields in unsequenced
		// I think payload is currently unused
		_, err = t.tx.Exec(insertUnsequencedEntrySql,
			t.ls.logID.TreeID, []byte(leaf.LeafHash), messageId, signedTimestampBytes, signedTimestampBytes)

		if err != nil {
			glog.Warningf("Error inserting into Unsequenced: %s", err)
			return err
		}
	}

	return nil
}

func (t *logTX) GetSequencedLeafCount() (int64, error) {
	var sequencedLeafCount int64
	err := t.tx.QueryRow(selectSequencedLeafCountSql).Scan(&sequencedLeafCount)

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
	args = append(args, interface{}(t.ls.logID.TreeID))
	rows, err := stx.Query(args...)
	if err != nil {
		glog.Warningf("Failed to get leaves by idx: %s", err)
		return nil, err
	}

	ret := make([]trillian.LogLeaf, len(leaves))
	num := 0

	var signedTimestampBytes []byte

	defer rows.Close()
	for rows.Next() {
		if err := rows.Scan(&ret[num].LeafHash, &ret[num].LeafValue, &ret[num].SequenceNumber,
			&signedTimestampBytes); err != nil {
			glog.Warningf("Failed to scan merkle leaves: %s", err)
			return nil, err
		}

		signedEntryTimestamp, err := decodeSignedTimestamp(signedTimestampBytes)

		if err != nil {
			return nil, err
		}

		ret[num].SignedEntryTimestamp = signedEntryTimestamp

		if got, want := len(ret[num].LeafHash), t.ts.hashSizeBytes; got != want {
			return nil, fmt.Errorf("Scanned leaf does not have hash length %d, got %d", want, got)
		}

		num++
	}

	if num != len(leaves) {
		return nil, fmt.Errorf("expected %d leaves, but saw %d", len(leaves), num)
	}
	return ret, nil
}

func (t *logTX) GetLeavesByHash(leafHashes []trillian.Hash) ([]trillian.LogLeaf, error) {
	tmpl, err := t.ls.getLeavesByHashStmt(len(leafHashes))
	if err != nil {
		return nil, err
	}
	stx := t.tx.Stmt(tmpl)
	args := make([]interface{}, 0)
	for _, hash := range leafHashes {
		args = append(args, interface{}([]byte(hash)))
	}
	args = append(args, interface{}(t.ls.logID.TreeID))
	rows, err := stx.Query(args...)
	if err != nil {
		glog.Warningf("Failed to get leaves by hash: %s", err)
		return nil, err
	}

	// The tree could include duplicates so we don't know how many results will be returned
	ret := make([]trillian.LogLeaf, 0)

	var signedTimestampBytes []byte

	defer rows.Close()
	for rows.Next() {
		leaf := trillian.LogLeaf{}

		if err := rows.Scan(&leaf.LeafHash, &leaf.LeafValue, &leaf.SequenceNumber, &signedTimestampBytes); err != nil {
			glog.Warningf("Failed to scan merkle leaves: %s", err)
			return nil, err
		}

		signedEntryTimestamp, err := decodeSignedTimestamp(signedTimestampBytes)

		if err != nil {
			return nil, err
		}

		leaf.SignedEntryTimestamp = signedEntryTimestamp

		if got, want := len(leaf.LeafHash), t.ls.hashSizeBytes; got != want {
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
		selectLatestSignedLogRootSql, t.ls.logID.TreeID).Scan(
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
		LogId:          t.ls.logID.LogID,
		TreeSize:       treeSize,
	}, nil
}

func (t *logTX) StoreSignedLogRoot(root trillian.SignedLogRoot) error {
	signatureBytes, err := proto.Marshal(root.Signature)

	if err != nil {
		glog.Warningf("Failed to marshal root signature: %v %v", root.Signature, err)
		return err
	}

	res, err := t.tx.Exec(insertTreeHeadSql, t.ls.logID.TreeID, root.TimestampNanos, root.TreeSize,
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
		if len(leaf.LeafHash) != t.ts.hashSizeBytes {
			return errors.New("Sequenced leaf has incorrect hash size")
		}

		signedTimestampBytes, err := EncodeSignedTimestamp(leaf.SignedEntryTimestamp)

		if err != nil {
			return err
		}

		_, err = t.tx.Exec(insertSequencedLeafSql, t.ls.logID.TreeID, []byte(leaf.LeafHash),
			leaf.SequenceNumber, signedTimestampBytes)

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
		args = append(args, interface{}([]byte(leaf.LeafHash)))
	}
	args = append(args, interface{}(t.ls.logID.TreeID))
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

func (t *logTX) getActiveLogIDsInternal(sql string) ([]trillian.LogID, error) {
	rows, err := t.tx.Query(sql)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	logIDs := make([]trillian.LogID, 0, 0)

	for rows.Next() {
		var logID []byte
		var treeID int64

		if err := rows.Scan(&treeID, &logID); err != nil {
			return []trillian.LogID{}, err
		}

		logIDs = append(logIDs, trillian.LogID{logID, treeID})
	}

	if rows.Err() != nil {
		return []trillian.LogID{}, rows.Err()
	}

	return logIDs, nil
}

// GetActiveLogIDs returns a list of the IDs of all configured logs
func (t *logTX) GetActiveLogIDs() ([]trillian.LogID, error) {
	return t.getActiveLogIDsInternal(selectActiveLogsSql)
}

// GetActiveLogIDsWithPendingWork returns a list of the IDs of all configured logs
// that have queued unsequenced leaves that need to be integrated
func (t *logTX) GetActiveLogIDsWithPendingWork() ([]trillian.LogID, error) {
	return t.getActiveLogIDsInternal(selectActiveLogsWithUnsequencedSql)
}
