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

package main

import (
	"context"
	"errors"
	"flag"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/trillian"
	"github.com/google/trillian/cmd/createtree/testonly"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/util/flagsaver"
	"github.com/kylelemons/godebug/pretty"
)

// defaultTree reflects all flag defaults with the addition of a valid private key.
var defaultTree = &trillian.Tree{
	TreeState:          trillian.TreeState_ACTIVE,
	TreeType:           trillian.TreeType_LOG,
	HashStrategy:       trillian.HashStrategy_RFC6962_SHA256,
	HashAlgorithm:      sigpb.DigitallySigned_SHA256,
	SignatureAlgorithm: sigpb.DigitallySigned_ECDSA,
	PrivateKey:         mustMarshalAny(&empty.Empty{}),
	MaxRootDuration:    ptypes.DurationProto(0 * time.Millisecond),
}

type testCase struct {
	desc      string
	setFlags  func()
	createErr error
	wantErr   bool
	wantTree  *trillian.Tree
}

func mustMarshalAny(p proto.Message) *any.Any {
	anyKey, err := ptypes.MarshalAny(p)
	if err != nil {
		panic(err)
	}
	return anyKey
}

func TestCreateTree(t *testing.T) {
	nonDefaultTree := *defaultTree
	nonDefaultTree.TreeType = trillian.TreeType_MAP
	nonDefaultTree.SignatureAlgorithm = sigpb.DigitallySigned_RSA
	nonDefaultTree.DisplayName = "Llamas Map"
	nonDefaultTree.Description = "For all your digital llama needs!"

	runTest(t, []*testCase{
		{
			desc: "validOpts",
			// runTest sets mandatory options, so no need to provide a setFlags func.
			wantTree: defaultTree,
		},
		{
			desc: "nonDefaultOpts",
			setFlags: func() {
				*treeType = nonDefaultTree.TreeType.String()
				*signatureAlgorithm = nonDefaultTree.SignatureAlgorithm.String()
				*displayName = nonDefaultTree.DisplayName
				*description = nonDefaultTree.Description
			},
			wantTree: &nonDefaultTree,
		},
		{
			desc: "mandatoryOptsNotSet",
			// Undo the flags set by runTest, so that mandatory options are no longer set.
			setFlags: resetFlags,
			wantErr:  true,
		},
		{
			desc:     "emptyAddr",
			setFlags: func() { *adminServerAddr = "" },
			wantErr:  true,
		},
		{
			desc:     "invalidEnumOpts",
			setFlags: func() { *treeType = "LLAMA!" },
			wantErr:  true,
		},
		{
			desc:     "invalidKeyTypeOpts",
			setFlags: func() { *privateKeyFormat = "LLAMA!!" },
			wantErr:  true,
		},
		{
			desc:      "createErr",
			createErr: errors.New("create tree failed"),
			wantErr:   true,
		},
	})
}

// runTest executes the createtree command against a fake TrillianAdminServer
// for each of the provided tests, and checks that the tree in the request is
// as expected, or an expected error occurs.
// Prior to each test case, it:
// 1. Resets all flags to their original values.
// 2. Sets the adminServerAddr flag to point to the fake server.
// 3. Calls the test's setFlags func (if provided) to allow it to change flags specific to the test.
func runTest(t *testing.T, tests []*testCase) {
	server := &testonly.FakeAdminServer{
		GeneratedKey: defaultTree.PrivateKey,
	}

	lis, stopFakeServer, err := testonly.StartFakeAdminServer(server)
	if err != nil {
		t.Fatalf("Error starting fake server: %v", err)
	}
	defer stopFakeServer()

	ctx := context.Background()
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			defer flagsaver.Save().Restore()
			*adminServerAddr = lis.Addr().String()
			if test.setFlags != nil {
				test.setFlags()
			}

			server.Err = test.createErr

			tree, err := createTree(ctx)
			switch hasErr := err != nil; {
			case hasErr != test.wantErr:
				t.Errorf("createTree() returned err = '%v', wantErr = %v", err, test.wantErr)
				return
			case hasErr:
				return
			}

			if !proto.Equal(tree, test.wantTree) {
				t.Errorf("post-createTree diff -got +want:\n%v", pretty.Compare(tree, test.wantTree))
			}
		})
	}
}

// resetFlags sets all flags to their default values.
func resetFlags() {
	flag.Visit(func(f *flag.Flag) {
		f.Value.Set(f.DefValue)
	})
}
