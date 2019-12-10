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
	"crypto"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/crypto/keys/pem"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/testonly"
	"github.com/kylelemons/godebug/pretty"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ktestonly "github.com/google/trillian/crypto/keys/testonly"
	spb "github.com/google/trillian/crypto/sigpb"

	_ "github.com/google/trillian/crypto/keys/der/proto" // PrivateKey proto handler
	_ "github.com/google/trillian/crypto/keys/pem/proto" // PEMKeyFile proto handler
	_ "github.com/google/trillian/merkle/maphasher"      // TEST_MAP_HASHER
)

const (
	privateKeyPass = "towel"
	privateKeyPEM  = `
-----BEGIN EC PRIVATE KEY-----
Proc-Type: 4,ENCRYPTED
DEK-Info: DES-CBC,D95ECC664FF4BDEC

Xy3zzHFwlFwjE8L1NCngJAFbu3zFf4IbBOCsz6Fa790utVNdulZncNCl2FMK3U2T
sdoiTW8ymO+qgwcNrqvPVmjFRBtkN0Pn5lgbWhN/aK3TlS9IYJ/EShbMUzjgVzie
S9+/31whWcH/FLeLJx4cBzvhgCtfquwA+s5ojeLYYsk=
-----END EC PRIVATE KEY-----`
	// PublicKeyPEM is the public key for: LogTree and PreorderedLogTree
	PublicKeyPEM = `
-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEywnWicNEQ8bn3GXcGpA+tiU4VL70
Ws9xezgQPrg96YGsFrF6KYG68iqyHDlQ+4FWuKfGKXHn3ooVtB/pfawb5Q==
-----END PUBLIC KEY-----`
)

// mustMarshalAny panics if ptypes.MarshalAny fails.
func mustMarshalAny(pb proto.Message) *any.Any {
	value, err := ptypes.MarshalAny(pb)
	if err != nil {
		panic(err)
	}
	return value
}

var (
	// LogTree is a valid, LOG-type trillian.Tree for tests.
	LogTree = &trillian.Tree{
		TreeState:          trillian.TreeState_ACTIVE,
		TreeType:           trillian.TreeType_LOG,
		HashStrategy:       trillian.HashStrategy_RFC6962_SHA256,
		HashAlgorithm:      spb.DigitallySigned_SHA256,
		SignatureAlgorithm: spb.DigitallySigned_ECDSA,
		DisplayName:        "Llamas Log",
		Description:        "Registry of publicly-owned llamas",
		PrivateKey: mustMarshalAny(&keyspb.PrivateKey{
			Der: ktestonly.MustMarshalPrivatePEMToDER(privateKeyPEM, privateKeyPass),
		}),
		PublicKey: &keyspb.PublicKey{
			Der: ktestonly.MustMarshalPublicPEMToDER(PublicKeyPEM),
		},
		MaxRootDuration: ptypes.DurationProto(0 * time.Millisecond),
	}

	// PreorderedLogTree is a valid, PREORDERED_LOG-type trillian.Tree for tests.
	PreorderedLogTree = &trillian.Tree{
		TreeState:          trillian.TreeState_ACTIVE,
		TreeType:           trillian.TreeType_PREORDERED_LOG,
		HashStrategy:       trillian.HashStrategy_RFC6962_SHA256,
		HashAlgorithm:      spb.DigitallySigned_SHA256,
		SignatureAlgorithm: spb.DigitallySigned_ECDSA,
		DisplayName:        "Pre-ordered Log",
		Description:        "Mirror registry of publicly-owned llamas",
		PrivateKey: mustMarshalAny(&keyspb.PrivateKey{
			Der: ktestonly.MustMarshalPrivatePEMToDER(privateKeyPEM, privateKeyPass),
		}),
		PublicKey: &keyspb.PublicKey{
			Der: ktestonly.MustMarshalPublicPEMToDER(PublicKeyPEM),
		},
		MaxRootDuration: ptypes.DurationProto(0 * time.Millisecond),
	}

	// MapTree is a valid, MAP-type trillian.Tree for tests.
	MapTree = &trillian.Tree{
		TreeState:          trillian.TreeState_ACTIVE,
		TreeType:           trillian.TreeType_MAP,
		HashStrategy:       trillian.HashStrategy_TEST_MAP_HASHER,
		HashAlgorithm:      spb.DigitallySigned_SHA256,
		SignatureAlgorithm: spb.DigitallySigned_ECDSA,
		DisplayName:        "Llamas Map",
		Description:        "Key Transparency map for all your digital llama needs.",
		PrivateKey: mustMarshalAny(&keyspb.PrivateKey{
			Der: ktestonly.MustMarshalPrivatePEMToDER(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass),
		}),
		PublicKey: &keyspb.PublicKey{
			Der: ktestonly.MustMarshalPublicPEMToDER(testonly.DemoPublicKey),
		},
		MaxRootDuration: ptypes.DurationProto(0 * time.Millisecond),
	}
)

