// Copyright 2018 Google Inc. All Rights Reserved.
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

package main

import (
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian/types"
	"github.com/jinzhu/gorm"

	// Include all currently supported gorm dialects
	_ "github.com/jinzhu/gorm/dialects/mssql"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

// initialEmptyRoot represents an empty trusted root and is used when no
// information is known about the log being monitored.
var initialEmptyRoot TrustedLogRoot

func init() {
	// Set up initialEmptyRoot, which will get used when the database does not
	// have an existing trusted log root (ie. the first time a tree is
	// monitored).
	emptyRoot := &types.LogRootV1{
		TreeSize: 0,
	}
	marshalled, err := emptyRoot.MarshalBinary()
	if err != nil {
		glog.Exitf("Could not marshal the empty root: %v\n", err)
	}
	initialEmptyRoot = TrustedLogRoot{MarshalledRoot: marshalled, TreeSize: 0}
}

// TrustedLogRoot is the database model used for storing information about
// trusted log roots.
type TrustedLogRoot struct {
	LogID          int64 `gorm:"primary_key"`
	MarshalledRoot []byte
	TreeSize       uint64

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Open opens a new database connection
func Open(database, databaseURI string) (*gorm.DB, error) {
	db, err := gorm.Open(database, databaseURI)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	return db, nil
}

// Setup ensures that the database tables are set up properly.
func Setup(db *gorm.DB) error {
	if err := db.AutoMigrate(&TrustedLogRoot{}).Error; err != nil {
		return fmt.Errorf("failed to migrate the trusted_log_roots table: %v", err)
	}

	return nil
}

// GetCurrentRootFor returns the current trusted root from the database.
func GetCurrentRootFor(db *gorm.DB, logID int64) (*types.LogRootV1, error) {
	trustedLogRoot := TrustedLogRoot{}

	query := db.Where(TrustedLogRoot{LogID: logID})
	query = query.Attrs(initialEmptyRoot) // Values to write if the record is not found.
	if err := query.FirstOrCreate(&trustedLogRoot).Error; err != nil {
		return nil, fmt.Errorf("failed to get or create the current trusted root for log %d: %v", logID, err)
	}

	if trustedLogRoot.TreeSize == 0 {
		glog.Warningf("Using an empty root for log %d", logID)
	}

	currentRoot := &types.LogRootV1{}
	if err := currentRoot.UnmarshalBinary(trustedLogRoot.MarshalledRoot); err != nil {
		return nil, fmt.Errorf("failed to unmarshal the current trusted root for log %d: %v", logID, err)
	}

	return currentRoot, nil
}

// WriteRootFor updates the current trusted root in the database
func WriteRootFor(db *gorm.DB, logID int64, newRoot *types.LogRootV1) error {
	binary, err := newRoot.MarshalBinary()
	if err != nil {
		return fmt.Errorf("could not marshal the new root: %v", err)
	}

	query := db.Model(&TrustedLogRoot{LogID: logID})
	result := query.Update(TrustedLogRoot{
		MarshalledRoot: binary,
		TreeSize:       newRoot.TreeSize,
	})

	if result.Error != nil {
		return fmt.Errorf("could not update the root for log %d: %v", logID, result.Error)
	}

	return nil
}
