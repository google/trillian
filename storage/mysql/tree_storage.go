package mysql

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/cache"
	"github.com/google/trillian/storage/storagepb"
)

// These statements are fixed
const (
	insertSubtreeMultiSQL = `INSERT INTO Subtree(TreeId, SubtreeId, Nodes, SubtreeRevision) ` + placeholderSQL
	insertTreeHeadSQL     = `INSERT INTO TreeHead(TreeId,TreeHeadTimestamp,TreeSize,RootHash,TreeRevision,RootSignature)
		 VALUES(?,?,?,?,?,?)`
	selectTreeRevisionAtSizeOrLargerSQL = "SELECT TreeRevision,TreeSize FROM TreeHead WHERE TreeId=? AND TreeSize>=? ORDER BY TreeRevision LIMIT 1"
	selectActiveLogsSQL                 = "SELECT TreeId, KeyId from Trees where TreeType='LOG'"
	selectActiveLogsWithUnsequencedSQL  = "SELECT DISTINCT t.TreeId, t.KeyId from Trees t INNER JOIN Unsequenced u WHERE TreeType='LOG' AND t.TreeId=u.TreeId"

	selectSubtreeSQL = `
 SELECT x.SubtreeId, x.MaxRevision, Subtree.Nodes
 FROM (
 	SELECT n.SubtreeId, max(n.SubtreeRevision) AS MaxRevision
	FROM Subtree n
	WHERE n.SubtreeId IN (` + placeholderSQL + `) AND
	 n.TreeId = ? AND n.SubtreeRevision <= ?
	GROUP BY n.SubtreeId
 ) AS x
 INNER JOIN Subtree 
 ON Subtree.SubtreeId = x.SubtreeId 
 AND Subtree.SubtreeRevision = x.MaxRevision 
 AND Subtree.TreeId = ?`
	placeholderSQL = "<placeholder>"
)

