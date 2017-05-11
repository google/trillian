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
	"fmt"
	"strings"
	"sync"

	"github.com/go-sql-driver/mysql"
	"github.com/golang/glog"
	"github.com/google/trillian/storage/sql/coresql/wrapper"
	"github.com/google/trillian/storage"
)

// These are all tree related queries
const (
	insertSubtreeMultiSQL = `INSERT INTO Subtree(TreeId, SubtreeId, Nodes, SubtreeRevision) ` + placeholderSQL
	selectSubtreeSQL      = `
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

	selectTreeRevisionAtSizeOrLargerSQL = "SELECT TreeRevision,TreeSize FROM TreeHead WHERE TreeId=? AND TreeSize>=? ORDER BY TreeRevision LIMIT 1"

	insertTreeHeadSQL = `INSERT INTO TreeHead(TreeId,TreeHeadTimestamp,TreeSize,RootHash,TreeRevision,RootSignature)
		 VALUES(?,?,?,?,?,?)`
	selectActiveLogsSQL                = "SELECT TreeId from Trees where TreeType='LOG'"
	selectActiveLogsWithUnsequencedSQL = "SELECT DISTINCT t.TreeId from Trees t INNER JOIN Unsequenced u WHERE TreeType='LOG' AND t.TreeId=u.TreeId"
	selectTreeRowSQL                   = "SELECT 1 FROM Trees WHERE TreeId = ?"
)

// These are all log related queries
const (
	selectLeavesByIndexSQL = `
	    SELECT s.MerkleLeafHash,l.LeafIdentityHash,l.LeafValue,s.SequenceNumber,l.ExtraData
			FROM LeafData l,SequencedLeafData s
			WHERE l.LeafIdentityHash = s.LeafIdentityHash
			AND s.SequenceNumber IN (` + placeholderSQL + `) AND l.TreeId = ? AND s.TreeId = l.TreeId`
	selectLeavesByMerkleHashSQL = `
			SELECT s.MerkleLeafHash,l.LeafIdentityHash,l.LeafValue,s.SequenceNumber,l.ExtraData
			FROM LeafData l,SequencedLeafData s
			WHERE l.LeafIdentityHash = s.LeafIdentityHash
			AND s.MerkleLeafHash IN (` + placeholderSQL + `) AND l.TreeId = ? AND s.TreeId = l.TreeId`
	// Same as above except with leaves ordered by sequence so we only incur this cost when necessary
	orderBySequenceNumberSQL                     = " ORDER BY s.SequenceNumber"
	selectLeavesByMerkleHashOrderedBySequenceSQL = selectLeavesByMerkleHashSQL + orderBySequenceNumberSQL
	// TODO(drysdale): rework the code so the dummy hash isn't needed (e.g. this assumes hash size is 32)
	dummyMerkleLeafHash = "00000000000000000000000000000000"
	// This statement returns a dummy Merkle leaf hash value (which must be
	// of the right size) so that its signature matches that of the other
	// leaf-selection statements.
	selectLeavesByLeafIdentityHashSQL = `SELECT '` + dummyMerkleLeafHash + `',l.LeafIdentityHash,l.LeafValue,-1,l.ExtraData
			FROM LeafData l
			WHERE l.LeafIdentityHash IN (` + placeholderSQL + `) AND l.TreeId = ?`
	selectQueuedLeavesSQL = `SELECT LeafIdentityHash,MerkleLeafHash
			FROM Unsequenced
			WHERE TreeID=?
			AND QueueTimestampNanos<=?
			ORDER BY QueueTimestampNanos,LeafIdentityHash ASC LIMIT ?`
	deleteUnsequencedSQL         = "DELETE FROM Unsequenced WHERE LeafIdentityHash IN (<placeholder>) AND TreeId = ?"
	selectLatestSignedLogRootSQL = `SELECT TreeHeadTimestamp,TreeSize,RootHash,TreeRevision,RootSignature
			FROM TreeHead WHERE TreeId=?
			ORDER BY TreeHeadTimestamp DESC LIMIT 1`
	insertUnsequencedEntrySQL = `INSERT INTO Unsequenced(TreeId,LeafIdentityHash,MerkleLeafHash,MessageId,QueueTimestampNanos)
			VALUES(?,?,?,?,?)`
	insertUnsequencedLeafSQL = `INSERT INTO LeafData(TreeId,LeafIdentityHash,LeafValue,ExtraData)
			VALUES(?,?,?,?)`
	insertSequencedLeafSQL = `INSERT INTO SequencedLeafData(TreeId,LeafIdentityHash,MerkleLeafHash,SequenceNumber)
			VALUES(?,?,?,?)`
	selectSequencedLeafCountSQL = "SELECT COUNT(*) FROM SequencedLeafData WHERE TreeId=?"
)

// These are all map related queries
const (
	insertMapHeadSQL = `INSERT INTO MapHead(TreeId, MapHeadTimestamp, RootHash, MapRevision, RootSignature, MapperData)
	VALUES(?, ?, ?, ?, ?, ?)`
	selectLatestSignedMapRootSQL = `SELECT MapHeadTimestamp, RootHash, MapRevision, RootSignature, MapperData
		 FROM MapHead WHERE TreeId=?
		 ORDER BY MapHeadTimestamp DESC LIMIT 1`
	insertMapLeafSQL = `INSERT INTO MapLeaf(TreeId, KeyHash, MapRevision, LeafValue) VALUES (?, ?, ?, ?)`
	selectMapLeafSQL = `
 SELECT t1.KeyHash, t1.MapRevision, t1.LeafValue
 FROM MapLeaf t1
 INNER JOIN
 (
	SELECT TreeId, KeyHash, MAX(MapRevision) as maxrev
	FROM MapLeaf t0
	WHERE t0.KeyHash IN (` + placeholderSQL + `) AND
	      t0.TreeId = ? AND t0.MapRevision <= ?
	GROUP BY t0.TreeId, t0.KeyHash
 ) t2
 ON t1.TreeId=t2.TreeId
 AND t1.KeyHash=t2.KeyHash
 AND t1.MapRevision=t2.maxrev`
)

// These are all admin related queries
const (
	selectTreeIDsSQL  = "SELECT TreeId FROM Trees"
	selectAllTreesSQL = `
		SELECT
			TreeId,
			TreeState,
			TreeType,
			HashStrategy,
			HashAlgorithm,
			SignatureAlgorithm,
			DisplayName,
			Description,
			CreateTimeMillis,
			UpdateTimeMillis,
			PrivateKey,
			PublicKey
		FROM Trees`
	selectTreeByIDSQL = selectAllTreesSQL + " WHERE TreeId = ?"
	insertTreeSQL     = `
		INSERT INTO Trees(
			TreeId,
			TreeState,
			TreeType,
			HashStrategy,
			HashAlgorithm,
			SignatureAlgorithm,
			DisplayName,
			Description,
			CreateTimeMillis,
			UpdateTimeMillis,
			PrivateKey,
			PublicKey)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	insertTreeControlSQL = `
		INSERT INTO TreeControl(
			TreeId,
			SigningEnabled,
			SequencingEnabled,
			SequenceIntervalSeconds)
		VALUES(?, ?, ?, ?)`
	updateTreeSQL = `
		UPDATE Trees
		SET TreeState = ?, DisplayName = ?, Description = ?, UpdateTimeMillis = ?
		WHERE TreeId = ?`
)

