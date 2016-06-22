package mysql

import (
	"errors"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
)

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
	return trillian.SignedMapRoot{}, errors.New("unimplemented")
}

func (m *mapTX) StoreSignedMapRoot(root trillian.SignedMapRoot) error {
	return errors.New("unimplemented")
}