// AdminStorageTester runs a suite of tests against AdminStorage implementations.
type AdminStorageTester struct {
	// NewAdminStorage returns an AdminStorage instance pointing to a clean
	// test database.
	NewAdminStorage func() storage.AdminStorage
}

// RunAllTests runs all AdminStorage tests.
func (tester *AdminStorageTester) RunAllTests(t *testing.T) {
	t.Run("TestCreateTree", tester.TestCreateTree)
	t.Run("TestUpdateTree", tester.TestUpdateTree)
	t.Run("TestListTrees", tester.TestListTrees)
	t.Run("TestSoftDeleteTree", tester.TestSoftDeleteTree)
	t.Run("TestSoftDeleteTreeErrors", tester.TestSoftDeleteTreeErrors)
	t.Run("TestHardDeleteTree", tester.TestHardDeleteTree)
	t.Run("TestHardDeleteTreeErrors", tester.TestHardDeleteTreeErrors)
	t.Run("TestUndeleteTree", tester.TestUndeleteTree)
	t.Run("TestUndeleteTreeErrors", tester.TestUndeleteTreeErrors)
	t.Run("TestAdminTXReadWriteTransaction", tester.TestAdminTXReadWriteTransaction)
}

// TestCreateTree tests AdminStorage Tree creation.
func (tester *AdminStorageTester) TestCreateTree(t *testing.T) {
	// Check that validation runs, but leave details to the validation
	// tests.
	invalidTree := proto.Clone(LogTree).(*trillian.Tree)
	invalidTree.TreeType = trillian.TreeType_UNKNOWN_TREE_TYPE

	validTree1 := proto.Clone(LogTree).(*trillian.Tree)
	validTree2 := proto.Clone(MapTree).(*trillian.Tree)
	validTree3 := proto.Clone(PreorderedLogTree).(*trillian.Tree)

	validTreeWithoutOptionals := proto.Clone(LogTree).(*trillian.Tree)
	validTreeWithoutOptionals.DisplayName = ""
	validTreeWithoutOptionals.Description = ""

	tests := []struct {
		desc    string
		tree    *trillian.Tree
		wantErr bool
	}{
		{
			desc:    "invalidTree",
			tree:    invalidTree,
			wantErr: true,
		},
		{
			desc: "validTree1",
			tree: validTree1,
		},
		{
			desc: "validTree2",
			tree: validTree2,
		},
		{
			desc: "validTree3",
			tree: validTree3,
		},
		{
			desc: "validTreeWithoutOptionals",
			tree: validTreeWithoutOptionals,
		},
	}

	ctx := context.Background()
	s := tester.NewAdminStorage()
	for _, test := range tests {
		func() {
			// Test CreateTree up to the tx commit
			newTree, err := storage.CreateTree(ctx, s, test.tree)
			if hasErr := err != nil; hasErr != test.wantErr {
				t.Errorf("%v: CreateTree() = (_, %v), wantErr = %v", test.desc, err, test.wantErr)
				return
			} else if hasErr {
				// Tested above
				return
			}

			createTime := newTree.CreateTime
			updateTime := newTree.UpdateTime
			if _, err := ptypes.Timestamp(createTime); err != nil {
				t.Errorf("%v: CreateTime malformed after creation: %v", test.desc, newTree)
				return
			}

			switch {
			case newTree.TreeId == 0:
				t.Errorf("%v: TreeID not returned from creation: %v", test.desc, newTree)
				return
			case !reflect.DeepEqual(createTime, updateTime):
				t.Errorf("%v: CreateTime != UpdateTime: %v", test.desc, newTree)
				return
			}

			wantTree := proto.Clone(test.tree).(*trillian.Tree)
			wantTree.TreeId = newTree.TreeId
			wantTree.CreateTime = createTime
			wantTree.UpdateTime = updateTime
			// Ignore storage_settings changes (OK to vary between implementations)
			wantTree.StorageSettings = newTree.StorageSettings
			if !proto.Equal(newTree, wantTree) {
				diff := pretty.Compare(newTree, &wantTree)
				t.Errorf("%v: post-CreateTree diff:\n%v", test.desc, diff)
				return
			}

			if err := assertStoredTree(ctx, s, newTree); err != nil {
				t.Errorf("%v: %v", test.desc, err)
			}
		}()
	}
}

