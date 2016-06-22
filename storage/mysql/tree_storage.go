package mysql

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
)

// These statements are fixed
const getLogPropertiesSql string = "SELECT AllowsDuplicateLeaves FROM Trees WHERE TreeId=?"
const insertNodeSql string = `INSERT INTO Node(TreeId, NodeId, NodeHash, NodeRevision) VALUES (?, ?, ?, ?)`
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
const selectLatestSignedRootSql string = `SELECT TreeHeadTimestamp,TreeSize,RootHash,TreeRevision,RootSignature
		 FROM TreeHead WHERE TreeId=?
		 ORDER BY TreeHeadTimestamp DESC LIMIT 1`
const insertTreeHeadSql string = `INSERT INTO TreeHead(TreeId,TreeHeadTimestamp,TreeSize,RootHash,TreeRevision,RootSignature)
		 VALUES(?,?,?,?,?,?)`
const selectTreeRevisionAtSizeSql string = "SELECT TreeRevision FROM TreeHead WHERE TreeId=? AND TreeSize=? ORDER BY TreeRevision DESC LIMIT 1"

const placeholderSql string = "<placeholder>"

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
const selectNodesSql string = `SELECT x.NodeId, x.MaxRevision, Node.NodeHash
				 FROM (SELECT n.NodeId, max(n.NodeRevision) AS MaxRevision
							 FROM Node n
							 WHERE n.NodeId IN (` + placeholderSql + `) AND
										 n.TreeId = ? AND
										 n.NodeRevision >= -?
							 GROUP BY n.NodeId) AS x
				 INNER JOIN Node ON Node.NodeId = x.NodeId AND
														Node.NodeRevision = x.MaxRevision AND
														Node.TreeId = ?`

// mySQLTreeStorage is shared between the mySQLLog- and (forthcoming) mySQLMap-
// Storage implementations, and contains functionality which is common to both,
type mySQLTreeStorage struct {
	treeID        int64
	db            *sql.DB
	hashSizeBytes int

	// Must hold the mutex before manipulating the statement map. Sharing a lock because
	// it only needs to be held while the statements are built, not while they execute and
	// this will be a short time. These maps are from the number of placeholder '?'
	// in the query to the statement that should be used.
	statementMutex sync.Mutex
	statements     map[string]map[int]*sql.Stmt
	setNode        *sql.Stmt
}

type mySQLLogStorage struct {
	mySQLTreeStorage

	logID           trillian.LogID
	allowDuplicates bool
}

type mySQLMapStorage struct {
	mySQLTreeStorage

	mapID trillian.MapID
}

func openDB(dbURL string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dbURL)
	if err != nil {
		// Don't log uri as it could contain credentials
		glog.Warningf("Could not open MySQL database, check config: %s", err)
		return nil, err
	}

	if _, err := db.Exec("SET sql_mode = 'STRICT_ALL_TABLES'"); err != nil {
		glog.Warningf("Failed to set strict mode on mysql db: %s", err)
		return nil, err
	}

	return db, nil
}

func newTreeStorage(treeID int64, dbURL string, hasher trillian.Hasher) (mySQLTreeStorage, error) {
	db, err := openDB(dbURL)
	if err != nil {
		return mySQLTreeStorage{}, err
	}

	s := mySQLTreeStorage{
		treeID:        treeID,
		db:            db,
		hashSizeBytes: hasher.Size(),
		statements:    make(map[string]map[int]*sql.Stmt),
	}

	if s.setNode, err = s.db.Prepare(insertNodeSql); err != nil {
		glog.Warningf("Failed to prepare node insert statement: %s", err)
		return mySQLTreeStorage{}, err
	}

	return s, nil
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
	if err := s.db.QueryRow(getLogPropertiesSql, id.TreeID).Scan(&s.allowDuplicates); err == sql.ErrNoRows {
		s.allowDuplicates = false
	} else if err != nil {
		glog.Warningf("Failed to get trees row for id %v: %s", id, err)
		return nil, err
	}

	if s.setNode, err = s.db.Prepare(insertNodeSql); err != nil {
		glog.Warningf("Failed to prepare node insert statement: %s", err)
		return nil, err
	}

	return &s, nil
}

func NewMapStorage(id trillian.MapID, dbURL string) (storage.MapStorage, error) {
	ts, err := newTreeStorage(id.TreeID, dbURL, trillian.NewSHA256())
	if err != nil {
		glog.Warningf("Couldn't create a new treeStorage: %s", err)
		return nil, err
	}

	s := mySQLMapStorage{
		mySQLTreeStorage: ts,
		mapID:            id,
	}

	if err != nil {
		glog.Warningf("Couldn't create a new treeStorage: %s", err)
		return nil, err
	}

	return &s, nil
}

