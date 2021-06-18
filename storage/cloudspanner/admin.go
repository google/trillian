// Copyright 2018 Google LLC. All Rights Reserved.
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

package cloudspanner

import (
	"context"
	"fmt"
	"sync"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/cloudspanner/spannerpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	// NumUnseqBuckets is the length of the unsequenced time ring buffer.
	NumUnseqBuckets = int64(4)
	// NumMerkleBuckets is the number of individual buckets below each unsequenced ring buffer.
	NumMerkleBuckets = int64(16)
	// TimeNow is the function used to get the current time. Exposed so it may be mocked by tests.
	TimeNow = time.Now

	treeStateMap = map[trillian.TreeState]spannerpb.TreeState{
		trillian.TreeState_ACTIVE: spannerpb.TreeState_ACTIVE,
		trillian.TreeState_FROZEN: spannerpb.TreeState_FROZEN,
	}
	treeTypeMap = map[trillian.TreeType]spannerpb.TreeType{
		trillian.TreeType_LOG:            spannerpb.TreeType_LOG,
		trillian.TreeType_PREORDERED_LOG: spannerpb.TreeType_PREORDERED_LOG,
	}

	treeStateReverseMap = reverseTreeStateMap(treeStateMap)
	treeTypeReverseMap  = reverseTreeTypeMap(treeTypeMap)
)

const nanosPerMilli = int64(time.Millisecond / time.Nanosecond)

func reverseTreeStateMap(m map[trillian.TreeState]spannerpb.TreeState) map[spannerpb.TreeState]trillian.TreeState {
	reverse := make(map[spannerpb.TreeState]trillian.TreeState)
	for k, v := range m {
		if x, ok := reverse[v]; ok {
			glog.Fatalf("Duplicate values for key %v: %v and %v", v, x, k)
		}
		reverse[v] = k
	}
	return reverse
}

func reverseTreeTypeMap(m map[trillian.TreeType]spannerpb.TreeType) map[spannerpb.TreeType]trillian.TreeType {
	reverse := make(map[spannerpb.TreeType]trillian.TreeType)
	for k, v := range m {
		if x, ok := reverse[v]; ok {
			glog.Fatalf("Duplicate values for key %v: %v and %v", v, x, k)
		}
		reverse[v] = k
	}
	return reverse
}

// adminTX implements both storage.ReadOnlyAdminTX and storage.AdminTX.
type adminTX struct {
	client *spanner.Client

	// mu guards tx, but it's only actively used for Commit/Close. In other
	// scenarios we trust Spanner to blow up if you try to use a closed tx.
	//
	// Note that, if tx is a spanner.SnapshotTransaction, it'll be set to nil
	// when adminTX is closed.
	mu sync.RWMutex

	// tx is either spanner.ReadOnlyTransaction or spanner.ReadWriteTransaction,
	// according to the role adminTX is meant to fill.
	//
	// If tx is a snapshot transaction it'll be set to nil when adminTX is closed
	// to avoid reuse.
	tx spanRead
}

// adminStorage implements storage.AdminStorage.
type adminStorage struct {
	client *spanner.Client
}

// NewAdminStorage returns a Spanner-based storage.AdminStorage implementation.
func NewAdminStorage(client *spanner.Client) storage.AdminStorage {
	return &adminStorage{client}
}

// CheckDatabaseAccessible implements AdminStorage.CheckDatabaseAccessible.
func (s *adminStorage) CheckDatabaseAccessible(ctx context.Context) error {
	return checkDatabaseAccessible(ctx, s.client)
}

// Snapshot implements AdminStorage.Snapshot.
func (s *adminStorage) Snapshot(ctx context.Context) (storage.ReadOnlyAdminTX, error) {
	tx := s.client.ReadOnlyTransaction()
	return &adminTX{client: s.client, tx: tx}, nil
}

// Begin implements AdminStorage.Begin.
func (s *adminStorage) Begin(ctx context.Context) (storage.AdminTX, error) {
	return nil, ErrNotImplemented
}

