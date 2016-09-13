package mysql

import (
	"database/sql"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/cache"
)

const insertMapHeadSQL string = `INSERT INTO MapHead(TreeId, MapHeadTimestamp, RootHash, MapRevision, RootSignature, TransactionLogRoot)
	VALUES(?, ?, ?, ?, ?, ?)`

const selectLatestSignedMapRootSql string = `SELECT MapHeadTimestamp, RootHash, MapRevision, RootSignature, TransactionLogRoot
		 FROM MapHead WHERE TreeId=?
		 ORDER BY MapHeadTimestamp DESC LIMIT 1`

const insertMapLeafSQL string = `INSERT INTO MapLeaf(TreeId, KeyHash, MapRevision, TheData) VALUES (?, ?, ?, ?)`
const selectMapLeafSQL string = `SELECT KeyHash, MapRevision, TheData
	 FROM MapLeaf
	 WHERE TreeId = ? AND
	 			 KeyHash = ? AND
				 MapRevision <= ?
	 ORDER BY MapRevision DESC LIMIT 1`

type mySQLMapStorage struct {
	mySQLTreeStorage

	mapID trillian.MapID
}

func (m *mySQLMapStorage) MapID() trillian.MapID {
	return m.mapID
}

func NewMapStorage(id trillian.MapID, dbURL string) (storage.MapStorage, error) {
	// TODO(al): pass this through/configure from DB
	th := merkle.NewRFC6962TreeHasher(trillian.NewSHA256())
	ts, err := newTreeStorage(id.TreeID, dbURL, th.Size(), cache.PopulateMapSubtreeNodes(th))
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

func (m *mySQLMapStorage) Begin() (storage.MapTX, error) {
	ttx, err := m.beginTreeTx()
	if err != nil {
		return nil, err
	}
	ret := &mapTX{
		treeTX: ttx,
		ms:     m,
	}

	root, err := ret.LatestSignedMapRoot()
	if err != nil {
		return nil, err
	}

	ret.treeTX.writeRevision = root.MapRevision + 1

	return ret, nil
}

func (m *mySQLMapStorage) Snapshot() (storage.ReadOnlyMapTX, error) {
	tx, err := m.Begin()
	if err != nil {
		return nil, err
	}
	return tx.(storage.ReadOnlyMapTX), err
}

type mapTX struct {
	treeTX
	ms *mySQLMapStorage
}

func (t *mapTX) WriteRevision() int64 {
	return t.treeTX.writeRevision
}

func (m *mapTX) Set(keyHash trillian.Hash, value trillian.MapLeaf) error {
	// TODO(al): consider storing some sort of value which represents the group of keys being set in this Tx.
	//           That way, if this attempt partially fails (i.e. because some subset of the in-the-future merkle
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

	_, err = stmt.Exec(m.ms.mapID.TreeID, []byte(keyHash), m.writeRevision, flatValue)
	return err
}

func (m *mapTX) Get(revision int64, keyHash trillian.Hash) (trillian.MapLeaf, error) {
	stmt, err := m.tx.Prepare(selectMapLeafSQL)
	if err != nil {
		return trillian.MapLeaf{}, err
	}
	defer stmt.Close()

	var mapKeyHash trillian.Hash
	var mapRevision int64
	var flatData []byte

	err = stmt.QueryRow(
		m.ms.mapID.TreeID, []byte(keyHash), revision).Scan(
		&mapKeyHash, &mapRevision, &flatData)

	// It's possible there is no value for this value yet
	if err == sql.ErrNoRows {
		return trillian.MapLeaf{}, storage.ErrNoSuchKey
	} else if err != nil {
		return trillian.MapLeaf{}, err
	}

	var mapLeaf trillian.MapLeaf
	err = proto.Unmarshal(flatData, &mapLeaf)
	return mapLeaf, err
}

func (m *mapTX) LatestSignedMapRoot() (trillian.SignedMapRoot, error) {
	var timestamp, mapRevision int64
	var rootHash, rootSignatureBytes []byte
	var transactionLogRoot []byte
	var rootSignature trillian.DigitallySigned

	err := m.tx.QueryRow(
		selectLatestSignedMapRootSql, m.ms.mapID.TreeID).Scan(
		&timestamp, &rootHash, &mapRevision, &rootSignatureBytes, &transactionLogRoot)

	// It's possible there are no roots for this tree yet
	if err == sql.ErrNoRows {
		return trillian.SignedMapRoot{}, nil
	}

	err = proto.Unmarshal(rootSignatureBytes, &rootSignature)

	if err != nil {
		glog.Warningf("Failed to unmarshall root signature: %v", err)
		return trillian.SignedMapRoot{}, err
	}

	return trillian.SignedMapRoot{
		RootHash:       rootHash,
		TimestampNanos: timestamp,
		MapRevision:    mapRevision,
		Signature:      &rootSignature,
		MapId:          m.ms.mapID.MapID,
	}, nil
}

func (m *mapTX) StoreSignedMapRoot(root trillian.SignedMapRoot) error {
	signatureBytes, err := proto.Marshal(root.Signature)

	if err != nil {
		glog.Warningf("Failed to marshal root signature: %v %v", root.Signature, err)
		return err
	}

	// TODO(al): store transactionLogHead too
	res, err := m.tx.Exec(insertMapHeadSQL, m.ms.mapID.TreeID, root.TimestampNanos, root.RootHash, root.MapRevision, signatureBytes, []byte{})

	if err != nil {
		glog.Warningf("Failed to store signed map root: %s", err)
	}

	return checkResultOkAndRowCountIs(res, err, 1)
}
