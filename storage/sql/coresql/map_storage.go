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

package coresql

import (
	"context"
	"database/sql"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	spb "github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/cache"
	"github.com/google/trillian/storage/sql/coresql/wrapper"
	"github.com/google/trillian/trees"
)

var defaultMapStrata = []int{8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 176}

type sqlMapStorage struct {
	*sqlTreeStorage
	admin storage.AdminStorage
}

// NewMapStorage creates a sqlMapStorage instance for the specified database wrapper.
// It assumes storage.AdminStorage is backed by the same MySQL database as well.
func NewMapStorage(wrap wrapper.DBWrapper) storage.MapStorage {
	return &sqlMapStorage{
		admin:          NewAdminStorage(wrap),
		sqlTreeStorage: newTreeStorage(wrap),
	}
}

func (m *sqlMapStorage) CheckDatabaseAccessible(ctx context.Context) error {
	return m.wrap.CheckDatabaseAccessible(ctx)
}

type readOnlyMapTX struct {
	tx *sql.Tx
}

func (m *sqlMapStorage) Snapshot(ctx context.Context) (storage.ReadOnlyMapTX, error) {
	tx, err := m.wrap.DB().BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &readOnlyMapTX{tx}, nil
}

func (t *readOnlyMapTX) Commit() error {
	return t.tx.Commit()
}

func (t *readOnlyMapTX) Rollback() error {
	return t.tx.Rollback()
}

func (t *readOnlyMapTX) Close() error {
	if err := t.Rollback(); err != nil && err != sql.ErrTxDone {
		glog.Warningf("Rollback error on Close(): %v", err)
		return err
	}
	return nil
}

func (m *sqlMapStorage) begin(ctx context.Context, treeID int64, readonly bool) (storage.MapTreeTX, error) {
	tree, err := trees.GetTree(
		ctx,
		m.admin,
		treeID,
		trees.GetOpts{TreeType: trillian.TreeType_MAP, Readonly: readonly})
	if err != nil {
		return nil, err
	}
	hasher, err := trees.Hasher(tree)
	if err != nil {
		return nil, err
	}

	ttx, err := m.beginTreeTx(ctx, treeID, hasher.Size(), defaultMapStrata, cache.PopulateMapSubtreeNodes(hasher), cache.PrepareMapSubtreeWrite())
	if err != nil {
		return nil, err
	}

	mtx := &mapTreeTX{
		treeTX: ttx,
		ms:     m,
	}

	mtx.root, err = mtx.LatestSignedMapRoot(ctx)
	if err != nil {
		return nil, err
	}
	mtx.treeTX.writeRevision = mtx.root.MapRevision + 1

	return mtx, nil
}

func (m *sqlMapStorage) BeginForTree(ctx context.Context, treeID int64) (storage.MapTreeTX, error) {
	return m.begin(ctx, treeID, false /* readonly */)
}

func (m *sqlMapStorage) SnapshotForTree(ctx context.Context, treeID int64) (storage.ReadOnlyMapTreeTX, error) {
	tx, err := m.begin(ctx, treeID, true /* readonly */)
	if err != nil {
		return nil, err
	}
	return tx.(storage.ReadOnlyMapTreeTX), nil
}

type mapTreeTX struct {
	treeTX
	ms   *sqlMapStorage
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

	stmt, err := m.ts.wrap.InsertMapLeafStmt(m.tx)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, m.treeID, keyHash, m.writeRevision, flatValue)
	return err
}

// MapLeaf indexes are overwritten rather than returning the MapLeaf proto provided in Set.
// TODO: return a map[_something_]Mapleaf or []IndexValue to separate the index from the value.
func (m *mapTreeTX) Get(ctx context.Context, revision int64, indexes [][]byte) ([]trillian.MapLeaf, error) {
	stmt, err := m.ms.wrap.GetMapLeafStmt(ctx, m.tx, len(indexes))
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	args := make([]interface{}, 0, len(indexes)+2)
	for _, index := range indexes {
		args = append(args, index)
	}
	args = append(args, m.treeID)
	args = append(args, revision)

	glog.Infof("args size %d", len(args))

	rows, err := stmt.QueryContext(ctx, args...)
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
	glog.Infof("%d rows, %d empty", nr, er)
	return ret, nil
}

func (m *mapTreeTX) LatestSignedMapRoot(ctx context.Context) (trillian.SignedMapRoot, error) {
	var timestamp, mapRevision int64
	var rootHash, rootSignatureBytes []byte
	var rootSignature spb.DigitallySigned
	var mapperMetaBytes []byte
	var mapperMeta *trillian.MapperMetadata

	stmt, err := m.ms.wrap.GetLatestMapRootStmt(m.tx)
	if err != nil {
		return trillian.SignedMapRoot{}, err
	}
	defer stmt.Close()

	err = stmt.QueryRowContext(ctx, m.treeID).Scan(
		&timestamp, &rootHash, &mapRevision, &rootSignatureBytes, &mapperMetaBytes)

	// It's possible there are no roots for this tree yet
	if err == sql.ErrNoRows {
		return trillian.SignedMapRoot{}, nil
	}

	err = proto.Unmarshal(rootSignatureBytes, &rootSignature)
	if err != nil {
		glog.Warningf("Failed to unmarshal root signature: %v", err)
		return trillian.SignedMapRoot{}, err
	}

	if mapperMetaBytes != nil && len(mapperMetaBytes) != 0 {
		mapperMeta = &trillian.MapperMetadata{}
		if err := proto.Unmarshal(mapperMetaBytes, mapperMeta); err != nil {
			glog.Warningf("Failed to unmarshal Metadata; %v", err)
			return trillian.SignedMapRoot{}, err
		}
	}

	ret := trillian.SignedMapRoot{
		RootHash:       rootHash,
		TimestampNanos: timestamp,
		MapRevision:    mapRevision,
		Signature:      &rootSignature,
		MapId:          m.treeID,
		Metadata:       mapperMeta,
	}

	return ret, nil
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

	stmt, err := m.ms.wrap.InsertMapHeadStmt(m.tx)
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