// Error code returned by MySQL driver when inserting a duplicate row
const errNumDuplicate = 1062

type mySQLWrapper struct {
	// Statements managed by this provider are specific to this database.
	db *sql.DB
	// Must hold the mutex before manipulating the statement map. Sharing a lock because
	// it only needs to be held while the statements are built, not while they execute and
	// this will be a short time. These maps are from the number of placeholder '?'
	// in the query to the statement that should be used.
	statementMutex sync.Mutex
	statements     map[string]map[int]*sql.Stmt
}

// NewWrapper creates and returns a DBWrapper appropriate for use with MySQL.
func NewWrapper(db *sql.DB) wrapper.DBWrapper {
	return &mySQLWrapper{
		db:         db,
		statements: make(map[string]map[int]*sql.Stmt),
	}
}

func (m *mySQLWrapper) DB() *sql.DB {
	return m.db
}

func (m *mySQLWrapper) GetSubtrees(ctx context.Context, tx *sql.Tx, treeID, treeRevision int64, nodeIDs []storage.NodeID, subtreeScanFn func(*sql.Rows) error) error {
	args := make([]interface{}, 0, len(nodeIDs)+3)
	// populate args with nodeIDs, variable args first
	for _, nodeID := range nodeIDs {
		if nodeID.PrefixLenBits%8 != 0 {
			return fmt.Errorf("invalid subtree ID - not multiple of 8: %d", nodeID.PrefixLenBits)
		}

		nodeIDBytes := nodeID.Path[:nodeID.PrefixLenBits/8]
		args = append(args, interface{}(nodeIDBytes))
	}
	args = append(args, interface{}(treeID))
	args = append(args, interface{}(treeRevision))
	args = append(args, interface{}(treeID))
	stmt, err := wrapper.PrepInTx(ctx, tx, func() (stmt *sql.Stmt, err error) {
		return m.getStmt(selectSubtreeSQL, len(nodeIDs), "?", "?")
	})
	if stmt != nil {
		defer stmt.Close()
	}
	if err != nil {
		return err
	}
	rows, err := stmt.QueryContext(ctx, args...)
	if err != nil {
		glog.Warningf("Failed to get merkle subtrees: %v", err)
		return err
	}
	defer rows.Close()
	return subtreeScanFn(rows)
}