// TestUpdateTree tests AdminStorage Tree updates.
func (tester *AdminStorageTester) TestUpdateTree(t *testing.T) {
	ctx := context.Background()
	s := tester.NewAdminStorage()

	unrelatedTree := makeTreeOrFail(ctx, s, spec{Tree: MapTree}, t.Fatalf)

	referenceLog := proto.Clone(LogTree).(*trillian.Tree)
	validLog := proto.Clone(referenceLog).(*trillian.Tree)
	validLog.TreeState = trillian.TreeState_FROZEN
	validLog.DisplayName = "Frozen Tree"
	validLog.Description = "A Frozen Tree"
	validLogFunc := func(tree *trillian.Tree) {
		tree.TreeState = validLog.TreeState
		tree.DisplayName = validLog.DisplayName
		tree.Description = validLog.Description
	}

	validLogWithoutOptionalsFunc := func(tree *trillian.Tree) {
		tree.DisplayName = ""
		tree.Description = ""
	}
	validLogWithoutOptionals := proto.Clone(referenceLog).(*trillian.Tree)
	validLogWithoutOptionalsFunc(validLogWithoutOptionals)

	invalidLogFunc := func(tree *trillian.Tree) {
		tree.TreeState = trillian.TreeState_UNKNOWN_TREE_STATE
	}

	readonlyChangedFunc := func(tree *trillian.Tree) {
		tree.TreeType = trillian.TreeType_MAP
	}

	referenceMap := proto.Clone(MapTree).(*trillian.Tree)
	validMap := proto.Clone(referenceMap).(*trillian.Tree)
	validMap.DisplayName = "Updated Map"
	validMapFunc := func(tree *trillian.Tree) {
		tree.DisplayName = validMap.DisplayName
	}

	newPrivateKey := &empty.Empty{}
	privateKeyChangedButKeyMaterialSameTree := tweakedCopy(LogTree, func(tree *trillian.Tree) {
		tree.PrivateKey = testonly.MustMarshalAny(t, newPrivateKey)
	})
	keys.RegisterHandler(newPrivateKey, func(ctx context.Context, pb proto.Message) (crypto.Signer, error) {
		return pem.UnmarshalPrivateKey(privateKeyPEM, privateKeyPass)
	})
	defer keys.UnregisterHandler(newPrivateKey)

	privateKeyChangedButKeyMaterialSameFunc := func(tree *trillian.Tree) {
		tree.PrivateKey = privateKeyChangedButKeyMaterialSameTree.PrivateKey
	}

	privateKeyChangedAndKeyMaterialDifferentFunc := func(tree *trillian.Tree) {
		tree.PrivateKey = testonly.MustMarshalAny(t, &keyspb.PrivateKey{
			Der: ktestonly.MustMarshalPrivatePEMToDER(testonly.DemoPrivateKey, testonly.DemoPrivateKeyPass),
		})
	}

	// Test for an unknown tree outside the loop: it makes the test logic simpler
	if _, err := storage.UpdateTree(ctx, s, -1, func(tree *trillian.Tree) {}); err == nil {
		t.Error("UpdateTree() for treeID -1 returned nil err")
	}

	tests := []struct {
		desc         string
		create, want *trillian.Tree
		updateFunc   func(*trillian.Tree)
		wantErr      bool
	}{
		{
			desc:       "validLog",
			create:     referenceLog,
			updateFunc: validLogFunc,
			want:       validLog,
		},
		{
			desc:       "validLogWithoutOptionals",
			create:     referenceLog,
			updateFunc: validLogWithoutOptionalsFunc,
			want:       validLogWithoutOptionals,
		},
		{
			desc:       "invalidLog",
			create:     referenceLog,
			updateFunc: invalidLogFunc,
			wantErr:    true,
		},
		{
			desc:       "readonlyChanged",
			create:     referenceLog,
			updateFunc: readonlyChangedFunc,
			wantErr:    true,
		},
		{
			desc:       "validMap",
			create:     referenceMap,
			updateFunc: validMapFunc,
			want:       validMap,
		},
		{
			desc:       "privateKeyChangedButKeyMaterialSame",
			create:     referenceLog,
			updateFunc: privateKeyChangedButKeyMaterialSameFunc,
			want:       privateKeyChangedButKeyMaterialSameTree,
		},
		{
			desc:       "privateKeyChangedAndKeyMaterialDifferent",
			create:     referenceLog,
			updateFunc: privateKeyChangedAndKeyMaterialDifferentFunc,
			wantErr:    true,
		},
	}
	for _, test := range tests {
		createdTree, err := storage.CreateTree(ctx, s, test.create)
		if err != nil {
			t.Errorf("CreateTree() = (_, %v), want = (_, nil)", err)
			continue
		}

		updatedTree, err := storage.UpdateTree(ctx, s, createdTree.TreeId, test.updateFunc)
		if hasErr := err != nil; hasErr != test.wantErr {
			t.Errorf("%v: UpdateTree() = (_, %v), wantErr = %v", test.desc, err, test.wantErr)
			continue
		} else if hasErr {
			continue
		}

		if createdTree.TreeId != updatedTree.TreeId {
			t.Errorf("%v: TreeId = %v, want = %v", test.desc, updatedTree.TreeId, createdTree.TreeId)
		}
		if !reflect.DeepEqual(createdTree.CreateTime, updatedTree.CreateTime) {
			t.Errorf("%v: CreateTime = %v, want = %v", test.desc, updatedTree.CreateTime, createdTree.CreateTime)
		}
		createUpdateTime, err := ptypes.Timestamp(createdTree.UpdateTime)
		if err != nil {
			t.Errorf("%v: createdTree.UpdateTime malformed: %v", test.desc, err)
		}
		updatedUpdateTime, err := ptypes.Timestamp(updatedTree.UpdateTime)
		if err != nil {
			t.Errorf("%v: updatedTree.UpdateTime malformed: %v", test.desc, err)
		}
		if createUpdateTime.After(updatedUpdateTime) {
			t.Errorf("%v: UpdateTime = %v, want >= %v", test.desc, updatedTree.UpdateTime, createdTree.UpdateTime)
		}
		// Copy storage-generated values to want before comparing
		wantTree := tweakedCopy(test.want, func(t *trillian.Tree) {
			t.TreeId = updatedTree.TreeId
			t.CreateTime = updatedTree.CreateTime
			t.UpdateTime = updatedTree.UpdateTime
			// Ignore storage_settings changes (OK to vary between implementations)
			t.StorageSettings = updatedTree.StorageSettings
		})
		if !proto.Equal(updatedTree, wantTree) {
			diff := pretty.Compare(updatedTree, wantTree)
			t.Errorf("%v: updatedTree doesn't match wantTree:\n%s", test.desc, diff)
		}

		if err := assertStoredTree(ctx, s, updatedTree); err != nil {
			t.Errorf("%v: %v", test.desc, err)
		}

		if err := assertStoredTree(ctx, s, unrelatedTree); err != nil {
			t.Errorf("%v: %v", test.desc, err)
		}
	}
}

