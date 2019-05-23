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

package integration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/client"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/types"
)

var (
	keyNonExistent = []byte{
		0xd1, 0xd2, 0xd3, 0xd4, 0xd5, 0xd6, 0xd7, 0xd8, 0xd9, 0xda, 0xdb, 0xdc, 0xdd, 0xde, 0xdf, 0xd0,
		0xe1, 0xe2, 0xe3, 0xe4, 0xe5, 0xe6, 0xe7, 0xe8, 0xe9, 0xea, 0xeb, 0xec, 0xed, 0xee, 0xef, 0xf0,
	}
	key0 = []byte{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20,
	}
	key1 = []byte{
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20,
		0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, 0x29, 0x2a, 0x2b, 0x2c, 0x2d, 0x2e, 0x2f, 0x30,
	}
	value0 = []byte("value-0000")
	value1 = []byte("value-0001")
	value2 = []byte("value-0002")
)

// MapInfo describes an in-progress integration test.
type MapInfo struct {
	cl       trillian.TrillianMapClient
	id       int64
	tree     *trillian.Tree
	verifier *client.MapVerifier
	contents *testonly.MapContents
}

// New builds a MapInfo to track the progress of an integration test run.
func New(cl trillian.TrillianMapClient, tree *trillian.Tree) (*MapInfo, error) {
	verifier, err := client.NewMapVerifierFromTree(tree)
	if err != nil {
		return nil, fmt.Errorf("failed to create map verifier: %v", err)
	}
	return &MapInfo{
		cl:       cl,
		id:       tree.TreeId,
		tree:     tree,
		verifier: verifier,
		contents: &testonly.MapContents{},
	}, nil
}