func (m *mySQLWrapper) SetSubtrees(ctx context.Context, tx *sql.Tx, args []interface{}) error {
	if len(args) % 4 != 0 {
		return fmt.Errorf("args for SetSubtrees must be multiple of 4 but got: %d", len(args))
	}
	stmt, err := wrapper.PrepInTx(ctx, tx, func() (stmt *sql.Stmt, err error) {
		return m.getStmt(insertSubtreeMultiSQL, len(args) / 4, "VALUES(?, ?, ?, ?)", "(?, ?, ?, ?)")
	})
	if stmt != nil {
		defer stmt.Close()
	}
	if err != nil {
		return err
	}
	_, err = stmt.ExecContext(ctx, args...)
	return err
}

func (m *mySQLWrapper) GetTreeRevisionIncludingSize(ctx context.Context, tx *sql.Tx, treeID, treeSize int64) (int64, int64, error) {
	var treeRevision, actualTreeSize int64
	stmt, err := tx.PrepareContext(ctx, selectTreeRevisionAtSizeOrLargerSQL)
	if err != nil {
		return 0, 0, err
	}
	defer stmt.Close()
	err = stmt.QueryRowContext(ctx, treeID, treeSize).Scan(&treeRevision, &actualTreeSize)
	if err != nil {
		return 0, 0, err
	}
	return treeRevision, actualTreeSize, err
}

func (m *mySQLWrapper) InsertTreeHeadStmt(tx *sql.Tx) (*sql.Stmt, error) {
	return tx.Prepare(insertTreeHeadSQL)
}

func (m *mySQLWrapper) GetActiveLogsStmt(tx *sql.Tx) (*sql.Stmt, error) {
	return tx.Prepare(selectActiveLogsSQL)
}

func (m *mySQLWrapper) GetActiveLogsWithWorkStmt(tx *sql.Tx) (*sql.Stmt, error) {
	return tx.Prepare(selectActiveLogsWithUnsequencedSQL)
}

func (m *mySQLWrapper) GetLeavesByIndexStmt(ctx context.Context, tx *sql.Tx, num int) (*sql.Stmt, error) {
	return wrapper.PrepInTx(ctx, tx, func() (stmt *sql.Stmt, err error) {
		return m.getStmt(selectLeavesByIndexSQL, num, "?", "?")
	})
}

func (m *mySQLWrapper) GetLeavesByLeafIdentityHashStmt(ctx context.Context, tx *sql.Tx, num int) (*sql.Stmt, error) {
	return wrapper.PrepInTx(ctx, tx, func() (stmt *sql.Stmt, err error) {
		return m.getStmt(selectLeavesByLeafIdentityHashSQL, num, "?", "?")
	})
}

func (m *mySQLWrapper) DeleteUnsequencedStmt(ctx context.Context, tx *sql.Tx, num int) (*sql.Stmt, error) {
	return wrapper.PrepInTx(ctx, tx, func() (stmt *sql.Stmt, err error) {
		return m.getStmt(deleteUnsequencedSQL, num, "?", "?")
	})
}

func (m *mySQLWrapper) GetLeavesByMerkleHashStmt(ctx context.Context, tx *sql.Tx, num int, orderBySequence bool) (*sql.Stmt, error) {
	if orderBySequence {
		return wrapper.PrepInTx(ctx, tx, func() (stmt *sql.Stmt, err error) {
			return m.getStmt(selectLeavesByMerkleHashOrderedBySequenceSQL, num, "?", "?")
		})
	}

	return wrapper.PrepInTx(ctx, tx, func() (stmt *sql.Stmt, err error) {
		return m.getStmt(selectLeavesByMerkleHashSQL, num, "?", "?")
	})
}