// TestListTrees tests both ListTreeIDs and ListTrees.
func (tester *AdminStorageTester) TestListTrees(t *testing.T) {
	ctx := context.Background()
	s := tester.NewAdminStorage()

	run := func(desc string, includeDeleted bool, wantTrees []*trillian.Tree) {
		if err := storage.RunInAdminSnapshot(ctx, s, func(tx storage.ReadOnlyAdminTX) error {
			if err := runListTreeIDsTest(ctx, tx, includeDeleted, wantTrees); err != nil {
				t.Errorf("%v: %v", desc, err)
			}
			if err := runListTreesTest(ctx, tx, includeDeleted, wantTrees); err != nil {
				t.Errorf("%v: %v", desc, err)
			}
			// Always return nil, as we're reporting errors independently above.
			return nil
		}); err != nil {
			// Capture Begin() / Commit() errors
			t.Errorf("%v: RunInAdminSnapshot() returned err = %v", desc, err)
		}
	}

	// Do a first pass with an empty DB
	run("empty", false /* includeDeleted */, nil /* wantTrees */)
	run("emptyDeleted", true /* includeDeleted */, nil /* wantTrees */)

	// Add some trees and do another pass
	activeLog := makeTreeOrFail(ctx, s, spec{Tree: LogTree}, t.Fatalf)
	frozenLog := makeTreeOrFail(ctx, s, spec{Tree: LogTree, Frozen: true}, t.Fatalf)
	deletedLog := makeTreeOrFail(ctx, s, spec{Tree: LogTree, Deleted: true}, t.Fatalf)
	activeMap := makeTreeOrFail(ctx, s, spec{Tree: MapTree}, t.Fatalf)
	run("multipleTrees", false /* includeDeleted */, []*trillian.Tree{activeLog, frozenLog, activeMap})
	run("multipleTreesDeleted", true /* includeDeleted */, []*trillian.Tree{activeLog, frozenLog, deletedLog, activeMap})
}

