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

// Package main contains the implementation and entry point for the createtree
// command.
//
// Example usage:
// $ ./createtree \
//     --admin_server=host:port \
//     --pem_key_path=/path/to/pem/file \
//     --pem_key_password=mypassword
//
// The command outputs the tree ID of the created tree to stdout, or an error to
// stderr in case of failure. The output is minimal to allow for easy usage in
// automated scripts.
//
// Several flags are provided to configure the create tree, most of which try to
// assume reasonable defaults. Multiple types of private keys may be supported;
// one has only to set the appropriate --private_key_format value and supply the
// corresponding flags for the chosen key type.
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
	"github.com/google/trillian/cmd"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/google/trillian/crypto/sigpb"
	"google.golang.org/grpc"
)

var (
	configFile      = flag.String("config", "", "Config file containing flags")
	adminServerAddr = flag.String("admin_server", "", "Address of the gRPC Trillian Admin Server (host:port)")

	treeState          = flag.String("tree_state", trillian.TreeState_ACTIVE.String(), "State of the new tree")
	treeType           = flag.String("tree_type", trillian.TreeType_LOG.String(), "Type of the new tree")
	hashStrategy       = flag.String("hash_strategy", trillian.HashStrategy_RFC_6962.String(), "Hash strategy (aka preimage protection) of the new tree")
	hashAlgorithm      = flag.String("hash_algorithm", sigpb.DigitallySigned_SHA256.String(), "Hash algorithm of the new tree")
	signatureAlgorithm = flag.String("signature_algorithm", sigpb.DigitallySigned_RSA.String(), "Signature algorithm of the new tree")
	displayName        = flag.String("display_name", "", "Display name of the new tree")
	description        = flag.String("description", "", "Description of the new tree")

	privateKeyFormat = flag.String("private_key_format", "PEMKeyFile", "Type of private key to be used")
	pemKeyPath       = flag.String("pem_key_path", "", "Path to the private key PEM file")
	pemKeyPassword   = flag.String("pem_key_password", "", "Password of the private key PEM file")
)

// createOpts contains all user-supplied options required to run the program.
// It's meant to facilitate tests and focus flag reads to a single point.
type createOpts struct {
	addr                                                                                     string
	treeState, treeType, hashStrategy, hashAlgorithm, sigAlgorithm, displayName, description string
	privateKeyType, pemKeyPath, pemKeyPass                                                   string
}

func createTree(ctx context.Context, opts *createOpts) (*trillian.Tree, error) {
	if opts.addr == "" {
		return nil, errors.New("empty --admin_server, please provide the Admin server host:port")
	}

	req, err := newRequest(opts)
	if err != nil {
		return nil, err
	}

	conn, err := grpc.Dial(opts.addr, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	tree, err := trillian.NewTrillianAdminClient(conn).CreateTree(ctx, req)
	if err != nil {
		return nil, err
	}
	return tree, nil
}

func newRequest(opts *createOpts) (*trillian.CreateTreeRequest, error) {
	ts, ok := trillian.TreeState_value[opts.treeState]
	if !ok {
		return nil, fmt.Errorf("unknown TreeState: %v", opts.treeState)
	}

	tt, ok := trillian.TreeType_value[opts.treeType]
	if !ok {
		return nil, fmt.Errorf("unknown TreeType: %v", opts.treeType)
	}

	hs, ok := trillian.HashStrategy_value[opts.hashStrategy]
	if !ok {
		return nil, fmt.Errorf("unknown HashStrategy: %v", opts.hashStrategy)
	}

	ha, ok := sigpb.DigitallySigned_HashAlgorithm_value[opts.hashAlgorithm]
	if !ok {
		return nil, fmt.Errorf("unknown HashAlgorithm: %v", opts.hashAlgorithm)
	}

	sa, ok := sigpb.DigitallySigned_SignatureAlgorithm_value[opts.sigAlgorithm]
	if !ok {
		return nil, fmt.Errorf("unknown SignatureAlgorithm: %v", opts.sigAlgorithm)
	}

	pk, err := newPK(opts)
	if err != nil {
		return nil, err
	}

	tree := &trillian.Tree{
		TreeState:          trillian.TreeState(ts),
		TreeType:           trillian.TreeType(tt),
		HashStrategy:       trillian.HashStrategy(hs),
		HashAlgorithm:      sigpb.DigitallySigned_HashAlgorithm(ha),
		SignatureAlgorithm: sigpb.DigitallySigned_SignatureAlgorithm(sa),
		DisplayName:        opts.displayName,
		Description:        opts.description,
		PrivateKey:         pk,
	}
	return &trillian.CreateTreeRequest{Tree: tree}, nil
}

func newPK(opts *createOpts) (*any.Any, error) {
	switch opts.privateKeyType {
	case "PEMKeyFile":
		if opts.pemKeyPath == "" {
			return nil, errors.New("empty PEM path")
		}
		if opts.pemKeyPass == "" {
			return nil, fmt.Errorf("empty password for PEM key file %q", opts.pemKeyPath)
		}
		pemKey := &keyspb.PEMKeyFile{
			Path:     opts.pemKeyPath,
			Password: opts.pemKeyPass,
		}
		return ptypes.MarshalAny(pemKey)
	default:
		return nil, fmt.Errorf("unknown private key type: %v", opts.privateKeyType)
	}
}

func newOptsFromFlags() *createOpts {
	return &createOpts{
		addr:           *adminServerAddr,
		treeState:      *treeState,
		treeType:       *treeType,
		hashStrategy:   *hashStrategy,
		hashAlgorithm:  *hashAlgorithm,
		sigAlgorithm:   *signatureAlgorithm,
		displayName:    *displayName,
		description:    *description,
		privateKeyType: *privateKeyFormat,
		pemKeyPath:     *pemKeyPath,
		pemKeyPass:     *pemKeyPassword,
	}
}

func main() {
	flag.Parse()

	if *configFile != "" {
		if flag.NFlag() != 1 {
			fmt.Printf("No other flags can be provided when --config is set")
			os.Exit(1)
		}

		if err := cmd.ParseFlagFile(*configFile); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to parse %v: %v\n", *configFile, err)
			os.Exit(1)
		}

		// Alternative to printing error if more than just "--config" flag is provided:
		// let command-line flags take precedent by re-parsing from the command-line.
		// flag.Parse()
	}

	ctx := context.Background()
	tree, err := createTree(ctx, newOptsFromFlags())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create tree: %v\n", err)
		os.Exit(1)
	}

	// DO NOT change the output format, scripts are meant to depend on it.
	// If you really want to change it, provide an output_format flag and
	// keep the default as-is.
	fmt.Println(tree.TreeId)
}