// ReadWriteTransaction implements AdminStorage.ReadWriteTransaction.
func (s *adminStorage) ReadWriteTransaction(ctx context.Context, f storage.AdminTXFunc) error {
	_, err := s.client.ReadWriteTransaction(ctx, func(ctx context.Context, stx *spanner.ReadWriteTransaction) error {
		tx := &adminTX{client: s.client, tx: stx}
		return f(ctx, tx)
	})
	return err
}

// Commit implements ReadOnlyAdminTX.Commit.
func (t *adminTX) Commit() error {
	return t.Close()
}

// Close implements ReadOnlyAdminTX.Close.
func (t *adminTX) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.tx == nil {
		return nil
	}
	// tx will be committed by ReadWriteTransaction(), so only close readonly tx here
	if stx, ok := t.tx.(*spanner.ReadOnlyTransaction); ok {
		glog.V(1).Infof("Closed admin %p", stx)
		stx.Close()
	}
	t.tx = nil
	return nil
}

// GetTree implements AdminReader.GetTree.
func (t *adminTX) GetTree(ctx context.Context, treeID int64) (*trillian.Tree, error) {
	info, err := t.getTreeInfo(ctx, treeID)
	if err != nil {
		return nil, err
	}
	return toTrillianTree(info)
}

func (t *adminTX) getTreeInfo(ctx context.Context, treeID int64) (*spannerpb.TreeInfo, error) {
	cols := []string{
		"TreeID",
		"TreeState",
		"TreeType",
		"TreeInfo",
		"Deleted",
		"DeleteTimeMillis",
	}

	row, err := t.tx.ReadRow(ctx, "TreeRoots", spanner.Key{treeID}, cols)
	switch {
	case spanner.ErrCode(err) == codes.NotFound:
		// Improve on the error message
		return nil, status.Errorf(codes.NotFound, "tree %v not found", treeID)
	case err != nil:
		return nil, err
	}

	info := &spannerpb.TreeInfo{}
	var infoBytes []byte
	var tID, tState, tType int64
	var deleted bool
	var delMillis spanner.NullInt64
	if err := row.Columns(
		&tID,
		&tState, // info.TreeState,
		&tType,  // info.TreeType,
		&infoBytes,
		&deleted,
		&delMillis,
	); err != nil {
		return nil, err
	}

	if infoBytes != nil {
		if err := proto.Unmarshal(infoBytes, info); err != nil {
			return nil, err
		}
	}
	if tID != info.TreeId {
		return nil, fmt.Errorf("inconsistency, treeIDs don't match: %d != %d", tID, info.TreeId)
	}
	if treeID != tID {
		return nil, fmt.Errorf("inconsistency, got treeID %d, want %d", tID, treeID)
	}
	// TODO(al): check other denormalisations are consistent too.

	// Sanity checks
	switch tt := info.TreeType; tt {
	case spannerpb.TreeType_PREORDERED_LOG:
		fallthrough
	case spannerpb.TreeType_LOG:
		if info.GetLogStorageConfig() == nil {
			return nil, status.Errorf(codes.Internal, "corrupt TreeInfo %#v: LogStorageConfig is nil", treeID)
		}
	default:
		return nil, status.Errorf(codes.Internal, "corrupt TreeInfo %#v: unexpected TreeType = %s", treeID, tt)
	}

	return info, nil
}

// ListTrees implements AdminReader.ListTrees.
func (t *adminTX) ListTrees(ctx context.Context, includeDeleted bool) ([]*trillian.Tree, error) {
	trees := []*trillian.Tree{}
	err := t.readTrees(ctx, includeDeleted, false /* idOnly */, func(r *spanner.Row) error {
		var infoBytes []byte
		if err := r.Columns(&infoBytes); err != nil {
			return err
		}
		info := &spannerpb.TreeInfo{}
		if err := proto.Unmarshal(infoBytes, info); err != nil {
			return err
		}
		tree, err := toTrillianTree(info)
		if err != nil {
			return err
		}
		trees = append(trees, tree)
		return nil
	})
	return trees, err
}