func runListTreeIDsTest(ctx context.Context, tx storage.ReadOnlyAdminTX, includeDeleted bool, wantTrees []*trillian.Tree) error {
	got, err := tx.ListTreeIDs(ctx, includeDeleted)
	if err != nil {
		return fmt.Errorf("ListTreeIDs() returned err = %v", err)
	}

	want := make([]int64, 0, len(wantTrees))
	for _, tree := range wantTrees {
		want = append(want, tree.TreeId)
	}

	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })
	if diff := pretty.Compare(got, want); diff != "" {
		return fmt.Errorf("post-ListTreeIDs() diff (-got +want):\n%v", diff)
	}
	return nil
}

func runListTreesTest(ctx context.Context, tx storage.ReadOnlyAdminTX, includeDeleted bool, wantTrees []*trillian.Tree) error {
	got, err := tx.ListTrees(ctx, includeDeleted)
	if err != nil {
		return fmt.Errorf("ListTrees() returned err = %v", err)
	}

	if len(got) != len(wantTrees) {
		return fmt.Errorf("ListTrees() returned %v trees, want = %v", len(got), len(wantTrees))
	}

	want := wantTrees
	sort.Slice(got, func(i, j int) bool { return got[i].TreeId < got[j].TreeId })
	sort.Slice(want, func(i, j int) bool { return want[i].TreeId < want[j].TreeId })

	for i, wantTree := range want {
		if !proto.Equal(got[i], wantTree) {
			return fmt.Errorf("post-ListTrees() diff (-got +want):\n%v", pretty.Compare(got, want))
		}
	}
	return nil
}