func (m *mySQLWrapper) GetLatestSignedLogRootStmt(tx *sql.Tx) (*sql.Stmt, error) {
	return tx.Prepare(selectLatestSignedLogRootSQL)
}

func (m *mySQLWrapper) GetQueuedLeavesStmt(tx *sql.Tx) (*sql.Stmt, error) {
	return tx.Prepare(selectQueuedLeavesSQL)
}

func (m *mySQLWrapper) InsertUnsequencedEntryStmt(tx *sql.Tx) (*sql.Stmt, error) {
	return tx.Prepare(insertUnsequencedEntrySQL)
}

func (m *mySQLWrapper) InsertUnsequencedLeafStmt(tx *sql.Tx) (*sql.Stmt, error) {
	return tx.Prepare(insertUnsequencedLeafSQL)
}

func (m *mySQLWrapper) InsertSequencedLeafStmt(ctx context.Context, tx *sql.Tx) (*sql.Stmt, error) {
	return tx.PrepareContext(ctx, insertSequencedLeafSQL)
}

func (m *mySQLWrapper) GetSequencedLeafCountStmt(tx *sql.Tx) (*sql.Stmt, error) {
	return tx.Prepare(selectSequencedLeafCountSQL)
}

func (m *mySQLWrapper) GetMapLeafStmt(ctx context.Context, tx *sql.Tx, num int) (*sql.Stmt, error) {
	return wrapper.PrepInTx(ctx, tx, func() (stmt *sql.Stmt, err error) {
		return m.getStmt(selectMapLeafSQL, num, "?", "?")
	})
}

func (m *mySQLWrapper) InsertMapHeadStmt(tx *sql.Tx) (*sql.Stmt, error) {
	return tx.Prepare(insertMapHeadSQL)
}

func (m *mySQLWrapper) GetLatestMapRootStmt(tx *sql.Tx) (*sql.Stmt, error) {
	return tx.Prepare(selectLatestSignedMapRootSQL)
}

func (m *mySQLWrapper) InsertMapLeafStmt(tx *sql.Tx) (*sql.Stmt, error) {
	return tx.Prepare(insertMapLeafSQL)
}

func (m *mySQLWrapper) GetAllTreesStmt(tx *sql.Tx) (*sql.Stmt, error) {
	return tx.Prepare(selectAllTreesSQL)
}

func (m *mySQLWrapper) GetTreeStmt(tx *sql.Tx) (*sql.Stmt, error) {
	return tx.Prepare(selectTreeByIDSQL)
}

func (m *mySQLWrapper) GetTreeIDsStmt(tx *sql.Tx) (*sql.Stmt, error) {
	return tx.Prepare(selectTreeIDsSQL)
}

func (m *mySQLWrapper) InsertTreeStmt(tx *sql.Tx) (*sql.Stmt, error) {
	return tx.Prepare(insertTreeSQL)
}

func (m *mySQLWrapper) InsertTreeControlStmt(tx *sql.Tx) (*sql.Stmt, error) {
	return tx.Prepare(insertTreeControlSQL)
}

func (m *mySQLWrapper) UpdateTreeStmt(tx *sql.Tx) (*sql.Stmt, error) {
	return tx.Prepare(updateTreeSQL)
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

// getStmt creates and caches sql.Stmt structs based on the passed in statement
// and number of bound arguments.
func (m *mySQLWrapper) getStmt(statement string, num int, first, rest string) (*sql.Stmt, error) {
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

func (m *mySQLWrapper) IsDuplicateErr(err error) bool {
	if err != nil {
		if mysqlErr, ok := err.(*mysql.MySQLError); ok && mysqlErr.Number == errNumDuplicate {
			return true
		}
	}

	return false
}

func (m *mySQLWrapper) OnOpenDB(ctx context.Context) error {
	if _, err := m.db.ExecContext(ctx, "SET sql_mode = 'STRICT_ALL_TABLES'"); err != nil {
		glog.Warningf("Failed to set strict mode on mysql db: %s", err)
		return err
	}

	return nil
}

func (m *mySQLWrapper) TreeRowExists(treeID int64) error {
	var num int
	if err := m.db.QueryRow(selectTreeRowSQL, treeID).Scan(&num); err != nil {
		return fmt.Errorf("failed to get tree row for treeID %v: %v", treeID, err)
	}
	return nil
}

func (m *mySQLWrapper) CheckDatabaseAccessible(ctx context.Context) error {
	_ = ctx
	stmt, err := m.DB().PrepareContext(ctx, "SELECT TreeId FROM Trees LIMIT 1")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx)
	return err
}