func (t *adminTX) readTrees(ctx context.Context, includeDeleted, idOnly bool, f func(*spanner.Row) error) error {
	var stmt spanner.Statement
	if idOnly {
		stmt = spanner.NewStatement("SELECT t.TreeID FROM TreeRoots t")
	} else {
		stmt = spanner.NewStatement("SELECT t.TreeInfo FROM TreeRoots t")
	}
	if !includeDeleted {
		stmt.SQL += " WHERE t.Deleted = @deleted"
		stmt.Params["deleted"] = false
	}
	rows := t.tx.Query(ctx, stmt)
	return rows.Do(f)
}

// CreateTree implements AdminWriter.CreateTree.
func (t *adminTX) CreateTree(ctx context.Context, tree *trillian.Tree) (*trillian.Tree, error) {
	if err := storage.ValidateTreeForCreation(ctx, tree); err != nil {
		return nil, err
	}

	id, err := storage.NewTreeID()
	if err != nil {
		return nil, err
	}

	info, err := newTreeInfo(tree, id, TimeNow())
	if err != nil {
		return nil, err
	}

	infoBytes, err := proto.Marshal(info)
	if err != nil {
		return nil, err
	}

	m1 := spanner.Insert(
		"TreeRoots",
		[]string{
			"TreeID",
			"TreeState",
			"TreeType",
			"TreeInfo",
			"Deleted",
		},
		[]interface{}{
			info.TreeId,
			int64(info.TreeState),
			int64(info.TreeType),
			infoBytes,
			false,
		})

	stx, ok := t.tx.(*spanner.ReadWriteTransaction)
	if !ok {
		return nil, ErrWrongTXType
	}
	if err := stx.BufferWrite([]*spanner.Mutation{m1}); err != nil {
		return nil, err
	}
	return toTrillianTree(info)
}

// newTreeInfo creates a new TreeInfo from a Tree. Meant to be used for new trees.
func newTreeInfo(tree *trillian.Tree, treeID int64, now time.Time) (*spannerpb.TreeInfo, error) {
	ts, ok := treeStateMap[tree.TreeState]
	if !ok {
		return nil, status.Errorf(codes.Internal, "unexpected TreeState: %s", tree.TreeState)
	}

	tt, ok := treeTypeMap[tree.TreeType]
	if !ok {
		return nil, status.Errorf(codes.Internal, "unexpected TreeType: %s", tree.TreeType)
	}

	if err := tree.MaxRootDuration.CheckValid(); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "malformed MaxRootDuration: %v", err)
	}
	maxRootDuration := tree.MaxRootDuration.AsDuration()

	info := &spannerpb.TreeInfo{
		TreeId:                treeID,
		Name:                  tree.DisplayName,
		Description:           tree.Description,
		TreeState:             ts,
		TreeType:              tt,
		CreateTimeNanos:       now.UnixNano(),
		UpdateTimeNanos:       now.UnixNano(),
		MaxRootDurationMillis: int64(maxRootDuration / time.Millisecond),
	}

	switch tt := tree.TreeType; tt {
	case trillian.TreeType_PREORDERED_LOG:
		fallthrough
	case trillian.TreeType_LOG:
		config, err := logConfigOrDefault(tree)
		if err != nil {
			return nil, err
		}
		if err := validateLogStorageConfig(config); err != nil {
			return nil, err
		}
		info.StorageConfig = &spannerpb.TreeInfo_LogStorageConfig{LogStorageConfig: config}
	default:
		return nil, fmt.Errorf("Unknown tree type %v", tt)
	}

	return info, nil
}

func logConfigOrDefault(tree *trillian.Tree) (*spannerpb.LogStorageConfig, error) {
	settings, err := unmarshalSettings(tree)
	if err != nil {
		return nil, err
	}
	if settings == nil {
		return &spannerpb.LogStorageConfig{
			NumUnseqBuckets:  NumUnseqBuckets,
			NumMerkleBuckets: NumMerkleBuckets,
		}, nil
	}
	config, ok := settings.(*spannerpb.LogStorageConfig)
	if !ok {
		return nil, status.Errorf(codes.Internal, "unsupported config type for LOG tree: %T", settings)
	}
	return config, nil
}

