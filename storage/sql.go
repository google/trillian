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

package storage

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keyspb"
	spb "github.com/google/trillian/crypto/sigpb"
)

// ToMillisSinceEpoch converts a timestamp into milliseconds since epoch
func ToMillisSinceEpoch(t time.Time) int64 {
	return t.UnixNano() / 1000000
}

// FromMillisSinceEpoch converts
func FromMillisSinceEpoch(ts int64) time.Time {
	return time.Unix(0, ts*1000000)
}

// SetNullStringIfValid assigns src to dest if src is Valid.
func SetNullStringIfValid(src sql.NullString, dest *string) {
	if src.Valid {
		*dest = src.String
	}
}

// Row defines a common interface between sql.Row and sql.Rows(!)
type Row interface {
	Scan(dest ...interface{}) error
}

// ReadTree takes a sql row and returns a tree
func ReadTree(row Row) (*trillian.Tree, error) {
	tree := &trillian.Tree{}

	// Enums and Datetimes need an extra conversion step
	var treeState, treeType, hashStrategy, hashAlgorithm, signatureAlgorithm string
	var createMillis, updateMillis, maxRootDurationMillis int64
	var displayName, description sql.NullString
	var privateKey, publicKey []byte
	var deleted sql.NullBool
	var deleteMillis sql.NullInt64
	err := row.Scan(
		&tree.TreeId,
		&treeState,
		&treeType,
		&hashStrategy,
		&hashAlgorithm,
		&signatureAlgorithm,
		&displayName,
		&description,
		&createMillis,
		&updateMillis,
		&privateKey,
		&publicKey,
		&maxRootDurationMillis,
		&deleted,
		&deleteMillis,
	)
	if err != nil {
		return nil, err
	}

	SetNullStringIfValid(displayName, &tree.DisplayName)
	SetNullStringIfValid(description, &tree.Description)

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
	if hs, ok := trillian.HashStrategy_value[hashStrategy]; ok {
		tree.HashStrategy = trillian.HashStrategy(hs)
	} else {
		return nil, fmt.Errorf("unknown HashStrategy: %v", hashStrategy)
	}
	if ha, ok := spb.DigitallySigned_HashAlgorithm_value[hashAlgorithm]; ok {
		tree.HashAlgorithm = spb.DigitallySigned_HashAlgorithm(ha)
	} else {
		return nil, fmt.Errorf("unknown HashAlgorithm: %v", hashAlgorithm)
	}
	if sa, ok := spb.DigitallySigned_SignatureAlgorithm_value[signatureAlgorithm]; ok {
		tree.SignatureAlgorithm = spb.DigitallySigned_SignatureAlgorithm(sa)
	} else {
		return nil, fmt.Errorf("unknown SignatureAlgorithm: %v", signatureAlgorithm)
	}

	// Let's make sure we didn't mismatch any of the casts above
	ok := tree.TreeState.String() == treeState &&
		tree.TreeType.String() == treeType &&
		tree.HashStrategy.String() == hashStrategy &&
		tree.HashAlgorithm.String() == hashAlgorithm &&
		tree.SignatureAlgorithm.String() == signatureAlgorithm
	if !ok {
		return nil, fmt.Errorf(
			"mismatched enum: tree = %v, enums = [%v, %v, %v, %v, %v]",
			tree,
			treeState, treeType, hashStrategy, hashAlgorithm, signatureAlgorithm)
	}

	tree.CreateTime, err = ptypes.TimestampProto(FromMillisSinceEpoch(createMillis))
	if err != nil {
		return nil, fmt.Errorf("failed to parse create time: %w", err)
	}
	tree.UpdateTime, err = ptypes.TimestampProto(FromMillisSinceEpoch(updateMillis))
	if err != nil {
		return nil, fmt.Errorf("failed to parse update time: %w", err)
	}
	tree.MaxRootDuration = ptypes.DurationProto(time.Duration(maxRootDurationMillis * int64(time.Millisecond)))

	tree.PrivateKey = &any.Any{}
	if err := proto.Unmarshal(privateKey, tree.PrivateKey); err != nil {
		return nil, fmt.Errorf("could not unmarshal PrivateKey: %w", err)
	}
	tree.PublicKey = &keyspb.PublicKey{Der: publicKey}

	tree.Deleted = deleted.Valid && deleted.Bool
	if tree.Deleted && deleteMillis.Valid {
		tree.DeleteTime, err = ptypes.TimestampProto(FromMillisSinceEpoch(deleteMillis.Int64))
		if err != nil {
			return nil, fmt.Errorf("failed to parse delete time: %w", err)
		}
	}

	return tree, nil
}
