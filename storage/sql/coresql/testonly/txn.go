// Copyright 2017 Google Inc. All Rights Reserved.
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

package testonly

import (
	"context"
	"testing"

	"github.com/google/trillian/storage"
)

// BeginLogTx is a convenience function to start a log transaction and fail a test on error.
func BeginLogTx(s storage.LogStorage, logID int64, t *testing.T) storage.LogTreeTX {
	tx, err := s.BeginForTree(context.Background(), logID)
	if err != nil {
		t.Fatalf("Failed to begin log tx: %v", err)
	}
	return tx
}

// CommittableTX is an interface for objects that support commit.
type CommittableTX interface {
	Commit() error
}

// Commit commits a supplied Committable and fails a test if this does not succeed.
func Commit(tx CommittableTX, t *testing.T) {
	if err := tx.Commit(); err != nil {
		t.Errorf("Failed to commit tx: %v", err)
	}
}
