package mysql

import (
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
const insertNodeSql string = `INSERT INTO Node(TreeId, NodeId, NodeHash, NodeRevision) VALUES (?, ?, ?, ?)`
const insertTreeHeadSql string = `INSERT INTO TreeHead(TreeId,TreeHeadTimestamp,TreeSize,RootHash,TreeRevision,RootSignature)
		 VALUES(?,?,?,?,?,?)`
const selectTreeRevisionAtSizeSql string = "SELECT TreeRevision FROM TreeHead WHERE TreeId=? AND TreeSize=? ORDER BY TreeRevision DESC LIMIT 1"
const selectActiveLogsSql string = "select TreeId, KeyId from Trees where TreeType='LOG'"
const selectActiveLogsWithUnsequencedSql string = "SELECT DISTINCT t.TreeId, t.KeyId from Trees t INNER JOIN Unsequenced u WHERE TreeType='LOG' AND t.TreeId=u.TreeId"

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

const placeholderSql string = "<placeholder>"

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

type treeTX struct {
	closed bool
	tx     *sql.Tx
	ts     *mySQLTreeStorage
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

func (t *treeTX) IsOpen() bool {
	return !t.closed
}

func (t* treeTX) getActiveLogIDsInternal(sql string) ([]trillian.LogID, error) {
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

// GetActiveLogIDs returns the IDs of all configured logs, possibly with filtering
func (t *treeTX) GetActiveLogIDs(filterPendingWorkOnly bool) ([]trillian.LogID, error) {
	if (filterPendingWorkOnly) {
		return t.getActiveLogIDsInternal(selectActiveLogsWithUnsequencedSql)
	}

	return t.getActiveLogIDsInternal(selectActiveLogsSql)
}

