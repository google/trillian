// Copyright 2016 Google Inc. All Rights Reserved.
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

	"github.com/google/trillian"
	"github.com/google/trillian/merkle/hashers"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/cache"
	"github.com/google/trillian/trees"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"

	spb "github.com/google/trillian/crypto/sigpb"
)

const (
	insertMapHeadSQL = `INSERT INTO MapHead(TreeId, MapHeadTimestamp, RootHash, MapRevision, RootSignature, MapperData)
	VALUES(?, ?, ?, ?, ?, ?)`
	selectLatestSignedMapRootSQL = `SELECT MapHeadTimestamp, RootHash, MapRevision, RootSignature, MapperData
		 FROM MapHead WHERE TreeId=?
		 ORDER BY MapHeadTimestamp DESC LIMIT 1`
	selectGetSignedMapRootSQL = `SELECT MapHeadTimestamp, RootHash, MapRevision, RootSignature, MapperData
		 FROM MapHead WHERE TreeId=? AND MapRevision=?`
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

var defaultMapStrata = []int{8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 176}

type mySQLMapStorage struct {
	*mySQLTreeStorage
	admin storage.AdminStorage
}

// NewMapStorage creates a storage.MapStorage instance for the specified MySQL URL.
// It assumes storage.AdminStorage is backed by the same MySQL database as well.
func NewMapStorage(db *sql.DB) storage.MapStorage {
	return &mySQLMapStorage{
		admin:            NewAdminStorage(db),
		mySQLTreeStorage: newTreeStorage(db),
	}
}

func (m *mySQLMapStorage) CheckDatabaseAccessible(ctx context.Context) error {
	return checkDatabaseAccessible(ctx, m.db)
}

type readOnlyMapTX struct {
	*sql.Tx
}

func (m *mySQLMapStorage) Snapshot(ctx context.Context) (storage.ReadOnlyMapTX, error) {
	tx, err := m.db.BeginTx(ctx, nil /* opts */)
	if err != nil {
		return nil, err
	}
	return &readOnlyMapTX{tx}, nil
}

func (t *readOnlyMapTX) Close() error {
	if err := t.Rollback(); err != nil && err != sql.ErrTxDone {
		glog.Warningf("Rollback error on Close(): %v", err)
		return err
	}
	return nil
}

func (m *mySQLMapStorage) begin(ctx context.Context, treeID int64, readonly bool) (storage.MapTreeTX, error) {
	tree, err := trees.GetTree(
		ctx,
		m.admin,
		treeID,
		trees.GetOpts{TreeType: trillian.TreeType_MAP, Readonly: readonly})
	if err != nil {
		return nil, err
	}
	hasher, err := hashers.NewMapHasher(tree.HashStrategy)
	if err != nil {
		return nil, err
	}

	stCache := cache.NewMapSubtreeCache(defaultMapStrata, treeID, hasher)
	ttx, err := m.beginTreeTx(ctx, treeID, hasher.Size(), stCache)
	if err != nil {
		return nil, err
	}

	mtx := &mapTreeTX{
		treeTX: ttx,
		ms:     m,
	}

	mtx.root, err = mtx.LatestSignedMapRoot(ctx)
	if err != nil && err != storage.ErrMapNeedsInit {
		return nil, err
	}
	if err == storage.ErrMapNeedsInit {
		return mtx, err
	}

	mtx.treeTX.writeRevision = mtx.root.MapRevision + 1
	return mtx, nil
}

func (m *mySQLMapStorage) BeginForTree(ctx context.Context, treeID int64) (storage.MapTreeTX, error) {
	return m.begin(ctx, treeID, false /* readonly */)
}

func (m *mySQLMapStorage) SnapshotForTree(ctx context.Context, treeID int64) (storage.ReadOnlyMapTreeTX, error) {
	return m.begin(ctx, treeID, true /* readonly */)
}

type mapTreeTX struct {
	treeTX
	ms   *mySQLMapStorage
	root trillian.SignedMapRoot
}

func (m *mapTreeTX) ReadRevision() int64 {
	return m.root.MapRevision
}

func (m *mapTreeTX) WriteRevision() int64 {
	return m.treeTX.writeRevision
}

func (m *mapTreeTX) Set(ctx context.Context, keyHash []byte, value trillian.MapLeaf) error {
	// TODO(al): consider storing some sort of value which represents the group of keys being set in this Tx.
	//           That way, if this attempt partially fails (i.e. because some subset of the in-the-future Merkle
	//           nodes do get written), we can enforce that future map update attempts are a complete replay of
	//           the failed set.
	flatValue, err := proto.Marshal(&value)
	if err != nil {
		return nil
	}

	stmt, err := m.tx.PrepareContext(ctx, insertMapLeafSQL)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, m.treeID, keyHash, m.writeRevision, flatValue)
	return err
}

