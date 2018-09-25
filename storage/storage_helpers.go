package storage

import (
	"database/sql"
	"fmt"
	time "time"

	"github.com/gogo/protobuf/proto"
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

// FromMillisSinceEpoch converts an UNIX typestamp to a time struct
func FromMillisSinceEpoch(ts int64) time.Time {
	secs := int64(ts / 1000)
	msecs := int64(ts % 1000)
	return time.Unix(secs, msecs*1000000)
}

// SetNullStringIfValid assigns src to dest if src is Valid.
func SetNullStringIfValid(src sql.NullString, dest *string) {
	if src.Valid {
		*dest = src.String
	}
}

// ValidateStorageSettings checks for storage settings and returns an error if they exist
func ValidateStorageSettings(tree *trillian.Tree) error {
	if tree.StorageSettings != nil {
		return fmt.Errorf("storage_settings not supported, but got %v", tree.StorageSettings)
	}
	return nil
}

// Row defines a common interface between sql.Row and sql.Rows(!), so we have to
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
	ok := tree.TreeState.String() == treeState
	ok = ok && tree.TreeType.String() == treeType
	ok = ok && tree.HashStrategy.String() == hashStrategy
	ok = ok && tree.HashAlgorithm.String() == hashAlgorithm
	ok = ok && tree.SignatureAlgorithm.String() == signatureAlgorithm
	if !ok {
		return nil, fmt.Errorf(
			"mismatched enum: tree = %v, enums = [%v, %v, %v, %v, %v]",
			tree,
			treeState, treeType, hashStrategy, hashAlgorithm, signatureAlgorithm)
	}

	tree.CreateTime, err = ptypes.TimestampProto(FromMillisSinceEpoch(createMillis))
	if err != nil {
		return nil, fmt.Errorf("failed to parse create time: %v", err)
	}
	tree.UpdateTime, err = ptypes.TimestampProto(FromMillisSinceEpoch(updateMillis))
	if err != nil {
		return nil, fmt.Errorf("failed to parse update time: %v", err)
	}
	tree.MaxRootDuration = ptypes.DurationProto(time.Duration(maxRootDurationMillis * int64(time.Millisecond)))

	tree.PrivateKey = &any.Any{}
	if err := proto.Unmarshal(privateKey, tree.PrivateKey); err != nil {
		return nil, fmt.Errorf("could not unmarshal PrivateKey: %v", err)
	}
	tree.PublicKey = &keyspb.PublicKey{Der: publicKey}

	tree.Deleted = deleted.Valid && deleted.Bool
	if tree.Deleted && deleteMillis.Valid {
		tree.DeleteTime, err = ptypes.TimestampProto(FromMillisSinceEpoch(deleteMillis.Int64))
		if err != nil {
			return nil, fmt.Errorf("failed to parse delete time: %v", err)
		}
	}

	return tree, nil
}
