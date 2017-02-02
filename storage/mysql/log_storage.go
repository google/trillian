package mysql

import (
	"context"
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

const (
	getTreePropertiesSQL  = "SELECT AllowsDuplicateLeaves FROM Trees WHERE TreeId=?"
	getTreeParametersSQL  = "SELECT ReadOnlyRequests From TreeControl WHERE TreeID=?"
	selectQueuedLeavesSQL = `SELECT LeafIdentityHash,MerkleLeafHash,Payload
			FROM Unsequenced
			WHERE TreeID=?
			AND QueueTimestampNanos<=?
			ORDER BY QueueTimestampNanos,LeafIdentityHash ASC LIMIT ?`
	insertUnsequencedLeafSQL = `INSERT INTO LeafData(TreeId,LeafIdentityHash,LeafValue,ExtraData)
			VALUES(?,?,?,?) ON DUPLICATE KEY UPDATE LeafIdentityHash=LeafIdentityHash`
	insertUnsequencedLeafSQLNoDuplicates = `INSERT INTO LeafData(TreeId,LeafIdentityHash,LeafValue,ExtraData)
			VALUES(?,?,?,?)`
	insertUnsequencedEntrySQL = `INSERT INTO Unsequenced(TreeId,LeafIdentityHash,MerkleLeafHash,MessageId,Payload,QueueTimestampNanos)
			VALUES(?,?,?,?,?,?)`
	insertSequencedLeafSQL = `INSERT INTO SequencedLeafData(TreeId,LeafIdentityHash,MerkleLeafHash,SequenceNumber)
			VALUES(?,?,?,?)`
	selectSequencedLeafCountSQL  = "SELECT COUNT(*) FROM SequencedLeafData WHERE TreeId=?"
	selectLatestSignedLogRootSQL = `SELECT TreeHeadTimestamp,TreeSize,RootHash,TreeRevision,RootSignature
			FROM TreeHead WHERE TreeId=?
			ORDER BY TreeHeadTimestamp DESC LIMIT 1`

	// These statements need to be expanded to provide the correct number of parameter placeholders.
	deleteUnsequencedSQL   = "DELETE FROM Unsequenced WHERE LeafIdentityHash IN (<placeholder>) AND TreeId = ?"
	selectLeavesByIndexSQL = `SELECT s.MerkleLeafHash,l.LeafIdentityHash,l.LeafValue,s.SequenceNumber,l.ExtraData
			FROM LeafData l,SequencedLeafData s
			WHERE l.LeafIdentityHash = s.LeafIdentityHash
			AND s.SequenceNumber IN (` + placeholderSQL + `) AND l.TreeId = ? AND s.TreeId = l.TreeId`
	selectLeavesByMerkleHashSQL = `SELECT s.MerkleLeafHash,l.LeafIdentityHash,l.LeafValue,s.SequenceNumber,l.ExtraData
			FROM LeafData l,SequencedLeafData s
			WHERE l.LeafIdentityHash = s.LeafIdentityHash
			AND s.MerkleLeafHash IN (` + placeholderSQL + `) AND l.TreeId = ? AND s.TreeId = l.TreeId`

	// Same as above except with leaves ordered by sequence so we only incur this cost when necessary
	orderBySequenceNumberSQL                     = " ORDER BY s.SequenceNumber"
	selectLeavesByMerkleHashOrderedBySequenceSQL = selectLeavesByMerkleHashSQL + orderBySequenceNumberSQL
)

var defaultLogStrata = []int{8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8}

type mySQLLogStorage struct {
	*mySQLTreeStorage
}

// NewLogStorage creates a mySQLLogStorage instance for the specified MySQL URL.
func NewLogStorage(db *sql.DB) (storage.LogStorage, error) {
	return &mySQLLogStorage{
		mySQLTreeStorage: newTreeStorage(db),
	}, nil
}

func (m *mySQLLogStorage) getLeavesByIndexStmt(num int) (*sql.Stmt, error) {
	return m.getStmt(selectLeavesByIndexSQL, num, "?", "?")
}

func (m *mySQLLogStorage) getLeavesByMerkleHashStmt(num int, orderBySequence bool) (*sql.Stmt, error) {
	if orderBySequence {
		return m.getStmt(selectLeavesByMerkleHashOrderedBySequenceSQL, num, "?", "?")
	}

	return m.getStmt(selectLeavesByMerkleHashSQL, num, "?", "?")
}

func (m *mySQLLogStorage) getDeleteUnsequencedStmt(num int) (*sql.Stmt, error) {
	return m.getStmt(deleteUnsequencedSQL, num, "?", "?")
}

func getActiveLogIDsInternal(tx *sql.Tx, sql string) ([]int64, error) {
	rows, err := tx.Query(sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logIDs := make([]int64, 0)
	for rows.Next() {
		var keyID []byte
		var treeID int64
		if err := rows.Scan(&treeID, &keyID); err != nil {
			return nil, err
		}
		logIDs = append(logIDs, treeID)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return logIDs, nil
}

func getActiveLogIDs(tx *sql.Tx) ([]int64, error) {
	return getActiveLogIDsInternal(tx, selectActiveLogsSQL)
}

func getActiveLogIDsWithPendingWork(tx *sql.Tx) ([]int64, error) {
	return getActiveLogIDsInternal(tx, selectActiveLogsWithUnsequencedSQL)
}

// readOnlyLogTX implements storage.ReadOnlyLogTX
type readOnlyLogTX struct {
	tx *sql.Tx
}

func (m *mySQLLogStorage) Snapshot(ctx context.Context) (storage.ReadOnlyLogTX, error) {
	tx, err := m.db.Begin()
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

func (t *readOnlyLogTX) GetActiveLogIDs() ([]int64, error) {
	return getActiveLogIDs(t.tx)
}

func (t *readOnlyLogTX) GetActiveLogIDsWithPendingWork() ([]int64, error) {
	return getActiveLogIDsWithPendingWork(t.tx)
}

func (m *mySQLLogStorage) beginInternal(ctx context.Context, treeID int64) (storage.LogTreeTX, error) {
	// TODO(codingllama): Validate treeType, read hash algorithm from storage
	var allowDuplicates bool
	if err := m.db.QueryRow(getTreePropertiesSQL, treeID).Scan(&allowDuplicates); err != nil {
		return nil, fmt.Errorf("failed to get tree row for treeID %v: %s", treeID, err)
	}
	th := merkle.NewRFC6962TreeHasher(crypto.NewSHA256())

	ttx, err := m.beginTreeTx(ctx, treeID, th.Size(), defaultLogStrata, cache.PopulateLogSubtreeNodes(th))
	if err != nil {
		return nil, err
	}

	ltx := &logTreeTX{
		treeTX:          ttx,
		ls:              m,
		allowDuplicates: allowDuplicates,
	}

	root, err := ltx.LatestSignedLogRoot()
	if err != nil {
		ttx.Rollback()
		return nil, err
	}
	ltx.treeTX.writeRevision = root.TreeRevision + 1

	return ltx, nil
}

func (m *mySQLLogStorage) BeginForTree(ctx context.Context, treeID int64) (storage.LogTreeTX, error) {
	return m.beginInternal(ctx, treeID)
}

func (m *mySQLLogStorage) SnapshotForTree(ctx context.Context, treeID int64) (storage.ReadOnlyLogTreeTX, error) {
	tx, err := m.beginInternal(ctx, treeID)
	if err != nil {
		return nil, err
	}
	return tx.(storage.ReadOnlyLogTreeTX), err
}

type logTreeTX struct {
	treeTX
	ls              *mySQLLogStorage
	allowDuplicates bool
}

func (t *logTreeTX) WriteRevision() int64 {
	return t.treeTX.writeRevision
}

func (t *logTreeTX) DequeueLeaves(limit int, cutoffTime time.Time) ([]trillian.LogLeaf, error) {
	stx, err := t.tx.Prepare(selectQueuedLeavesSQL)

	if err != nil {
		glog.Warningf("Failed to prepare dequeue select: %s", err)
		return nil, err
	}

	leaves := make([]trillian.LogLeaf, 0, limit)
	rows, err := stx.Query(t.treeID, cutoffTime.UnixNano(), limit)

	if err != nil {
		glog.Warningf("Failed to select rows for work: %s", err)
		return nil, err
	}

	defer rows.Close()

	for rows.Next() {
		var leafIDHash []byte
		var merkleHash []byte
		var payload []byte

		err := rows.Scan(&leafIDHash, &merkleHash, &payload)

		if err != nil {
			glog.Warningf("Error scanning work rows: %s", err)
			return nil, err
		}

		if len(leafIDHash) != t.hashSizeBytes {
			return nil, errors.New("Dequeued a leaf with incorrect hash size")
		}

		// Note: the ExtraData being nil here is OK as the sequencer only writes to the
		// SequencedLeafData table and the client supplied value is already written to LeafData.
		leaf := trillian.LogLeaf{
			LeafIdentityHash: leafIDHash,
			MerkleLeafHash:   merkleHash,
			LeafValue:        payload,
			ExtraData:        nil,
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

func (t *logTreeTX) QueueLeaves(leaves []trillian.LogLeaf, queueTimestamp time.Time) error {
	// Don't accept batches if any of the leaves are invalid.
	for _, leaf := range leaves {
		if len(leaf.LeafIdentityHash) != t.hashSizeBytes {
			return fmt.Errorf("queued leaf must have a leaf ID hash of length %d", t.hashSizeBytes)
		}
	}

	// If the log does not allow duplicates we prevent the insert of such a leaf from
	// succeeding. If duplicates are allowed multiple sequenced leaves will share the same
	// leaf data in the database.
	var insertSQL string

	if t.allowDuplicates {
		insertSQL = insertUnsequencedLeafSQL
	} else {
		insertSQL = insertUnsequencedLeafSQLNoDuplicates
	}

	for i, leaf := range leaves {
		// Create the unsequenced leaf data entry. We don't use INSERT IGNORE because this
		// can suppress errors unrelated to key collisions. We don't use REPLACE because
		// if there's ever a hash collision it will do the wrong thing and it also
		// causes a DELETE / INSERT, which is undesirable.
		_, err := t.tx.Exec(insertSQL, t.treeID, leaf.LeafIdentityHash, leaf.LeafValue, leaf.ExtraData)

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

		if t.allowDuplicates {
			_, err := rand.Read(messageIDBytes)

			if err != nil {
				glog.Warningf("Failed to get a random message id: %s", err)
				return err
			}
		}

		hasher.Write(messageIDBytes)
		binary.Write(hasher, binary.LittleEndian, t.treeID)
		hasher.Write(leaf.LeafIdentityHash)
		messageID := hasher.Sum(nil)

		_, err = t.tx.Exec(insertUnsequencedEntrySQL,
			t.treeID, leaf.LeafIdentityHash, leaf.MerkleLeafHash, messageID, leaf.LeafValue, queueTimestamp.UnixNano())

		if err != nil {
			glog.Warningf("Error inserting into Unsequenced: %s", err)
			return fmt.Errorf("Unsequenced: %v", err)
		}
	}

	return nil
}

func (t *logTreeTX) GetSequencedLeafCount() (int64, error) {
	var sequencedLeafCount int64

	err := t.tx.QueryRow(selectSequencedLeafCountSQL, t.treeID).Scan(&sequencedLeafCount)

	if err != nil {
		glog.Warningf("Error getting sequenced leaf count: %s", err)
	}

	return sequencedLeafCount, err
}

func (t *logTreeTX) GetLeavesByIndex(leaves []int64) ([]trillian.LogLeaf, error) {
	tmpl, err := t.ls.getLeavesByIndexStmt(len(leaves))
	if err != nil {
		return nil, err
	}
	stx := t.tx.Stmt(tmpl)
	var args []interface{}
	for _, nodeID := range leaves {
		args = append(args, interface{}(int64(nodeID)))
	}
	args = append(args, interface{}(t.treeID))
	rows, err := stx.Query(args...)
	if err != nil {
		glog.Warningf("Failed to get leaves by idx: %s", err)
		return nil, err
	}

	ret := make([]trillian.LogLeaf, len(leaves))
	num := 0

	defer rows.Close()
	for rows.Next() {
		if err := rows.Scan(&ret[num].MerkleLeafHash, &ret[num].LeafIdentityHash, &ret[num].LeafValue, &ret[num].LeafIndex, &ret[num].ExtraData); err != nil {
			glog.Warningf("Failed to scan merkle leaves: %s", err)
			return nil, err
		}

		if got, want := len(ret[num].MerkleLeafHash), t.hashSizeBytes; got != want {
			return nil, fmt.Errorf("scanned leaf does not have hash length %d, got %d", want, got)
		}

		num++
	}

	if num != len(leaves) {
		return nil, fmt.Errorf("expected %d leaves, but saw %d", len(leaves), num)
	}
	return ret, nil
}

func (t *logTreeTX) GetLeavesByHash(leafHashes [][]byte, orderBySequence bool) ([]trillian.LogLeaf, error) {
	tmpl, err := t.ls.getLeavesByMerkleHashStmt(len(leafHashes), orderBySequence)

	if err != nil {
		return nil, err
	}

	return t.getLeavesByHashInternal(leafHashes, tmpl, "merkle")
}

func (t *logTreeTX) LatestSignedLogRoot() (trillian.SignedLogRoot, error) {
	var timestamp, treeSize, treeRevision int64
	var rootHash, rootSignatureBytes []byte
	var rootSignature trillian.DigitallySigned

	err := t.tx.QueryRow(
		selectLatestSignedLogRootSQL, t.treeID).Scan(
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

func (t *logTreeTX) StoreSignedLogRoot(root trillian.SignedLogRoot) error {
	signatureBytes, err := proto.Marshal(root.Signature)

	if err != nil {
		glog.Warningf("Failed to marshal root signature: %v %v", root.Signature, err)
		return err
	}

	res, err := t.tx.Exec(insertTreeHeadSQL, t.treeID, root.TimestampNanos, root.TreeSize,
		root.RootHash, root.TreeRevision, signatureBytes)

	if err != nil {
		glog.Warningf("Failed to store signed root: %s", err)
	}

	return checkResultOkAndRowCountIs(res, err, 1)
}

func (t *logTreeTX) UpdateSequencedLeaves(leaves []trillian.LogLeaf) error {
	// TODO: In theory we can do this with CASE / WHEN in one SQL statement but it's more fiddly
	// and can be implemented later if necessary
	for _, leaf := range leaves {
		// This should fail on insert but catch it early
		if len(leaf.LeafIdentityHash) != t.hashSizeBytes {
			return errors.New("Sequenced leaf has incorrect hash size")
		}

		_, err := t.tx.Exec(insertSequencedLeafSQL, t.treeID, leaf.LeafIdentityHash, leaf.MerkleLeafHash,
			leaf.LeafIndex)

		if err != nil {
			glog.Warningf("Failed to update sequenced leaves: %s", err)
			return err
		}
	}

	return nil
}

func (t *logTreeTX) removeSequencedLeaves(leaves []trillian.LogLeaf) error {
	tmpl, err := t.ls.getDeleteUnsequencedStmt(len(leaves))
	if err != nil {
		glog.Warningf("Failed to get delete statement for sequenced work: %s", err)
		return err
	}
	stx := t.tx.Stmt(tmpl)
	var args []interface{}
	for _, leaf := range leaves {
		args = append(args, interface{}(leaf.LeafIdentityHash))
	}
	args = append(args, interface{}(t.treeID))
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

func (t *logTreeTX) getLeavesByHashInternal(leafHashes [][]byte, tmpl *sql.Stmt, desc string) ([]trillian.LogLeaf, error) {
	stx := t.tx.Stmt(tmpl)
	var args []interface{}
	for _, hash := range leafHashes {
		args = append(args, interface{}([]byte(hash)))
	}
	args = append(args, interface{}(t.treeID))
	rows, err := stx.Query(args...)
	if err != nil {
		glog.Warningf("Query() %s hash = %v", desc, err)
		return nil, err
	}

	// The tree could include duplicates so we don't know how many results will be returned
	var ret []trillian.LogLeaf

	defer rows.Close()
	for rows.Next() {
		leaf := trillian.LogLeaf{}

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
func (t *logTreeTX) GetActiveLogIDs() ([]int64, error) {
	return getActiveLogIDs(t.tx)
}

// GetActiveLogIDsWithPendingWork returns a list of the IDs of all configured logs
// that have queued unsequenced leaves that need to be integrated
func (t *logTreeTX) GetActiveLogIDsWithPendingWork() ([]int64, error) {
	return getActiveLogIDsWithPendingWork(t.tx)
}