// RunIntegration runs a simple Map integration test.
// nolint: gocyclo
func (mi *MapInfo) RunIntegration(ctx context.Context) error {
	fmt.Printf("%d: ================ Revision 0 ==================\n", mi.id)
	// Map should be empty at revision 0.
	fmt.Printf("%d: Get SMR\n", mi.id)
	smrRsp, err := mi.cl.GetSignedMapRoot(ctx, &trillian.GetSignedMapRootRequest{MapId: mi.id})
	if err != nil {
		return fmt.Errorf("failed to retrieve SMR 0: %v", err)
	}
	root0, err := mi.checkMapRoot(smrRsp.MapRoot)
	if err != nil {
		return fmt.Errorf("checks on SMR 0 failed: %v", err)
	}
	fmt.Printf("%d: Got SMR(time=%q, rev=%d): roothash=%x\n", mi.id, timeFromNanos(root0.TimestampNanos), root0.Revision, root0.RootHash)

	// Asking for a later revision should fail.
	fmt.Printf("%d: Get SMR(2)\n", mi.id)
	smrRsp, err = mi.cl.GetSignedMapRootByRevision(ctx, &trillian.GetSignedMapRootByRevisionRequest{MapId: mi.id, Revision: 2})
	if err == nil {
		return fmt.Errorf("unexpectedly succeeded in get-SMR-at-rev[2]: %+v", smrRsp)
	}
	fmt.Printf("%d: SMR(@2) not available (as expected)\n", mi.id)

	// All keys should give an empty value.
	if err := mi.expectEmptyValues(ctx, keyNonExistent, key0, key1); err != nil {
		return err
	}

	fmt.Printf("%d: ================ Revision 1 ==================\n", mi.id)
	// Add a single leaf (key0=>value0) for revision 1.
	fmt.Printf("%d: Set map[key0]=%q\n", mi.id, value0)
	leaves := []*trillian.MapLeaf{{Index: key0, LeafValue: value0}}
	setRsp, err := mi.cl.SetLeaves(ctx, &trillian.SetMapLeavesRequest{MapId: mi.id, Leaves: leaves})
	if err != nil {
		return fmt.Errorf("failed to set-map-leaves: %v", err)
	}
	mi.contents = mi.contents.UpdatedWith(1, leaves)

	root1, err := mi.checkMapRoot(setRsp.MapRoot)
	if err != nil {
		return fmt.Errorf("checks on SMR 1 failed: %v", err)
	}
	fmt.Printf("%d: Got SMR(time=%q, rev=%d): roothash=%x\n", mi.id, timeFromNanos(root1.TimestampNanos), root1.Revision, root1.RootHash)
	if err := mi.expectEmptyValues(ctx, keyNonExistent, key1); err != nil {
		return err
	}
	if err := mi.expectValue(ctx, key0, value0); err != nil {
		return err
	}

	fmt.Printf("%d: ================ Revision 2 ==================\n", mi.id)
	// Remove the single leaf by setting it to have empty contents, creating revision 2.
	fmt.Printf("%d: Set map[key0]=''\n", mi.id)
	emptyLeaves := []*trillian.MapLeaf{{Index: key0, LeafValue: []byte{}}}
	setRsp, err = mi.cl.SetLeaves(ctx, &trillian.SetMapLeavesRequest{MapId: mi.id, Leaves: emptyLeaves, Metadata: []byte("metadata-2")})
	if err != nil {
		return fmt.Errorf("failed to set-map-leaves: %v", err)
	}
	mi.contents = mi.contents.UpdatedWith(2, emptyLeaves)

	root2, err := mi.checkMapRoot(setRsp.MapRoot)
	if err != nil {
		return fmt.Errorf("checks on SMR 2 failed: %v", err)
	}
	fmt.Printf("%d: Got SMR(time=%q, rev=%d): roothash=%x\n", mi.id, timeFromNanos(root2.TimestampNanos), root2.Revision, root2.RootHash)
	// All keys should give an empty value.
	if err := mi.expectEmptyValues(ctx, keyNonExistent, key0, key1); err != nil {
		return err
	}

	// Revision 1 is unaffected and still has key0=>value0
	if err := mi.expectValueAtRev(ctx, 1, key0, value0); err != nil {
		return err
	}
	// Revision 0 is unaffected and still has key0=><nil>
	if err := mi.expectValueAtRev(ctx, 0, key0, nil); err != nil {
		return err
	}

	// Revision 0 and revision 2 should have the same root hash.
	if !bytes.Equal(root2.RootHash, root0.RootHash) {
		return fmt.Errorf("unexpected root2.Hash=%x vs root0.Hash=%x", root2.RootHash, root0.RootHash)
	}

	fmt.Printf("%d: ================ Revision 3 ==================\n", mi.id)
	// Set two leaves in one go to make revision 3.
	fmt.Printf("%d: Set map[key0]=%q\n", mi.id, value1)
	fmt.Printf("%d: Set map[key1]=%q\n", mi.id, value2)

	newLeaves := []*trillian.MapLeaf{
		{Index: key0, LeafValue: value1},
		{Index: key1, LeafValue: value2},
	}
	setRsp, err = mi.cl.SetLeaves(ctx, &trillian.SetMapLeavesRequest{MapId: mi.id, Leaves: newLeaves, Metadata: []byte("metadata-3")})
	if err != nil {
		return fmt.Errorf("failed to set-map-leaves: %v", err)
	}
	mi.contents = mi.contents.UpdatedWith(3, newLeaves)

	root3, err := mi.checkMapRoot(setRsp.MapRoot)
	if err != nil {
		return fmt.Errorf("checks on SMR 3 failed: %v", err)
	}
	fmt.Printf("%d: Got SMR(time=%q, rev=%d): roothash=%x\n", mi.id, timeFromNanos(root3.TimestampNanos), root3.Revision, root3.RootHash)
	if err := mi.expectEmptyValues(ctx, keyNonExistent); err != nil {
		return err
	}
	if err := mi.expectValue(ctx, key0, value1); err != nil {
		return err
	}
	if err := mi.expectValue(ctx, key1, value2); err != nil {
		return err
	}
	// Revision 2 is unaffected and still has key0=><nil>
	if err := mi.expectValueAtRev(ctx, 2, key0, nil); err != nil {
		return err
	}
	// Asking for revision 1's root should give the same answer as before.
	smrRsp, err = mi.cl.GetSignedMapRootByRevision(ctx, &trillian.GetSignedMapRootByRevisionRequest{MapId: mi.id, Revision: 1})
	if err != nil {
		return fmt.Errorf("failed to re-retrieve SMR 1: %v", err)
	}
	root1a, err := mi.verifier.VerifySignedMapRoot(smrRsp.MapRoot)
	if err != nil {
		return fmt.Errorf("failed to verify SMR 1: %v", err)
	}
	if !reflect.DeepEqual(root1, root1a) {
		return fmt.Errorf("re-retrieving SMR 1 (%+v) different than earlier (%+v)", root1a, root1)
	}

	fmt.Printf("%d: ============== Invalid Requests ================\n", mi.id)
	if _, err := mi.cl.SetLeaves(ctx, &trillian.SetMapLeavesRequest{MapId: mi.id + 1, Leaves: newLeaves}); err == nil {
		return errors.New("succeeded in set-map-leaves for wrong MapId")
	}
	if _, err := mi.cl.GetLeaves(ctx, &trillian.GetMapLeavesRequest{MapId: mi.id + 1, Index: [][]byte{key0}}); err == nil {
		return errors.New("succeeded in get-map-leaves for wrong MapId")
	}
	if _, err = mi.cl.GetSignedMapRootByRevision(ctx, &trillian.GetSignedMapRootByRevisionRequest{MapId: mi.id, Revision: 100}); err == nil {
		return errors.New("succeeded in get-map-root for a future revision")
	}

	return nil
}

