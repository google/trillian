// Copyright 2024 Trillian Authors. All Rights Reserved.
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

package postgresql

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/trillian"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// toMillisSinceEpoch converts a timestamp into milliseconds since epoch
func toMillisSinceEpoch(t time.Time) int64 {
	return t.UnixNano() / 1000000
}

// fromMillisSinceEpoch converts
func fromMillisSinceEpoch(ts int64) time.Time {
	return time.Unix(0, ts*1000000)
}

// setNullStringIfValid assigns src to dest if src is Valid.
func setNullStringIfValid(src sql.NullString, dest *string) {
	if src.Valid {
		*dest = src.String
	}
}

// row defines a common interface between sql.Row and sql.Rows(!)
type row interface {
	Scan(dest ...interface{}) error
}

// readTree takes a sql row and returns a tree
func readTree(r row) (*trillian.Tree, error) {
	tree := &trillian.Tree{}

	// Enums and Datetimes need an extra conversion step
	var treeState, treeType string
	var createMillis, updateMillis, maxRootDurationMillis int64
	var displayName, description sql.NullString
	var deleted sql.NullBool
	var deleteMillis sql.NullInt64
	err := r.Scan(
		&tree.TreeId,
		&treeState,
		&treeType,
		&displayName,
		&description,
		&createMillis,
		&updateMillis,
		&maxRootDurationMillis,
		&deleted,
		&deleteMillis,
	)
	if err != nil {
		return nil, err
	}

	setNullStringIfValid(displayName, &tree.DisplayName)
	setNullStringIfValid(description, &tree.Description)

	// Convert all things!
	if ts, ok := trillian.TreeState_value[treeState]; ok {
		tree.TreeState = trillian.TreeState(ts)
	} else {
		return nil, fmt.Errorf("unknown TreeState: %v", treeState)
	}
	if tt, ok := trillian.TreeType_value[treeType]; ok {
		tree.TreeType = trillian.TreeType(tt)
	} else {
		return nil, fmt.Errorf("unknown TreeType: %v", treeType)
	}

	// Let's make sure we didn't mismatch any of the casts above
	ok := tree.TreeState.String() == treeState &&
		tree.TreeType.String() == treeType
	if !ok {
		return nil, fmt.Errorf(
			"mismatched enum: tree = %v, enums = [%v, %v]",
			tree,
			treeState, treeType)
	}

	tree.CreateTime = timestamppb.New(fromMillisSinceEpoch(createMillis))
	if err := tree.CreateTime.CheckValid(); err != nil {
		return nil, fmt.Errorf("failed to parse create time: %w", err)
	}
	tree.UpdateTime = timestamppb.New(fromMillisSinceEpoch(updateMillis))
	if err := tree.UpdateTime.CheckValid(); err != nil {
		return nil, fmt.Errorf("failed to parse update time: %w", err)
	}
	tree.MaxRootDuration = durationpb.New(time.Duration(maxRootDurationMillis * int64(time.Millisecond)))

	tree.Deleted = deleted.Valid && deleted.Bool
	if tree.Deleted && deleteMillis.Valid {
		tree.DeleteTime = timestamppb.New(fromMillisSinceEpoch(deleteMillis.Int64))
		if err := tree.DeleteTime.CheckValid(); err != nil {
			return nil, fmt.Errorf("failed to parse delete time: %w", err)
		}
	}

	return tree, nil
}