// TestSoftDeleteTree tests success scenarios of SoftDeleteTree.
func (tester *AdminStorageTester) TestSoftDeleteTree(t *testing.T) {
	ctx := context.Background()
	s := tester.NewAdminStorage()

	logTree := makeTreeOrFail(ctx, s, spec{Tree: LogTree}, t.Fatalf)
	mapTree := makeTreeOrFail(ctx, s, spec{Tree: MapTree}, t.Fatalf)

	tests := []struct {
		desc string
		tree *trillian.Tree
	}{
		{desc: "logTree", tree: logTree},
		{desc: "mapTree", tree: mapTree},
	}
	for _, test := range tests {
		deletedTree, err := storage.SoftDeleteTree(ctx, s, test.tree.TreeId)
		if err != nil {
			t.Errorf("%v: SoftDeleteTree() returned err = %v", test.desc, err)
			continue
		}

		if deletedTree.GetDeleteTime().GetSeconds() == 0 {
			t.Errorf("%v: tree.DeleteTime = %v, want > 0", test.desc, deletedTree.DeleteTime)
		}

		wantTree := proto.Clone(test.tree).(*trillian.Tree)
		wantTree.Deleted = true
		wantTree.DeleteTime = deletedTree.DeleteTime
		if got, want := deletedTree, wantTree; !proto.Equal(got, want) {
			t.Errorf("%v: post-SoftDeleteTree diff (-got +want):\n%v", test.desc, pretty.Compare(got, want))
		}

		if err := assertStoredTree(ctx, s, deletedTree); err != nil {
			t.Errorf("%v: %v", test.desc, err)
		}
	}
}

// TestSoftDeleteTreeErrors tests error scenarios of SoftDeleteTree.
func (tester *AdminStorageTester) TestSoftDeleteTreeErrors(t *testing.T) {
	ctx := context.Background()
	s := tester.NewAdminStorage()

	softDeleted := makeTreeOrFail(ctx, s, spec{Tree: LogTree, Deleted: true}, t.Fatalf)

	tests := []struct {
		desc     string
		treeID   int64
		wantCode codes.Code
	}{
		{desc: "unknownTree", treeID: 12345, wantCode: codes.NotFound},
		{desc: "alreadyDeleted", treeID: softDeleted.TreeId, wantCode: codes.FailedPrecondition},
	}
	for _, test := range tests {
		if _, err := storage.SoftDeleteTree(ctx, s, test.treeID); status.Code(err) != test.wantCode {
			t.Errorf("%v: SoftDeleteTree() returned err = %v, wantCode = %s", test.desc, err, test.wantCode)
		}
	}
}

// TestHardDeleteTree tests success scenarios of HardDeleteTree.
func (tester *AdminStorageTester) TestHardDeleteTree(t *testing.T) {
	ctx := context.Background()
	s := tester.NewAdminStorage()

	logTree := makeTreeOrFail(ctx, s, spec{Tree: LogTree, Deleted: true}, t.Fatalf)
	frozenTree := makeTreeOrFail(ctx, s, spec{Tree: LogTree, Deleted: true, Frozen: true}, t.Fatalf)
	mapTree := makeTreeOrFail(ctx, s, spec{Tree: MapTree, Deleted: true}, t.Fatalf)

	tests := []struct {
		desc   string
		treeID int64
	}{
		{desc: "logTree", treeID: logTree.TreeId},
		{desc: "frozenTree", treeID: frozenTree.TreeId},
		{desc: "mapTree", treeID: mapTree.TreeId},
	}
	for _, test := range tests {
		if err := storage.HardDeleteTree(ctx, s, test.treeID); err != nil {
			t.Errorf("%v: HardDeleteTree() returned err = %v", test.desc, err)
			continue
		}
	}
}

