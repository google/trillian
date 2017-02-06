package mysql

import (
	"context"
	"database/sql"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/cache"
)

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

var defaultMapStrata = []int{8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 176}

type mySQLMapStorage struct {
	*mySQLTreeStorage
}

// NewMapStorage creates a mySQLMapStorage instance for the specified MySQL URL.
func NewMapStorage(db *sql.DB) (storage.MapStorage, error) {
	return &mySQLMapStorage{
		mySQLTreeStorage: newTreeStorage(db),
	}, nil
}

func (m *mySQLMapStorage) Begin(ctx context.Context, treeID int64) (storage.MapTX, error) {
	// TODO(codingllama): Validate treeType, read hash algorithm from storage
	th := merkle.NewRFC6962TreeHasher()

	ttx, err := m.beginTreeTx(ctx, treeID, th.Size(), defaultMapStrata, cache.PopulateMapSubtreeNodes(th), cache.PrepareMapSubtreeWrite())
	if err != nil {
		return nil, err
	}

	mtx := &mapTX{
		treeTX: ttx,
		ms:     m,
	}

	root, err := mtx.LatestSignedMapRoot()
	if err != nil {
		return nil, err
	}
	mtx.treeTX.writeRevision = root.MapRevision + 1

	return mtx, nil
}

func (m *mySQLMapStorage) Snapshot(ctx context.Context, treeID int64) (storage.ReadOnlyMapTX, error) {
	tx, err := m.Begin(ctx, treeID)
	if err != nil {
		return nil, err
	}
	return tx.(storage.ReadOnlyMapTX), nil
}

type mapTX struct {
	treeTX
	ms *mySQLMapStorage
}

func (m *mapTX) WriteRevision() int64 {
	return m.treeTX.writeRevision
}

func (m *mapTX) Set(keyHash []byte, value trillian.MapLeaf) error {
	// TODO(al): consider storing some sort of value which represents the group of keys being set in this Tx.
	//           That way, if this attempt partially fails (i.e. because some subset of the in-the-future Merkle
	//           nodes do get written), we can enforce that future map update attempts are a complete replay of
	//           the failed set.
	flatValue, err := proto.Marshal(&value)
	if err != nil {
		return nil
	}

	stmt, err := m.tx.Prepare(insertMapLeafSQL)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(m.treeID, keyHash, m.writeRevision, flatValue)
	return err
}

// MapLeaf indexes are overwritten rather than returning the MapLeaf proto provided in Set.
// TODO: return a map[_something_]Mapleaf or []IndexValue to separate the index from the value.
func (m *mapTX) Get(revision int64, indexes [][]byte) ([]trillian.MapLeaf, error) {
	stmt, err := m.ms.getStmt(selectMapLeafSQL, len(indexes), "?", "?")
	if err != nil {
		return nil, err
	}
	stx := m.tx.Stmt(stmt)
	defer stx.Close()

	args := make([]interface{}, 0, len(indexes)+2)
	for _, index := range indexes {
		args = append(args, index)
	}
	args = append(args, m.treeID)
	args = append(args, revision)

	glog.Infof("args size %d", len(args))

	rows, err := stx.Query(args...)
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

func (m *mapTX) LatestSignedMapRoot() (trillian.SignedMapRoot, error) {
	var timestamp, mapRevision int64
	var rootHash, rootSignatureBytes []byte
	var rootSignature trillian.DigitallySigned
	var mapperMetaBytes []byte
	var mapperMeta *trillian.MapperMetadata

	stmt, err := m.tx.Prepare(selectLatestSignedMapRootSQL)
	if err != nil {
		return trillian.SignedMapRoot{}, err
	}
	defer stmt.Close()

	err = stmt.QueryRow(m.treeID).Scan(
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

func (m *mapTX) StoreSignedMapRoot(root trillian.SignedMapRoot) error {
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

	stmt, err := m.tx.Prepare(insertMapHeadSQL)
	if err != nil {
		return err
	}
	defer stmt.Close()

	// TODO(al): store transactionLogHead too
	res, err := stmt.Exec(m.treeID, root.TimestampNanos, root.RootHash, root.MapRevision, signatureBytes, mapperMetaBytes)

	if err != nil {
		glog.Warningf("Failed to store signed map root: %s", err)
	}

	return checkResultOkAndRowCountIs(res, err, 1)
}