func (mi *MapInfo) checkMapRoot(smr *trillian.SignedMapRoot) (*types.MapRootV1, error) {
	root, err := mi.verifier.VerifySignedMapRoot(smr)
	if err != nil {
		return nil, fmt.Errorf("failed to verify SMR: %v", err)
	}
	wantRootHash, err := mi.contents.RootHash(mi.id, mi.verifier.Hasher)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate expected root hash: %v", err)
	}
	if !bytes.Equal(root.RootHash, wantRootHash) {
		return nil, fmt.Errorf("unexpected root hash %x, want %x", root.RootHash, wantRootHash)
	}

	if got, want := int64(root.Revision), mi.contents.Rev; got != want {
		return nil, fmt.Errorf("unexpected revision %d, want %d", got, want)
	}
	return root, nil
}

// expectEmptyValues retrieves the current map values for a set of keys, and check that
// all of the corresponding values are empty.
func (mi *MapInfo) expectEmptyValues(ctx context.Context, keys ...[]byte) error {
	fmt.Printf("%d: GetLeaves(keys=%x)\n", mi.id, keys)
	leavesRsp, err := mi.cl.GetLeaves(ctx, &trillian.GetMapLeavesRequest{MapId: mi.id, Index: keys})
	if err != nil {
		return fmt.Errorf("failed to get-map-leaves: %v", err)
	}
	root, err := mi.verifier.VerifySignedMapRoot(leavesRsp.MapRoot)
	if err != nil {
		return fmt.Errorf("failed to verify SMR in get-map-leaves-rsp: %v", err)
	}
	fmt.Printf("%d:   SMR(time=%q, rev=%d): roothash=%x\n", mi.id, timeFromNanos(root.TimestampNanos), root.Revision, root.RootHash)
	if got, want := len(leavesRsp.MapLeafInclusion), len(keys); got != want {
		return fmt.Errorf("unexpected leaf count from get-map-leaves: %d", got)
	}
	for i, key := range keys {
		got := leavesRsp.MapLeafInclusion[i].Leaf.LeafValue
		fmt.Printf("%d:   Leaf[%x] = %q\n", mi.id, key, got)
		if len(got) != 0 {
			return fmt.Errorf("unexpected leaf value %x from get-map-leaves", got)
		}
		if got := leavesRsp.MapLeafInclusion[i].Leaf.Index; !bytes.Equal(got, key) {
			return fmt.Errorf("unexpected leaf key %x from get-map-leaves", got)
		}
		if err := mi.verifier.VerifyMapLeafInclusionHash(root.RootHash, leavesRsp.MapLeafInclusion[i]); err != nil {
			return fmt.Errorf("failed to verify inclusion proof for key=%x: %v", key, err)
		}
	}
	return nil
}

