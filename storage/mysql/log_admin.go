package mysql

import (
	"fmt"

	"github.com/google/trillian/crypto"
	"github.com/google/trillian/merkle"
	"github.com/google/trillian/storage/cache"
)

const (
	setTreePropertiesSQL = `INSERT INTO Trees(TreeId,KeyId,TreeType,LeafHasherType,TreeHasherType,AllowsDuplicateLeaves) VALUES(?, ?, ?, ?, ?, ?)`
	setTreeParametersSQL = `INSERT INTO TreeControl(TreeId,ReadOnlyRequests,SigningEnabled,SequencingEnabled,SequenceIntervalSeconds,SignIntervalSeconds) 
		VALUES(?, ?, ?, ?, ?, ?)`
	deleteTreeSQL        = `DELETE FROM Trees WHERE TreeId = ?`
	deleteTreeControlSQL = `DELETE FROM TreeControl WHERE TreeId = ?`

	keyID                = 1
	treeType             = "LOG"
	leafHasherType       = "SHA256"
	treeHasherType       = "SHA256"
	allowDuplicateLeaves = false
	readOnly             = false
	signingEnabled       = false
	sequencingEnabled    = false
	sequenceInterval     = 1
	signInterval         = 1
)

// CreateTree instantiates a new log with default parameters.
// TODO(codinglama): Move to admin API when the admin API is created.
func CreateTree(treeID int64, dbURL string) error {
	// TODO(codinglama) replace with a GetDatabase from the new extension API when LogID is removed.
	th := merkle.NewRFC6962TreeHasher(crypto.NewSHA256())
	m, err := newTreeStorage(treeID, dbURL, th.Size(), defaultLogStrata, cache.PopulateLogSubtreeNodes(th))
	if err != nil {
		return fmt.Errorf("couldn't create a new treeStorage: %s", err)
	}
	// Insert Tree Row
	stmt, err := m.db.Prepare(setTreePropertiesSQL)
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(treeID, keyID, treeType, leafHasherType, treeHasherType, allowDuplicateLeaves)
	if err != nil {
		return err
	}
	// Insert Tree Control Row
	stmt2, err := m.db.Prepare(setTreeParametersSQL)
	if err != nil {
		return err
	}
	defer stmt2.Close()
	_, err = stmt2.Exec(treeID, readOnly, signingEnabled, sequencingEnabled, sequenceInterval, signInterval)
	if err != nil {
		return err
	}
	return nil
}

// DeleteTree deletes a tree by the treeID.
func DeleteTree(treeID int64, dbURL string) error {
	// TODO(codinglama) replace with a GetDatabase from the new extension API when LogID is removed.
	th := merkle.NewRFC6962TreeHasher(crypto.NewSHA256())
	m, err := newTreeStorage(treeID, dbURL, th.Size(), defaultLogStrata, cache.PopulateLogSubtreeNodes(th))
	if err != nil {
		return fmt.Errorf("couldn't create a new treeStorage: %s", err)
	}

	for _, sql := range []string{deleteTreeControlSQL, deleteTreeSQL} {
		stmt, err := m.db.Prepare(sql)
		if err != nil {
			return err
		}
		defer stmt.Close()
		_, err = stmt.Exec(treeID)
		if err != nil {
			return err
		}
	}
	return nil
}
