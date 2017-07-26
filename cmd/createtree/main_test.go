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
	"errors"
	"flag"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/util/flagsaver"
	"github.com/kylelemons/godebug/pretty"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// defaultTree reflects all flag defaults with the addition of a valid private key.
var defaultTree = &trillian.Tree{
	TreeState:          trillian.TreeState_ACTIVE,
	TreeType:           trillian.TreeType_LOG,
	HashStrategy:       trillian.HashStrategy_RFC6962_SHA256,
	HashAlgorithm:      sigpb.DigitallySigned_SHA256,
	SignatureAlgorithm: sigpb.DigitallySigned_RSA,
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
	nonDefaultTree.SignatureAlgorithm = sigpb.DigitallySigned_ECDSA
	nonDefaultTree.DisplayName = "Llamas Map"
	nonDefaultTree.Description = "For all your digital llama needs!"

	runTest(t, []*testCase{
		{
			desc:     "validOpts",
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
			desc:     "defaultOptsOnly",
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

func runTest(t *testing.T, tests []*testCase) {
	server, lis, stopFn, err := startFakeServer()
	if err != nil {
		t.Fatalf("Error starting fake server: %v", err)
	}
	defer stopFn()
	server.generatedKey = defaultTree.PrivateKey

	ctx := context.Background()
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			defer flagsaver.Save().Restore()
			*adminServerAddr = lis.Addr().String()
			if test.setFlags != nil {
				test.setFlags()
			}

			server.err = test.createErr

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

func resetFlags() {
	flag.Visit(func(f *flag.Flag) {
		f.Value.Set(f.DefValue)
	})
}

// fakeAdminServer that implements CreateTree.
// If err is not nil, it will be returned in response to CreateTree requests.
// If generatedKey is not nil, and a request has a KeySpec set, the response
// will contain generatedKey.
// The response to a CreateTree request will otherwise contain an identical copy
// of the tree sent in the request.
// The remaining methods are not implemented.
type fakeAdminServer struct {
	err          error
	generatedKey *any.Any
}

// startFakeServer starts a fakeAdminServer on a random port.
// Returns the started server, the listener it's using for connection and a
// close function that must be defer-called on the scope the server is meant to
// stop.
func startFakeServer() (*fakeAdminServer, net.Listener, func(), error) {
	grpcServer := grpc.NewServer()
	fakeServer := &fakeAdminServer{}
	trillian.RegisterTrillianAdminServer(grpcServer, fakeServer)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, nil, err
	}
	go grpcServer.Serve(lis)

	stopFn := func() {
		grpcServer.Stop()
		lis.Close()
	}
	return fakeServer, lis, stopFn, nil
}

func (s *fakeAdminServer) CreateTree(ctx context.Context, req *trillian.CreateTreeRequest) (*trillian.Tree, error) {
	if s.err != nil {
		return nil, s.err
	}
	resp := *req.Tree
	if req.KeySpec != nil {
		if s.generatedKey == nil {
			panic("fakeAdminServer.generatedKey == nil but CreateTreeRequest requests generated key")
		}

		var keySigAlgo sigpb.DigitallySigned_SignatureAlgorithm
		switch req.KeySpec.Params.(type) {
		case *keyspb.Specification_EcdsaParams:
			keySigAlgo = sigpb.DigitallySigned_ECDSA
		case *keyspb.Specification_RsaParams:
			keySigAlgo = sigpb.DigitallySigned_RSA
		default:
			return nil, fmt.Errorf("got unsupported type of key_spec.params: %T", req.KeySpec.Params)
		}
		if treeSigAlgo := req.Tree.GetSignatureAlgorithm(); treeSigAlgo != keySigAlgo {
			return nil, fmt.Errorf("got tree.SignatureAlgorithm = %v but key_spec.Params of type %T", treeSigAlgo, req.KeySpec.Params)
		}

		resp.PrivateKey = s.generatedKey
	}
	return &resp, nil
}

var errUnimplemented = errors.New("unimplemented")

func (s *fakeAdminServer) ListTrees(context.Context, *trillian.ListTreesRequest) (*trillian.ListTreesResponse, error) {
	return nil, errUnimplemented
}

func (s *fakeAdminServer) GetTree(context.Context, *trillian.GetTreeRequest) (*trillian.Tree, error) {
	return nil, errUnimplemented
}

func (s *fakeAdminServer) UpdateTree(context.Context, *trillian.UpdateTreeRequest) (*trillian.Tree, error) {
	return nil, errUnimplemented
}

func (s *fakeAdminServer) DeleteTree(context.Context, *trillian.DeleteTreeRequest) (*empty.Empty, error) {
	return nil, errUnimplemented
}