// expectValue retrieves the current map value for a particular key and checks that the
// associated LeafValue is the specified value.
func (mi *MapInfo) expectValue(ctx context.Context, key, want []byte) error {
	fmt.Printf("%d: GetLeaves(key=%x)\n", mi.id, key)
	leavesRsp, err := mi.cl.GetLeaf(ctx, &trillian.GetMapLeafRequest{MapId: mi.id, Index: key})
	if err != nil {
		return fmt.Errorf("failed to get-map-leaf: %v", err)
	}
	return mi.checkSingleLeafResponse(leavesRsp, key, want)
}

// expectValueAtRev retrieves the current map value for a particular key and checks that the
// associated LeafValue is the specified value.
func (mi *MapInfo) expectValueAtRev(ctx context.Context, rev int64, key, want []byte) error {
	fmt.Printf("%d: GetLeavesAtRevision(rev=%d, key=%x)\n", mi.id, rev, key)
	leavesRsp, err := mi.cl.GetLeafByRevision(ctx, &trillian.GetMapLeafByRevisionRequest{MapId: mi.id, Revision: rev, Index: key})
	if err != nil {
		return fmt.Errorf("failed to get-map-leaf-at-rev: %v", err)
	}
	return mi.checkSingleLeafResponse(leavesRsp, key, want)
}

// checkSingleLeaf response checks a response has exactly the leaf info expected.
func (mi *MapInfo) checkSingleLeafResponse(rsp *trillian.GetMapLeafResponse, key, want []byte) error {
	root, err := mi.verifier.VerifySignedMapRoot(rsp.MapRoot)
	if err != nil {
		return fmt.Errorf("failed to verify SMR in get-map-leaves-rsp: %v", err)
	}
	fmt.Printf("%d:   SMR(time=%q, rev=%d): roothash=%x\n", mi.id, timeFromNanos(root.TimestampNanos), root.Revision, root.RootHash)
	if got := rsp.MapLeafInclusion.Leaf.Index; !bytes.Equal(got, key) {
		return fmt.Errorf("unexpected leaf key %x from get-map-leaves", got)
	}
	wantHash := mi.verifier.Hasher.HashLeaf(mi.id, key, want)
	// The hash for an empty leaf may be nil or wantHash.
	if got := rsp.MapLeafInclusion.Leaf.LeafHash; !bytes.Equal(got, wantHash) && (len(want) != 0 || len(got) != 0) {
		return fmt.Errorf("unexpected leaf hash %x from get-map-leaves, want %x", got, wantHash)
	}
	if err := mi.verifier.VerifyMapLeafInclusionHash(root.RootHash, rsp.MapLeafInclusion); err != nil {
		return fmt.Errorf("failed to verify inclusion proof for key=%x: %v", key, err)
	}
	got := rsp.MapLeafInclusion.Leaf.LeafValue
	fmt.Printf("%d:   Leaf[%x] = %q\n", mi.id, key, got)
	if !bytes.Equal(got, want) {
		return fmt.Errorf("unexpected leaf value %x from get-map-leaves, want %x", got, want)
	}
	return nil
}

// timeFromNanos converts a timestamp in nanoseconds  to a time.Time.
func timeFromNanos(nanos uint64) time.Time {
	return time.Unix(0, int64(nanos))
}