// UpdateTree implements AdminWriter.UpdateTree.
func (t *adminTX) UpdateTree(ctx context.Context, treeID int64, updateFunc func(*trillian.Tree)) (*trillian.Tree, error) {
	info, err := t.getTreeInfo(ctx, treeID)
	if err != nil {
		return nil, err
	}

	tree, err := toTrillianTree(info)
	if err != nil {
		return nil, err
	}
	beforeTree := proto.Clone(tree).(*trillian.Tree)
	updateFunc(tree)
	if err = storage.ValidateTreeForUpdate(ctx, beforeTree, tree); err != nil {
		return nil, err
	}
	if !proto.Equal(beforeTree.StorageSettings, tree.StorageSettings) {
		return nil, status.New(codes.InvalidArgument, "readonly field changed: storage_settings").Err()
	}

	ts, ok := treeStateMap[tree.TreeState]
	if !ok {
		return nil, status.Errorf(codes.Internal, "unexpected TreeState: %s", tree.TreeState)
	}

	if err := tree.MaxRootDuration.CheckValid(); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "malformed MaxRootDuration: %v", err)
	}
	maxRootDuration := tree.MaxRootDuration.AsDuration()

	// Update (just) the mutable fields in treeInfo.
	now := TimeNow()
	info.TreeState = ts
	info.Name = tree.DisplayName
	info.Description = tree.Description
	info.UpdateTimeNanos = now.UnixNano()
	info.MaxRootDurationMillis = int64(maxRootDuration / time.Millisecond)

	if err := t.updateTreeInfo(ctx, info); err != nil {
		return nil, err
	}

	return toTrillianTree(info)
}

func (t *adminTX) updateTreeInfo(ctx context.Context, info *spannerpb.TreeInfo) error {
	infoBytes, err := proto.Marshal(info)
	if err != nil {
		return err
	}

	m1 := spanner.Update(
		"TreeRoots",
		[]string{
			"TreeID",
			"TreeState",
			"TreeType",
			"TreeInfo",
			"Deleted",
			"DeleteTimeMillis",
		},
		[]interface{}{
			info.TreeId,
			int64(info.TreeState),
			int64(info.TreeType),
			infoBytes,
			info.Deleted,
			info.DeleteTimeNanos / nanosPerMilli,
		})

	stx, ok := t.tx.(*spanner.ReadWriteTransaction)
	if !ok {
		return ErrWrongTXType
	}
	return stx.BufferWrite([]*spanner.Mutation{m1})
}

// SoftDeleteTree implements AdminWriter.SoftDeleteTree.
func (t *adminTX) SoftDeleteTree(ctx context.Context, treeID int64) (*trillian.Tree, error) {
	info, err := t.getTreeInfo(ctx, treeID)
	if err != nil {
		return nil, err
	}
	if info.Deleted {
		return nil, status.Errorf(codes.FailedPrecondition, "tree %v already soft deleted", treeID)
	}

	info.Deleted = true
	info.DeleteTimeNanos = TimeNow().UnixNano()
	if err := t.updateTreeInfo(ctx, info); err != nil {
		return nil, err
	}

	return toTrillianTree(info)
}

// HardDeleteTree implements AdminWriter.HardDeleteTree.
func (t *adminTX) HardDeleteTree(ctx context.Context, treeID int64) error {
	info, err := t.getTreeInfo(ctx, treeID)
	if err != nil {
		return err
	}
	if !info.Deleted {
		return status.Errorf(codes.FailedPrecondition, "tree %v is not soft deleted", treeID)
	}

	stx, ok := t.tx.(*spanner.ReadWriteTransaction)
	if !ok {
		return ErrWrongTXType
	}

	// Due to cloud spanner sizing recommendations, we don't interleave our tables
	// which means no ON DELETE CASCADE goodies for us, so we have to
	// transactionally delete related data from all tables.
	return stx.BufferWrite([]*spanner.Mutation{
		spanner.Delete("TreeRoots", spanner.Key{info.TreeId}),
		spanner.Delete("TreeHeads", spanner.Key{info.TreeId}.AsPrefix()),
		spanner.Delete("SubtreeData", spanner.Key{info.TreeId}.AsPrefix()),
		spanner.Delete("LeafData", spanner.Key{info.TreeId}.AsPrefix()),
		spanner.Delete("SequencedLeafData", spanner.Key{info.TreeId}.AsPrefix()),
		spanner.Delete("Unsequenced", spanner.Key{info.TreeId}.AsPrefix()),
	})
}