// mySQLTreeStorage is shared between the mySQLLog- and (forthcoming) mySQLMap-
// Storage implementations, and contains functionality which is common to both,
type mySQLTreeStorage struct {
	treeID          int64
	db              *sql.DB
	hashSizeBytes   int
	populateSubtree storage.PopulateSubtreeFunc

	// Must hold the mutex before manipulating the statement map. Sharing a lock because
	// it only needs to be held while the statements are built, not while they execute and
	// this will be a short time. These maps are from the number of placeholder '?'
	// in the query to the statement that should be used.
	statementMutex sync.Mutex
	statements     map[string]map[int]*sql.Stmt
	strataDepths   []int
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

func newTreeStorage(treeID int64, dbURL string, hashSizeBytes int, strataDepths []int, populateSubtree storage.PopulateSubtreeFunc) (*mySQLTreeStorage, error) {
	db, err := openDB(dbURL)
	if err != nil {
		return &mySQLTreeStorage{}, err
	}

	s := mySQLTreeStorage{
		treeID:          treeID,
		db:              db,
		hashSizeBytes:   hashSizeBytes,
		populateSubtree: populateSubtree,
		statements:      make(map[string]map[int]*sql.Stmt),
		strataDepths:    strataDepths,
	}

	return &s, nil
}

// expandPlaceholderSQL expands an sql statement by adding a specified number of '?'
// placeholder slots. At most one placeholder will be expanded.
func expandPlaceholderSQL(sql string, num int, first, rest string) string {
	if num <= 0 {
		panic(fmt.Errorf("Trying to expand SQL placeholder with <= 0 parameters: %s", sql))
	}

	parameters := first + strings.Repeat(","+rest, num-1)

	return strings.Replace(sql, placeholderSQL, parameters, 1)
}

// Node IDs are stored using proto serialization
func decodeNodeID(nodeIDBytes []byte) (*storage.NodeID, error) {
	var nodeIDProto storagepb.NodeIDProto

	if err := proto.Unmarshal(nodeIDBytes, &nodeIDProto); err != nil {
		glog.Warningf("Failed to decode nodeid: %s", err)
		return nil, err
	}

	return storage.NewNodeIDFromProto(nodeIDProto), nil
}

func encodeNodeID(n storage.NodeID) ([]byte, error) {
	nodeIDProto := n.AsProto()
	marshalledBytes, err := proto.Marshal(nodeIDProto)

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
func (m *mySQLTreeStorage) getStmt(statement string, num int, first, rest string) (*sql.Stmt, error) {
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

	s, err := m.db.Prepare(expandPlaceholderSQL(statement, num, first, rest))

	if err != nil {
		glog.Warningf("Failed to prepare statement %d: %s", num, err)
		return nil, err
	}

	m.statements[statement][num] = s

	return s, nil
}

func (m *mySQLTreeStorage) getSubtreeStmt(num int) (*sql.Stmt, error) {
	return m.getStmt(selectSubtreeSQL, num, "?", "?")
}

func (m *mySQLTreeStorage) setSubtreeStmt(num int) (*sql.Stmt, error) {
	return m.getStmt(insertSubtreeMultiSQL, num, "VALUES(?, ?, ?, ?)", "(?, ?, ?, ?)")
}

func (m *mySQLTreeStorage) beginTreeTx() (treeTX, error) {
	t, err := m.db.Begin()
	if err != nil {
		glog.Warningf("Could not start tree TX: %s", err)
		return treeTX{}, err
	}
	return treeTX{
		tx:            t,
		ts:            m,
		subtreeCache:  cache.NewSubtreeCache(m.strataDepths, m.populateSubtree),
		writeRevision: -1,
	}, nil
}

type treeTX struct {
	closed        bool
	tx            *sql.Tx
	ts            *mySQLTreeStorage
	subtreeCache  cache.SubtreeCache
	writeRevision int64
}

func (t *treeTX) getSubtree(treeRevision int64, nodeID storage.NodeID) (*storagepb.SubtreeProto, error) {
	s, err := t.getSubtrees(treeRevision, []storage.NodeID{nodeID})
	if err != nil {
		return nil, err
	}
	switch len(s) {
	case 0:
		return nil, nil
	case 1:
		return s[0], nil
	default:
		return nil, fmt.Errorf("got %d subtrees, but expected 1", len(s))
	}
}

func (t *treeTX) getSubtrees(treeRevision int64, nodeIDs []storage.NodeID) ([]*storagepb.SubtreeProto, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}

	tmpl, err := t.ts.getSubtreeStmt(len(nodeIDs))
	if err != nil {
		return nil, err
	}
	stx := t.tx.Stmt(tmpl)
	defer stx.Close()

	args := make([]interface{}, 0, len(nodeIDs)+3)

	// populate args with nodeIDs
	for _, nodeID := range nodeIDs {
		if nodeID.PrefixLenBits%8 != 0 {
			return nil, fmt.Errorf("invalid subtree ID - not multiple of 8: %d", nodeID.PrefixLenBits)
		}

		nodeIDBytes := nodeID.Path[:nodeID.PrefixLenBits/8]

		args = append(args, interface{}(nodeIDBytes))
	}

	args = append(args, interface{}(t.ts.treeID))
	args = append(args, interface{}(treeRevision))
	args = append(args, interface{}(t.ts.treeID))

	rows, err := stx.Query(args...)
	if err != nil {
		glog.Warningf("Failed to get merkle subtrees: %s", err)
		return nil, err
	}
	defer rows.Close()

	if rows.Err() != nil {
		// Nothing from the DB
		glog.Warningf("Nothing from DB: %s", rows.Err())
		return nil, rows.Err()
	}

	ret := make([]*storagepb.SubtreeProto, 0, len(nodeIDs))

	for rows.Next() {

		var subtreeIDBytes []byte
		var subtreeRev int64
		var nodesRaw []byte
		if err := rows.Scan(&subtreeIDBytes, &subtreeRev, &nodesRaw); err != nil {
			glog.Warningf("Failed to scan merkle subtree: %s", err)
			return nil, err
		}
		var subtree storagepb.SubtreeProto
		if err := proto.Unmarshal(nodesRaw, &subtree); err != nil {
			glog.Warningf("Failed to unmarshal SubtreeProto: %s", err)
			return nil, err
		}
		if subtree.Prefix == nil {
			subtree.Prefix = []byte{}
		}
		ret = append(ret, &subtree)
	}

	// The InternalNodes cache is nil here, but the SubtreeCache (which called
	// this method) will re-populate it.
	return ret, nil
}

func (t *treeTX) storeSubtrees(subtrees []*storagepb.SubtreeProto) error {
	if len(subtrees) == 0 {
		glog.Warning("attempted to store 0 subtrees...")
		return nil
	}

	// TODO(al): probably need to be able to batch this in the case where we have
	// a really large number of subtrees to store.
	args := make([]interface{}, 0, len(subtrees))
	for _, s := range subtrees {
		if s.Prefix == nil {
			panic(fmt.Errorf("nil prefix on %v", s))
		}
		// Ensure we're not storing the internal nodes, since we'll just recalculate
		// them when we read this subtree back.
		s.InternalNodes = nil
		subtreeBytes, err := proto.Marshal(s)
		if err != nil {
			return err
		}
		args = append(args, t.ts.treeID)
		args = append(args, s.Prefix)
		args = append(args, subtreeBytes)
		args = append(args, t.writeRevision)
	}

	tmpl, err := t.ts.setSubtreeStmt(len(subtrees))
	if err != nil {
		return err
	}
	stx := t.tx.Stmt(tmpl)
	defer stx.Close()

	r, err := stx.Exec(args...)
	if err != nil {
		glog.Warningf("Failed to set merkle subtrees: %s", err)
		return err
	}
	_, _ = r.RowsAffected()
	return nil
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
		return fmt.Errorf("Expected %d row(s) to be affected but saw: %d", count,
			rowsAffected)
	}

	return nil
}

