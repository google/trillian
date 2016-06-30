package mysql

import (
	"database/sql"
	"errors"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
)

const insertMapHeadSQL string = `INSERT INTO MapHead(TreeId, MapHeadTimestamp, RootHash, MapRevision, RootSignature, TransactionLogRoot)
	VALUES(?, ?, ?, ?, ?, ?)`

const selectLatestSignedMapRootSql string = `SELECT MapHeadTimestamp, RootHash, MapRevision, RootSignature, TransactionLogRoot
		 FROM MapHead WHERE TreeId=?
		 ORDER BY MapHeadTimestamp DESC LIMIT 1`

type mySQLMapStorage struct {
	mySQLTreeStorage

	mapID trillian.MapID
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

type mapTX struct {
	treeTX
	ms *mySQLMapStorage
}

func (m *mapTX) Set(key []byte, value trillian.MapLeaf) error {
	return errors.New("unimplemented")
}

func (m *mapTX) Get(revision int64, key []byte) (trillian.MapLeaf, error) {
	return trillian.MapLeaf{}, errors.New("unimplemented")
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