// TestHardDeleteTreeErrors tests error scenarios of HardDeleteTree.
func (tester *AdminStorageTester) TestHardDeleteTreeErrors(t *testing.T) {
	ctx := context.Background()
	s := tester.NewAdminStorage()

	activeTree := makeTreeOrFail(ctx, s, spec{Tree: LogTree}, t.Fatalf)

	tests := []struct {
		desc     string
		treeID   int64
		wantCode codes.Code
	}{
		{desc: "unknownTree", treeID: 12345, wantCode: codes.NotFound},
		{desc: "activeTree", treeID: activeTree.TreeId, wantCode: codes.FailedPrecondition},
	}
	for _, test := range tests {
		if err := storage.HardDeleteTree(ctx, s, test.treeID); status.Code(err) != test.wantCode {
			t.Errorf("%v: HardDeleteTree() returned err = %v, wantCode = %s", test.desc, err, test.wantCode)
		}
	}
}

// TestUndeleteTree tests success scenarios of UndeleteTree.
func (tester *AdminStorageTester) TestUndeleteTree(t *testing.T) {
	ctx := context.Background()
	s := tester.NewAdminStorage()

	activeDeleted := makeTreeOrFail(ctx, s, spec{Tree: LogTree, Deleted: true}, t.Fatalf)
	frozenDeleted := makeTreeOrFail(ctx, s, spec{Tree: LogTree, Frozen: true, Deleted: true}, t.Fatalf)

	tests := []struct {
		desc string
		tree *trillian.Tree
	}{
		{desc: "activeTree", tree: activeDeleted},
		{desc: "frozenTree", tree: frozenDeleted},
	}
	for _, test := range tests {
		tree, err := storage.UndeleteTree(ctx, s, test.tree.TreeId)
		if err != nil {
			t.Errorf("%v: UndeleteTree() returned err = %v", test.desc, err)
			continue
		}

		want := proto.Clone(test.tree).(*trillian.Tree)
		want.Deleted = false
		want.DeleteTime = nil
		if got := tree; !proto.Equal(got, want) {
			t.Errorf("%v: post-UndeleteTree diff (-got +want):\n%v", test.desc, pretty.Compare(got, want))
		}

		if err := assertStoredTree(ctx, s, tree); err != nil {
			t.Errorf("%v: %v", test.desc, err)
		}
	}
}

// TestUndeleteTreeErrors tests error scenarios of UndeleteTree.
func (tester *AdminStorageTester) TestUndeleteTreeErrors(t *testing.T) {
	ctx := context.Background()
	s := tester.NewAdminStorage()

	activeTree := makeTreeOrFail(ctx, s, spec{Tree: LogTree}, t.Fatalf)

	tests := []struct {
		desc     string
		treeID   int64
		wantCode codes.Code
	}{
		{desc: "unknownTree", treeID: 12345, wantCode: codes.NotFound},
		{desc: "activeTree", treeID: activeTree.TreeId, wantCode: codes.FailedPrecondition},
	}
	for _, test := range tests {
		if _, err := storage.UndeleteTree(ctx, s, test.treeID); status.Code(err) != test.wantCode {
			t.Errorf("%v: UndeleteTree() returned err = %v, wantCode = %s", test.desc, err, test.wantCode)
		}
	}
}