// expandPlaceholderSql expands an sql statement by adding a specified number of '?'
// placeholder slots. At most one placeholder will be expanded.
func expandPlaceholderSql(sql string, num int) string {
	if num <= 0 {
		panic(fmt.Errorf("Trying to expand SQL placeholder with <= 0 parameters: %s", sql))
	}

	parameters := "?" + strings.Repeat(",?", num-1)

	return strings.Replace(sql, placeholderSql, parameters, 1)
}

func decodeSignedTimestamp(signedEntryTimestampBytes []byte) (trillian.SignedEntryTimestamp, error) {
	var signedEntryTimestamp trillian.SignedEntryTimestamp

	if err := proto.Unmarshal(signedEntryTimestampBytes, &signedEntryTimestamp); err != nil {
		glog.Warningf("Failed to decode SignedTimestamp: %s", err)
		return trillian.SignedEntryTimestamp{}, err
	}

	return signedEntryTimestamp, nil
}

// TODO: Pull the encoding / decoding out of this file, move up to Storage. Review after
// all current PRs submitted.
func EncodeSignedTimestamp(signedEntryTimestamp trillian.SignedEntryTimestamp) ([]byte, error) {
	marshalled, err := proto.Marshal(&signedEntryTimestamp)

	if err != nil {
		glog.Warningf("Failed to encode SignedTimestamp: %s", err)
		return nil, err
	}

	return marshalled, err
}

// Node IDs are stored using proto serialization
func decodeNodeID(nodeIDBytes []byte) (*storage.NodeID, error) {
	var nodeIdProto storage.NodeIDProto

	if err := proto.Unmarshal(nodeIDBytes, &nodeIdProto); err != nil {
		glog.Warningf("Failed to decode nodeid: %s", err)
		return nil, err
	}

	return storage.NewNodeIDFromProto(nodeIdProto), nil
}

func encodeNodeID(n storage.NodeID) ([]byte, error) {
	nodeIdProto := n.AsProto()
	marshalledBytes, err := proto.Marshal(nodeIdProto)

	if err != nil {
		glog.Warningf("Failed to encode nodeid: %s", err)
		return nil, err
	}

	return marshalledBytes, nil
}

// getStmt creates and caches sql.Stmt structs based on the passed in statement
// and number of bound arguments.
// TODO(al,martin): consider pulling this all out as a separate unit for reuse
// elsewhere.
func (m *mySQLTreeStorage) getStmt(statement string, num int) (*sql.Stmt, error) {
	m.statementMutex.Lock()
	defer m.statementMutex.Unlock()

	if m.statements[statement] != nil {
		if m.statements[statement][num] != nil {
			// TODO(al,martin): we'll possibly need to expire Stmts from the cache,
			// e.g. when DB connections break etc.
			return m.statements[statement][num], nil
		}
	} else {
		m.statements[statement] = make(map[int]*sql.Stmt)
	}

	s, err := m.db.Prepare(expandPlaceholderSql(statement, num))

	if err != nil {
		glog.Warningf("Failed to prepare statement %d: %s", num, err)
		return nil, err
	}

	m.statements[statement][num] = s

	return s, nil
}