// Get returns a list of map leaves indicated by indexes.
// If an index is not found, no corresponding entry is returned.
// Each MapLeaf.Index is overwritten with the index the leaf was found at.
func (m *mapTreeTX) Get(ctx context.Context, revision int64, indexes [][]byte) ([]trillian.MapLeaf, error) {
	// If no indexes are requested, return an empty set.
	if len(indexes) == 0 {
		return []trillian.MapLeaf{}, nil
	}
	stmt, err := m.ms.getStmt(ctx, selectMapLeafSQL, len(indexes), "?", "?")
	if err != nil {
		return nil, err
	}
	stx := m.tx.StmtContext(ctx, stmt)
	defer stx.Close()

	args := make([]interface{}, 0, len(indexes)+2)
	for _, index := range indexes {
		args = append(args, index)
	}
	args = append(args, m.treeID)
	args = append(args, revision)

	rows, err := stx.QueryContext(ctx, args...)
	// It's possible there are no values for any of these keys yet
	if err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	ret := make([]trillian.MapLeaf, 0, len(indexes))
	nr := 0
	er := 0
	for rows.Next() {
		var mapKeyHash []byte
		var mapRevision int64
		var flatData []byte
		err = rows.Scan(&mapKeyHash, &mapRevision, &flatData)
		if err != nil {
			return nil, err
		}
		if len(flatData) == 0 {
			er++
			continue
		}
		var mapLeaf trillian.MapLeaf
		err = proto.Unmarshal(flatData, &mapLeaf)
		if err != nil {
			return nil, err
		}
		mapLeaf.Index = mapKeyHash
		ret = append(ret, mapLeaf)
		nr++
	}
	return ret, nil
}

func (m *mapTreeTX) GetSignedMapRoot(ctx context.Context, revision int64) (trillian.SignedMapRoot, error) {
	var timestamp, mapRevision int64
	var rootHash, rootSignatureBytes []byte
	var mapperMetaBytes []byte

	stmt, err := m.tx.PrepareContext(ctx, selectGetSignedMapRootSQL)
	if err != nil {
		return trillian.SignedMapRoot{}, err
	}
	defer stmt.Close()

	err = stmt.QueryRowContext(ctx, m.treeID, revision).Scan(
		&timestamp, &rootHash, &mapRevision, &rootSignatureBytes, &mapperMetaBytes)
	if err != nil {
		if revision == 0 {
			return trillian.SignedMapRoot{}, storage.ErrMapNeedsInit
		}
		return trillian.SignedMapRoot{}, err
	}
	return m.signedMapRoot(timestamp, mapRevision, rootHash, rootSignatureBytes, mapperMetaBytes)
}

func (m *mapTreeTX) LatestSignedMapRoot(ctx context.Context) (trillian.SignedMapRoot, error) {
	var timestamp, mapRevision int64
	var rootHash, rootSignatureBytes []byte
	var mapperMetaBytes []byte

	stmt, err := m.tx.PrepareContext(ctx, selectLatestSignedMapRootSQL)
	if err != nil {
		return trillian.SignedMapRoot{}, err
	}
	defer stmt.Close()

	err = stmt.QueryRowContext(ctx, m.treeID).Scan(
		&timestamp, &rootHash, &mapRevision, &rootSignatureBytes, &mapperMetaBytes)

	// It's possible there are no roots for this tree yet
	if err == sql.ErrNoRows {
		return trillian.SignedMapRoot{}, storage.ErrMapNeedsInit
	} else if err != nil {
		return trillian.SignedMapRoot{}, err
	}
	return m.signedMapRoot(timestamp, mapRevision, rootHash, rootSignatureBytes, mapperMetaBytes)
}

func (m *mapTreeTX) signedMapRoot(timestamp, mapRevision int64, rootHash, rootSignatureBytes, mapperMetaBytes []byte) (trillian.SignedMapRoot, error) {
	rootSignature := &spb.DigitallySigned{}

	err := proto.Unmarshal(rootSignatureBytes, rootSignature)
	if err != nil {
		err = fmt.Errorf("signedMapRoot: failed to unmarshal root signature: %v", err)
		return trillian.SignedMapRoot{}, err
	}

	smr := trillian.SignedMapRoot{
		RootHash:       rootHash,
		TimestampNanos: timestamp,
		MapRevision:    mapRevision,
		Signature:      rootSignature,
		MapId:          m.treeID,
	}

	if len(mapperMetaBytes) > 0 {
		mapperMeta := &any.Any{}
		if err := proto.Unmarshal(mapperMetaBytes, mapperMeta); err != nil {
			err = fmt.Errorf("signedMapRoot: failed to unmarshal metadata: %v", err)
			return trillian.SignedMapRoot{}, err
		}
		smr.Metadata = mapperMeta
	}

	return smr, nil
}

func (m *mapTreeTX) StoreSignedMapRoot(ctx context.Context, root trillian.SignedMapRoot) error {
	signatureBytes, err := proto.Marshal(root.Signature)
	if err != nil {
		glog.Warningf("Failed to marshal root signature: %v %v", root.Signature, err)
		return err
	}

	var mapperMetaBytes []byte

	if root.Metadata != nil {
		mapperMetaBytes, err = proto.Marshal(root.Metadata)
		if err != nil {
			glog.Warning("Failed to marshal MetaData: %v %v", root.Metadata, err)
			return err
		}
	}

	stmt, err := m.tx.PrepareContext(ctx, insertMapHeadSQL)
	if err != nil {
		return err
	}
	defer stmt.Close()

	// TODO(al): store transactionLogHead too
	res, err := stmt.ExecContext(ctx, m.treeID, root.TimestampNanos, root.RootHash, root.MapRevision, signatureBytes, mapperMetaBytes)

	if err != nil {
		glog.Warningf("Failed to store signed map root: %s", err)
	}

	return checkResultOkAndRowCountIs(res, err, 1)
}