// TestAdminTXReadWriteTransaction tests the ReadWriteTransaction method on AdminStorage.
func (tester *AdminStorageTester) TestAdminTXReadWriteTransaction(t *testing.T) {
	tests := []struct {
		wantCommit bool
	}{
		{wantCommit: true},
		{wantCommit: false},
	}

	ctx := context.Background()
	s := tester.NewAdminStorage()

	var tree *trillian.Tree

	for i, test := range tests {
		t.Run(fmt.Sprintf("%+v", test), func(t *testing.T) {
			err := s.ReadWriteTransaction(ctx, func(ctx context.Context, tx storage.AdminTX) error {
				var err error
				tree, err = tx.CreateTree(ctx, LogTree)
				if err != nil {
					t.Fatalf("%v: CreateTree() = (_, %v), want = (_, nil)", i, err)
				}
				if !test.wantCommit {
					return fmt.Errorf("no commit %d", i)
				}
				return nil
			})
			if (err != nil && test.wantCommit) ||
				(err == nil && !test.wantCommit) {
				t.Fatalf("%v: ReadWriteTransaction() = (_, %v), want = (_, nil)", i, err)
			}

			tx2, err := s.Snapshot(ctx)
			if err != nil {
				t.Fatalf("%v: Snapshot() = (_, %v), want = (_, nil)", i, err)
			}
			defer func() {
				if err := tx2.Close(); err != nil {
					t.Errorf("tx2.Close()=%v", err)
				}
			}()
			_, err = tx2.GetTree(ctx, tree.TreeId)
			if hasErr := err != nil; !test.wantCommit != hasErr {
				t.Errorf("%v: GetTree() = (_, %v), but wantCommit = %v", i, err, test.wantCommit)
			}

			// Multiple Close() calls are fine too
			if err := tx2.Close(); err != nil {
				t.Errorf("%v: Close() = %v, want = nil", i, err)
				return
			}
		})
	}
}

// assertStoredTree verifies that "want" is equal to the tree stored under its ID.
func assertStoredTree(ctx context.Context, s storage.AdminStorage, want *trillian.Tree) error {
	got, err := storage.GetTree(ctx, s, want.TreeId)
	if err != nil {
		return fmt.Errorf("GetTree() returned err = %v", err)
	}
	if !proto.Equal(got, want) {
		return fmt.Errorf("post-GetTree() diff (-got +want):\n%v", pretty.Compare(got, want))
	}
	return nil
}

type spec struct {
	Tree            *trillian.Tree
	Frozen, Deleted bool
}

// makeTreeOrFail delegates to makeTree. If makeTree returns a non-nil error, failFn is called.
func makeTreeOrFail(ctx context.Context, s storage.AdminStorage, spec spec, failFn func(string, ...interface{})) *trillian.Tree {
	tree, err := makeTree(ctx, s, spec)
	if err != nil {
		failFn("makeTree() returned err = %v", err)
		return nil
	}
	return tree
}

// makeTree creates a tree and updates it to Frozen and/or Deleted, according to "spec".
func makeTree(ctx context.Context, s storage.AdminStorage, spec spec) (*trillian.Tree, error) {
	tree := proto.Clone(spec.Tree).(*trillian.Tree)

	var err error
	tree, err = storage.CreateTree(ctx, s, tree)
	if err != nil {
		return nil, err
	}

	if spec.Frozen {
		tree, err = storage.UpdateTree(ctx, s, tree.TreeId, func(t *trillian.Tree) {
			t.TreeState = trillian.TreeState_FROZEN
		})
		if err != nil {
			return nil, err
		}
	}

	if spec.Deleted {
		tree, err = storage.SoftDeleteTree(ctx, s, tree.TreeId)
		if err != nil {
			return nil, err
		}
	}

	// Sanity checks
	if spec.Frozen && tree.TreeState != trillian.TreeState_FROZEN {
		return nil, fmt.Errorf("makeTree(): TreeState = %s, want = %s", tree.TreeState, trillian.TreeState_FROZEN)
	}
	if tree.Deleted != spec.Deleted {
		return nil, fmt.Errorf("makeTree(): Deleted = %v, want = %v", tree.Deleted, spec.Deleted)
	}

	return tree, nil
}

func tweakedCopy(tree *trillian.Tree, modFn func(t *trillian.Tree)) *trillian.Tree {
	newTree := proto.Clone(tree).(*trillian.Tree)
	modFn(newTree)
	return newTree
}
