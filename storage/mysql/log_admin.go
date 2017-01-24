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
	deleteTreeSQL1 = `DELETE FROM Trees WHERE TreeId = ?`
	deleteTreeSQL2 = `DELETE FROM TreeControl WHERE TreeId = ?`
)

// CreateLog instantiates a new log with default parameters.
// TODO(codinglama): Move to admin API when the adnmin API is created.
func CreateLog(logID int64, dbURL string) error {
	th := merkle.NewRFC6962TreeHasher(crypto.NewSHA256())
	m, err := newTreeStorage(logID, dbURL, th.Size(), defaultLogStrata, cache.PopulateLogSubtreeNodes(th))
	if err != nil {
		return fmt.Errorf("Couldn't create a new treeStorage: %s", err)
	}
	// Set Tree Row
	stmt, err := m.db.Prepare(setTreePropertiesSQL)
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(logID, 1, "MAP", "SHA256", "SHA256", false)
	if err != nil {
		return err
	}
	// Set Tree Control Row
	stmt2, err := m.db.Prepare(setTreeParametersSQL)
	if err != nil {
		return err
	}
	defer stmt2.Close()
	_, err = stmt2.Exec(logID, false, false, false, 1, 1)
	if err != nil {
		return err
	}
	return nil
}

// DeleteLog deletes
func DeleteLog(logID int64, dbURL string) error {
	th := merkle.NewRFC6962TreeHasher(crypto.NewSHA256())
	m, err := newTreeStorage(logID, dbURL, th.Size(), defaultLogStrata, cache.PopulateLogSubtreeNodes(th))
	if err != nil {
		return fmt.Errorf("Couldn't create a new treeStorage: %s", err)
	}
	// Delete Tree Control Row
	stmt2, err := m.db.Prepare(deleteTreeSQL2)
	if err != nil {
		return err
	}
	defer stmt2.Close()
	_, err = stmt2.Exec(logID)
	if err != nil {
		return err
	}
	// Delete Tree Row
	stmt, err := m.db.Prepare(deleteTreeSQL1)
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(logID)
	if err != nil {
		return err
	}
	return nil
}