// UndeleteTree implements AdminWriter.UndeleteTree.
func (t *adminTX) UndeleteTree(ctx context.Context, treeID int64) (*trillian.Tree, error) {
	info, err := t.getTreeInfo(ctx, treeID)
	if err != nil {
		return nil, err
	}
	if !info.Deleted {
		return nil, status.Errorf(codes.FailedPrecondition, "tree %v is not soft deleted", treeID)
	}

	info.Deleted = false
	info.DeleteTimeNanos = 0
	if err := t.updateTreeInfo(ctx, info); err != nil {
		return nil, err
	}

	return toTrillianTree(info)
}

func toTrillianTree(info *spannerpb.TreeInfo) (*trillian.Tree, error) {
	createdPB := timestamppb.New(time.Unix(0, info.CreateTimeNanos))
	updatedPB := timestamppb.New(time.Unix(0, info.UpdateTimeNanos))
	if err := createdPB.CheckValid(); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to convert creation time: %v", err)
	}
	if err := updatedPB.CheckValid(); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to convert update time: %v", err)
	}
	tree := &trillian.Tree{
		TreeId:          info.TreeId,
		DisplayName:     info.Name,
		Description:     info.Description,
		CreateTime:      createdPB,
		UpdateTime:      updatedPB,
		MaxRootDuration: durationpb.New(time.Duration(info.MaxRootDurationMillis) * time.Millisecond),
	}

	ts, ok := treeStateReverseMap[info.TreeState]
	if !ok {
		return nil, status.Errorf(codes.Internal, "unexpected TreeState: %s", info.TreeState)
	}
	tree.TreeState = ts

	tt, ok := treeTypeReverseMap[info.TreeType]
	if !ok {
		return nil, status.Errorf(codes.Internal, "unexpected TreeType: %s", info.TreeType)
	}
	tree.TreeType = tt

	var config proto.Message
	switch tt := info.TreeType; tt {
	case spannerpb.TreeType_PREORDERED_LOG:
		fallthrough
	case spannerpb.TreeType_LOG:
		config = info.GetLogStorageConfig()
	default:
		return nil, fmt.Errorf("unknown tree type %v", tt)
	}
	settings, err := anypb.New(config)
	if err != nil {
		return nil, fmt.Errorf("anypb.New(): %w", err)
	}
	tree.StorageSettings = settings

	if info.Deleted {
		tree.Deleted = info.Deleted
	}
	if info.DeleteTimeNanos > 0 {
		tree.DeleteTime = timestamppb.New(time.Unix(0, info.DeleteTimeNanos))
		if err := tree.DeleteTime.CheckValid(); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to convert delete time: %v", err)
		}
	}

	return tree, nil
}

// unmarshalSettings returns the message obtained from tree.StorageSettings.
// If tree.StorageSettings is nil no unmarshaling will be attempted; instead the method will return
// (nil, nil).
func unmarshalSettings(tree *trillian.Tree) (proto.Message, error) {
	settings := tree.GetStorageSettings()
	if settings == nil {
		return nil, nil
	}
	any, err := settings.UnmarshalNew()
	if err != nil {
		return nil, err
	}
	return any, nil
}

func validateLogStorageConfig(config *spannerpb.LogStorageConfig) error {
	if config.NumUnseqBuckets < 1 {
		return status.Errorf(codes.InvalidArgument, "NumUnseqBuckets = %v, want > 0", config.NumUnseqBuckets)
	}
	if config.NumMerkleBuckets < 1 || config.NumMerkleBuckets > 256 {
		return status.Errorf(codes.InvalidArgument, "NumMerkleBuckets = %v, want a number in range [1, 256]", config.NumMerkleBuckets)
	}
	return nil
}
