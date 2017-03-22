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
	"fmt"
	"os"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/sigpb"
	"google.golang.org/grpc"
)

var (
	adminEndpoint = flag.String("admin_endpoint", "", "Endpoint of the gRPC Trillian Admin Server")

	treeState          = flag.String("tree_state", trillian.TreeState_ACTIVE.String(), "State of the new tree")
	treeType           = flag.String("tree_type", trillian.TreeType_LOG.String(), "Type of the new tree")
	hashStrategy       = flag.String("hash_strategy", trillian.HashStrategy_RFC_6962.String(), "Hash strategy (aka preimage protection) of the new tree")
	hashAlgorithm      = flag.String("hash_algorithm", sigpb.DigitallySigned_SHA256.String(), "Hash algorithm of the new tree")
	signatureAlgorithm = flag.String("signature_algorithm", sigpb.DigitallySigned_RSA.String(), "Signature algorithm of the new tree")
	duplicatePolicy    = flag.String("duplicate_policy", trillian.DuplicatePolicy_DUPLICATES_NOT_ALLOWED.String(), "Duplicate policy of the new tree")
	displayName        = flag.String("display_name", "", "Display name of the new tree")
	description        = flag.String("description", "", "Description of the new tree")

	privateKeyType = flag.String("private_key_type", "PEMKeyFile", "Type of private key to be used")
	pemKeyPath     = flag.String("pem_key_path", "", "Path to the private key PEM file")
	pemKeyPassword = flag.String("pem_key_password", "", "Password of the private key PEM file")
)

// println is used by tests to examine program output.
var println = fmt.Println

// createOpts contains all user-supplied options required to run the program.
// It's meant to facilitate tests and focus flag reads to a single point.
type createOpts struct {
	endpoint                                                                                                  string
	treeState, treeType, hashStrategy, hashAlgorithm, sigAlgorithm, duplicatePolicy, displayName, description string
	privateKeyType, pemKeyPath, pemKeyPass                                                                    string
}

func createTree(opts *createOpts) (*trillian.Tree, error) {
	req, err := newRequestFromFlags(opts)
	if err != nil {
		return nil, err
	}

	conn, err := grpc.Dial(opts.endpoint, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	ctx := context.Background()
	tree, err := trillian.NewTrillianAdminClient(conn).CreateTree(ctx, req)
	if err != nil {
		return nil, err
	}

	// DO NOT change the output format, scripts are meant to depend on it.
	// If you really want to change it, provide an output_format flag and
	// keep the default as-is.
	println(tree.TreeId)

	return tree, nil
}

func newRequestFromFlags(opts *createOpts) (*trillian.CreateTreeRequest, error) {
	tree := &trillian.Tree{
		DisplayName: opts.displayName,
		Description: opts.description,
	}

	enums := []struct {
		values map[string]int32
		name   string
		assign func(int32, *trillian.Tree)
	}{
		{
			trillian.TreeState_value,
			opts.treeState,
			func(v int32, t *trillian.Tree) { t.TreeState = trillian.TreeState(v) },
		},
		{
			trillian.TreeType_value,
			opts.treeType,
			func(v int32, t *trillian.Tree) { t.TreeType = trillian.TreeType(v) },
		},
		{
			trillian.HashStrategy_value,
			opts.hashStrategy,
			func(v int32, t *trillian.Tree) { t.HashStrategy = trillian.HashStrategy(v) },
		},
		{
			sigpb.DigitallySigned_HashAlgorithm_value,
			opts.hashAlgorithm,
			func(v int32, t *trillian.Tree) { t.HashAlgorithm = sigpb.DigitallySigned_HashAlgorithm(v) },
		},
		{
			sigpb.DigitallySigned_SignatureAlgorithm_value,
			opts.sigAlgorithm,
			func(v int32, t *trillian.Tree) { t.SignatureAlgorithm = sigpb.DigitallySigned_SignatureAlgorithm(v) },
		},
		{
			trillian.DuplicatePolicy_value,
			opts.duplicatePolicy,
			func(v int32, t *trillian.Tree) { t.DuplicatePolicy = trillian.DuplicatePolicy(v) },
		},
	}
	for _, e := range enums {
		value, ok := e.values[e.name]
		if !ok {
			return nil, fmt.Errorf("unknown enum value: %v", e.name)
		}
		e.assign(value, tree)
	}

	pk, err := newPkFromFlags(opts)
	if err != nil {
		return nil, err
	}
	tree.PrivateKey = pk

	return &trillian.CreateTreeRequest{Tree: tree}, nil
}

func newPkFromFlags(opts *createOpts) (*any.Any, error) {
	switch opts.privateKeyType {
	case "PEMKeyFile":
		path := opts.pemKeyPath
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("error reading PEM key file at %v: %v", path, err)
		}
		pass := opts.pemKeyPass
		if pass == "" {
			return nil, errors.New("empty PEM key password")
		}
		pemKey := &trillian.PEMKeyFile{
			Path:     path,
			Password: pass,
		}
		return ptypes.MarshalAny(pemKey)
	default:
		return nil, fmt.Errorf("unknown private key type: %v", opts.privateKeyType)
	}
}

func newOptsFromFlags() *createOpts {
	return &createOpts{
		endpoint:        *adminEndpoint,
		treeState:       *treeState,
		treeType:        *treeType,
		hashStrategy:    *hashStrategy,
		hashAlgorithm:   *hashAlgorithm,
		sigAlgorithm:    *signatureAlgorithm,
		duplicatePolicy: *duplicatePolicy,
		displayName:     *displayName,
		description:     *description,
		privateKeyType:  *privateKeyType,
		pemKeyPath:      *pemKeyPath,
		pemKeyPass:      *pemKeyPassword,
	}
}

func main() {
	flag.Parse()
	if _, err := createTree(newOptsFromFlags()); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create tree: %v\n", err)
		os.Exit(1)
	}
}
