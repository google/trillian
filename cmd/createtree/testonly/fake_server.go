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
	"fmt"
	"net"

	"github.com/golang/protobuf/ptypes/any"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/google/trillian/crypto/sigpb"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// FakeAdminServer implements the TrillianAdminServer CreateTree RPC.
// The remaining RPCs are not implemented.
type FakeAdminServer struct {
	trillian.TrillianAdminServer

	// Err will be returned by CreateTree if not nil.
	Err error
	// GeneratedKey will be used to set a tree's PrivateKey if a CreateTree request has a KeySpec.
	// This is for simulating key generation.
	GeneratedKey *any.Any
}

// StartFakeAdminServer starts a FakeAdminServer on a random port.
// Returns the started server, the listener it's using for connection and a
// close function that must be defer-called on the scope the server is meant to
// stop.
func StartFakeAdminServer(server *FakeAdminServer) (net.Listener, func(), error) {
	grpcServer := grpc.NewServer()
	trillian.RegisterTrillianAdminServer(grpcServer, server)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	go grpcServer.Serve(lis)

	stopFn := func() {
		grpcServer.Stop()
		lis.Close()
	}

	return lis, stopFn, nil
}

// CreateTree returns req.Tree, unless s.Err is not nil, in which case it
// returns s.Err. This allows tests to examine the requested tree and check
// behavior under error conditions.
// If s.GeneratedKey and req.KeySpec are not nil, the returned tree will have
// its PrivateKey field set to s.GeneratedKey.
func (s *FakeAdminServer) CreateTree(ctx context.Context, req *trillian.CreateTreeRequest) (*trillian.Tree, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	resp := *req.Tree
	if req.KeySpec != nil {
		if s.GeneratedKey == nil {
			panic("fakeAdminServer.GeneratedKey == nil but CreateTreeRequest requests generated key")
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

		resp.PrivateKey = s.GeneratedKey
	}
	return &resp, nil
}