func (m *mySQLTreeStorage) getNodesStmt(num int) (*sql.Stmt, error) {
	return m.getStmt(selectNodesSql, num)
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

func (m *mySQLTreeStorage) beginTreeTx() (treeTX, error) {
	t, err := m.db.Begin()
	if err != nil {
		glog.Warningf("Could not start tree TX: %s", err)
		return treeTX{}, err
	}
	return treeTX{
		tx: t,
		ts: m,
	}, nil
}

func (m *mySQLLogStorage) LatestSignedLogRoot() (trillian.SignedLogRoot, error) {
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

func (m *mySQLLogStorage) Begin() (storage.LogTX, error) {
	ttx, err := m.beginTreeTx()
	if err != nil {
		return nil, err
	}
	return &logTX{
		treeTX: ttx,
		ls:     m,
	}, nil
}

func (m *mySQLLogStorage) Snapshot() (storage.ReadOnlyLogTX, error) {
	tx, err := m.Begin()
	if err != nil {
		return nil, err
	}
	return tx.(storage.ReadOnlyLogTX), err
}

func (m *mySQLMapStorage) Begin() (storage.MapTX, error) {
	ttx, err := m.beginTreeTx()
	if err != nil {
		return nil, err
	}
	return &mapTX{
		treeTX: ttx,
		ms:     m,
	}, nil
}

func (m *mySQLMapStorage) Snapshot() (storage.ReadOnlyMapTX, error) {
	tx, err := m.Begin()
	if err != nil {
		return nil, err
	}
	return tx.(storage.ReadOnlyMapTX), err
}

type treeTX struct {
	closed bool
	tx     *sql.Tx
	ts     *mySQLTreeStorage
}

type logTX struct {
	treeTX
	ls *mySQLLogStorage
}

type mapTX struct {
	treeTX
	ms *mySQLMapStorage
}

func checkResultOkAndRowCountIs(res sql.Result, err error, count int64) error {
	// The Exec() might have just failed
	if err != nil {
		return err
	}

	// Otherwise we have to look at the result of the operation
	rowsAffected, rowsError := res.RowsAffected()

	if rowsError != nil {
		return rowsError
	}

	if rowsAffected != count {
		return errors.New(fmt.Sprintf("Expected %d row(s) to be affected but saw: %d", count,
			rowsAffected))
	}

	return nil
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
		selectLatestSignedRootSql, t.ls.logID.TreeID).Scan(
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
		TimestampNanos: proto.Int64(timestamp),
		TreeRevision:   proto.Int64(treeRevision),
		Signature:      &rootSignature,
		LogId:          t.ls.logID.LogID,
		TreeSize:       proto.Int64(treeSize),
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

// GetTreeRevisionAtSize returns the max node version for a tree at a particular size.
// It is an error to request tree sizes larger than the currently published tree size.
// TODO: This only works for sizes where there is a stored tree head. This is deliberate atm
// as serving proofs at intermediate tree sizes is complicated and will be implemented later.
func (t *treeTX) GetTreeRevisionAtSize(treeSize int64) (int64, error) {
	// Negative size is not sensible and a zero sized tree has no nodes so no revisions
	if treeSize <= 0 {
		return 0, fmt.Errorf("Invalid tree size: %d", treeSize)
	}

	var treeRevision int64
	err := t.tx.QueryRow(selectTreeRevisionAtSizeSql, t.ts.treeID, treeSize).Scan(&treeRevision)

	return treeRevision, err
}

func (t *treeTX) GetMerkleNodes(treeRevision int64, nodeIDs []storage.NodeID) ([]storage.Node, error) {
	tmpl, err := t.ts.getNodesStmt(len(nodeIDs))
	if err != nil {
		return nil, err
	}
	stx := t.tx.Stmt(tmpl)
	args := make([]interface{}, 0)
	for _, nodeID := range nodeIDs {
		nodeIdBytes, err := encodeNodeID(nodeID)

		if err != nil {
			return nil, err
		}

		args = append(args, interface{}(nodeIdBytes))
	}
	args = append(args, interface{}(t.ts.treeID))
	args = append(args, interface{}(treeRevision))
	args = append(args, interface{}(t.ts.treeID))
	rows, err := stx.Query(args...)
	if err != nil {
		glog.Warningf("Failed to get merkle nodes: %s", err)
		return nil, err
	}

	ret := make([]storage.Node, len(nodeIDs))
	num := 0

	defer rows.Close()
	for rows.Next() {
		var nodeIDBytes []byte
		if err := rows.Scan(&nodeIDBytes, &ret[num].NodeRevision, &ret[num].Hash); err != nil {
			glog.Warningf("Failed to scan merkle nodes: %s", err)
			return nil, err
		}

		if got, want := len(ret[num].Hash), t.ts.hashSizeBytes; got != want {
			return nil, fmt.Errorf("Scanned node does not have hash length %d, got %d", want, got)
		}

		nodeID, err := decodeNodeID(nodeIDBytes)

		if err != nil {
			glog.Warningf("Failed to decode nodeid: %s", err)
			return nil, err
		}

		ret[num].NodeID = *nodeID

		num++
	}

	if num != len(nodeIDs) {
		return nil, fmt.Errorf("expected %d nodes, but saw %d", len(nodeIDs), num)
	}
	return ret, nil
}

func (t *treeTX) SetMerkleNodes(treeRevision int64, nodes []storage.Node) error {
	stx := t.tx.Stmt(t.ts.setNode)
	for _, n := range nodes {
		nodeIdBytes, err := encodeNodeID(n.NodeID)

		if err != nil {
			return err
		}

		_, err = stx.Exec(t.ts.treeID, nodeIdBytes, []byte(n.Hash), treeRevision)
		if err != nil {
			glog.Warningf("Failed to set merkle nodes: %s", err)
			return err
		}
	}
	return nil
}

func (t *treeTX) Commit() error {
	t.closed = true
	err := t.tx.Commit()

	if err != nil {
		glog.Warningf("TX commit error: %$s", err)
	}

	return err
}

func (t *treeTX) Rollback() error {
	t.closed = true
	err := t.tx.Rollback()

	if err != nil {
		glog.Warningf("TX rollback error: %s", err)
	}

	return err
}

func (m *mapTX) Set(key []byte, value trillian.MapLeaf) error {
	return errors.New("unimplemented")
}

func (m *mapTX) Get(revision int64, key []byte) (trillian.MapLeaf, error) {
	return trillian.MapLeaf{}, errors.New("unimplemented")
}

func (m *mapTX) LatestSignedMapRoot() (trillian.SignedMapRoot, error) {
	return trillian.SignedMapRoot{}, errors.New("unimplemented")
}

func (m *mapTX) StoreSignedMapRoot(root trillian.SignedMapRoot) error {
	return errors.New("unimplemented")
}

func (t *treeTX) IsOpen() bool {
	return !t.closed
}