// GetTreeRevisionAtSize returns the max node version for a tree at a particular size.
// It is an error to request tree sizes larger than the currently published tree size.
// For an inexact tree size this implementation always returns the next largest revision if an
// exact one does not exist but it isn't required to do so.
func (t *treeTX) GetTreeRevisionIncludingSize(treeSize int64) (int64, int64, error) {
	// Negative size is not sensible and a zero sized tree has no nodes so no revisions
	if treeSize <= 0 {
		return 0, 0, fmt.Errorf("invalid tree size: %d", treeSize)
	}

	var treeRevision, actualTreeSize int64
	err := t.tx.QueryRow(selectTreeRevisionAtSizeOrLargerSQL, t.ts.treeID, treeSize).Scan(&treeRevision, &actualTreeSize)

	return treeRevision, actualTreeSize, err
}

// getSubtreesAtRev returns a GetSubtreesFunc which reads at the passed in rev.
func (t *treeTX) getSubtreesAtRev(rev int64) cache.GetSubtreesFunc {
	return func(ids []storage.NodeID) ([]*storagepb.SubtreeProto, error) {
		return t.getSubtrees(rev, ids)
	}
}

// GetMerkleNodes returns the requests nodes at (or below) the passed in treeRevision.
func (t *treeTX) GetMerkleNodes(treeRevision int64, nodeIDs []storage.NodeID) ([]storage.Node, error) {
	return t.subtreeCache.GetNodes(nodeIDs, t.getSubtreesAtRev(treeRevision))
}

func (t *treeTX) SetMerkleNodes(nodes []storage.Node) error {
	for _, n := range nodes {
		err := t.subtreeCache.SetNodeHash(n.NodeID, n.Hash,
			func(nID storage.NodeID) (*storagepb.SubtreeProto, error) {
				return t.getSubtree(t.writeRevision, nID)
			})
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *treeTX) Commit() error {
	if t.writeRevision > -1 {
		t.subtreeCache.Flush(t.storeSubtrees)
	}
	t.closed = true
	err := t.tx.Commit()

	if err != nil {
		glog.Warningf("TX commit error: %s", err)
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
