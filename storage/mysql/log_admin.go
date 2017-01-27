package mysql

import (
	"database/sql"
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
func CreateTree(treeID int64, db *sql.DB) error {
	// Insert Tree Row
	stmt, err := db.Prepare(setTreePropertiesSQL)
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(treeID, keyID, treeType, leafHasherType, treeHasherType, allowDuplicateLeaves)
	if err != nil {
		return err
	}
	// Insert Tree Control Row
	stmt2, err := db.Prepare(setTreeParametersSQL)
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
func DeleteTree(treeID int64, db *sql.DB) error {
	for _, sql := range []string{deleteTreeControlSQL, deleteTreeSQL} {
		stmt, err := db.Prepare(sql)
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
