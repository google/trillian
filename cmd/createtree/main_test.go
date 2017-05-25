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
	"net"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/kylelemons/godebug/pretty"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

func marshalAny(p proto.Message) *any.Any {
	anyKey, err := ptypes.MarshalAny(p)
	if err != nil {
		panic(err)
	}
	return anyKey
}

func TestRun(t *testing.T) {
	pemPath, pemPassword := "../../testdata/log-rpc-server.privkey.pem", "towel"
	pemSigner, err := keys.NewFromPrivatePEMFile(pemPath, pemPassword)
	if err != nil {
		t.Fatalf("NewFromPrivatPEM(): %v", err)
	}
	pemDer, err := keys.MarshalPrivateKey(pemSigner)
	if err != nil {
		t.Fatalf("MashalPrivateKey(): %v", err)
	}
	anyPrivKey, err := ptypes.MarshalAny(&keyspb.PrivateKey{Der: pemDer})
	if err != nil {
		t.Fatalf("MarshalAny(%v): %v", pemDer, err)
	}

	// defaultTree reflects all flag defaults with the addition of a valid pk
	defaultTree := &trillian.Tree{
		TreeState:          trillian.TreeState_ACTIVE,
		TreeType:           trillian.TreeType_LOG,
		HashStrategy:       trillian.HashStrategy_RFC_6962,
		HashAlgorithm:      sigpb.DigitallySigned_SHA256,
		SignatureAlgorithm: sigpb.DigitallySigned_RSA,
		PrivateKey:         anyPrivKey,
		MaxRootDuration:    ptypes.DurationProto(0 * time.Millisecond),
	}

	server, lis, stopFn, err := startFakeServer()
	if err != nil {
		t.Fatalf("Error starting fake server: %v", err)
	}
	defer stopFn()

	validOpts := newOptsFromFlags()
	validOpts.addr = lis.Addr().String()
	validOpts.pemKeyPath = pemPath
	validOpts.pemKeyPass = pemPassword

	nonDefaultTree := *defaultTree
	nonDefaultTree.TreeType = trillian.TreeType_MAP
	nonDefaultTree.SignatureAlgorithm = sigpb.DigitallySigned_ECDSA
	nonDefaultTree.DisplayName = "Llamas Map"
	nonDefaultTree.Description = "For all your digital llama needs!"

	nonDefaultOpts := *validOpts
	nonDefaultOpts.treeType = nonDefaultTree.TreeType.String()
	nonDefaultOpts.sigAlgorithm = nonDefaultTree.SignatureAlgorithm.String()
	nonDefaultOpts.displayName = nonDefaultTree.DisplayName
	nonDefaultOpts.description = nonDefaultTree.Description

	emptyAddr := *validOpts
	emptyAddr.addr = ""

	invalidEnumOpts := *validOpts
	invalidEnumOpts.treeType = "LLAMA!"

	invalidKeyTypeOpts := *validOpts
	invalidKeyTypeOpts.privateKeyType = "LLAMA!!"

	emptyPEMPath := *validOpts
	emptyPEMPath.pemKeyPath = ""

	emptyPEMPass := *validOpts
	emptyPEMPass.pemKeyPass = ""

	pemKeyOpts := *validOpts
	pemKeyOpts.privateKeyType = "PEMKeyFile"
	pemKeyTree := *defaultTree
	pemKeyTree.PrivateKey, err = ptypes.MarshalAny(&keyspb.PEMKeyFile{
		Path:     pemPath,
		Password: pemPassword,
	})
	if err != nil {
		t.Fatalf("MarshalAny(PEMKeyFile): %v", err)
	}
	tests := []struct {
		desc      string
		opts      *createOpts
		createErr error
		wantErr   bool
		wantTree  *trillian.Tree
	}{
		{desc: "validOpts", opts: validOpts, wantTree: defaultTree},
		{desc: "nonDefaultOpts", opts: &nonDefaultOpts, wantTree: &nonDefaultTree},
		{desc: "defaultOptsOnly", opts: newOptsFromFlags(), wantErr: true}, // No mandatory opts provided
		{desc: "emptyAddr", opts: &emptyAddr, wantErr: true},
		{desc: "invalidEnumOpts", opts: &invalidEnumOpts, wantErr: true},
		{desc: "invalidKeyTypeOpts", opts: &invalidKeyTypeOpts, wantErr: true},
		{desc: "emptyPEMPath", opts: &emptyPEMPath, wantErr: true},
		{desc: "emptyPEMPass", opts: &emptyPEMPass, wantErr: true},
		{desc: "PEMKeyFile", opts: &pemKeyOpts, wantErr: false, wantTree: &pemKeyTree},
		{desc: "createErr", opts: validOpts, createErr: errors.New("create tree failed"), wantErr: true},
	}

	ctx := context.Background()
	for _, test := range tests {
		server.err = test.createErr

		tree, err := createTree(ctx, test.opts)
		switch hasErr := err != nil; {
		case hasErr != test.wantErr:
			t.Errorf("%v: createTree() returned err = '%v', wantErr = %v", test.desc, err, test.wantErr)
			continue
		case hasErr:
			continue
		}

		if diff := pretty.Compare(tree, test.wantTree); diff != "" {
			t.Errorf("%v: post-createTree diff:\n%v", test.desc, diff)
		}
	}
}

// fakeAdminServer that implements CreateTree. If err is nil, the CreateTree
// input is echoed as the output, otherwise err is returned instead.
// The remaining methods are not implemented.
type fakeAdminServer struct {
	err error
}

// startFakeServer starts a fakeAdminServer on a random port.
// Returns the started server, the listener it's using for connection and a
// close function that must be defer-called on the scope the server is meant to
// stop.
func startFakeServer() (*fakeAdminServer, net.Listener, func(), error) {
	grpcServer := grpc.NewServer()
	fakeServer := &fakeAdminServer{}
	trillian.RegisterTrillianAdminServer(grpcServer, fakeServer)

	lis, err := net.Listen("tcp", "")
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
