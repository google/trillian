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
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
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
	err := os.Chdir("../..")
	if err != nil {
		t.Fatalf("Unable to change working directory to ../..: %s", err)
	}

	pemPath, pemPassword := "testdata/log-rpc-server.privkey.pem", "towel"
	pemSigner, err := keys.NewFromPrivatePEMFile(pemPath, pemPassword)
	if err != nil {
		t.Fatalf("NewFromPrivatePEM(): %v", err)
	}
	pemDer, err := keys.MarshalPrivateKey(pemSigner)
	if err != nil {
		t.Fatalf("MarshalPrivateKey(): %v", err)
	}
	anyPrivKey, err := ptypes.MarshalAny(&keyspb.PrivateKey{Der: pemDer})
	if err != nil {
		t.Fatalf("MarshalAny(%v): %v", pemDer, err)
	}

	// defaultTree reflects all flag defaults with the addition of a valid pk
	defaultTree := &trillian.Tree{
		TreeState:          trillian.TreeState_ACTIVE,
		TreeType:           trillian.TreeType_LOG,
		HashStrategy:       trillian.HashStrategy_RFC6962_SHA256,
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
	server.generatedKey = anyPrivKey

	validOpts := newOptsFromFlags()
	validOpts.addr = lis.Addr().String()

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

	privateKeyOpts := *validOpts
	privateKeyOpts.privateKeyType = "PrivateKey"
	privateKeyOpts.pemKeyPath = pemPath
	privateKeyOpts.pemKeyPass = pemPassword

	emptyKeyTypeOpts := privateKeyOpts
	emptyKeyTypeOpts.privateKeyType = ""

	invalidKeyTypeOpts := privateKeyOpts
	invalidKeyTypeOpts.privateKeyType = "LLAMA!!"

	emptyPEMPath := privateKeyOpts
	emptyPEMPath.pemKeyPath = ""

	emptyPEMPass := privateKeyOpts
	emptyPEMPass.pemKeyPass = ""

	pemKeyOpts := privateKeyOpts
	pemKeyOpts.privateKeyType = "PEMKeyFile"
	pemKeyTree := *defaultTree
	pemKeyTree.PrivateKey, err = ptypes.MarshalAny(&keyspb.PEMKeyFile{
		Path:     pemPath,
		Password: pemPassword,
	})
	if err != nil {
		t.Fatalf("MarshalAny(PEMKeyFile): %v", err)
	}

	pkcs11Opts := *validOpts
	pkcs11Opts.privateKeyType = "PKCS11ConfigFile"
	pkcs11Opts.pkcs11ConfigPath = "testdata/pkcs11-conf.json"
	pkcs11Tree := *defaultTree
	pkcs11Tree.PrivateKey, err = ptypes.MarshalAny(&keyspb.PKCS11Config{
		TokenLabel: "log",
		Pin:        "1234",
		PublicKey: `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQC7/tWwqUXZJaNfnpvnqiaeNMkn
hKusCsyAidrHxvuL+t54XFCHJwsB3wIlQZ4mMwb8mC/KRYhCqECBEoCAf/b0m3j/
ASuEPLyYOrz/aEs3wP02IZQLGmihmjMk7T/ouNCuX7y1fTjX3GeVQ06U/EePwZFC
xToc6NWBri0N3VVsswIDAQAB
-----END PUBLIC KEY-----
`,
	})
	if err != nil {
		t.Fatalf("MarshalAny(PKCS11Config): %v", err)
	}

	emptyPKCS11Path := pkcs11Opts
	emptyPKCS11Path.pkcs11ConfigPath = ""

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
		{desc: "emptyKeyTypeOpts", opts: &emptyKeyTypeOpts, wantErr: true},
		{desc: "invalidKeyTypeOpts", opts: &invalidKeyTypeOpts, wantErr: true},
		{desc: "emptyPEMPath", opts: &emptyPEMPath, wantErr: true},
		{desc: "emptyPEMPass", opts: &emptyPEMPass, wantErr: true},
		{desc: "PrivateKey", opts: &privateKeyOpts, wantTree: defaultTree},
		{desc: "PEMKeyFile", opts: &pemKeyOpts, wantTree: &pemKeyTree},
		{desc: "createErr", opts: validOpts, createErr: errors.New("create tree failed"), wantErr: true},
		{desc: "PKCS11Config", opts: &pkcs11Opts, wantTree: &pkcs11Tree},
		{desc: "emptyPKCS11Path", opts: &emptyPKCS11Path, wantErr: true},
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

		if !proto.Equal(tree, test.wantTree) {
			t.Errorf("%v: post-createTree diff -got +want:\n%v", test.desc, pretty.Compare(tree, test.wantTree))
		}
	}
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
