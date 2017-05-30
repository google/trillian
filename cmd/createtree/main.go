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
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/golang/glog"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/google/trillian"
	"github.com/google/trillian/cmd"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/letsencrypt/pkcs11key"
	"google.golang.org/grpc"
)

var (
	adminServerAddr = flag.String("admin_server", "", "Address of the gRPC Trillian Admin Server (host:port)")

	treeState          = flag.String("tree_state", trillian.TreeState_ACTIVE.String(), "State of the new tree")
	treeType           = flag.String("tree_type", trillian.TreeType_LOG.String(), "Type of the new tree")
	hashStrategy       = flag.String("hash_strategy", trillian.HashStrategy_RFC6962_SHA256.String(), "Hash strategy (aka preimage protection) of the new tree")
	hashAlgorithm      = flag.String("hash_algorithm", sigpb.DigitallySigned_SHA256.String(), "Hash algorithm of the new tree")
	signatureAlgorithm = flag.String("signature_algorithm", sigpb.DigitallySigned_RSA.String(), "Signature algorithm of the new tree")
	displayName        = flag.String("display_name", "", "Display name of the new tree")
	description        = flag.String("description", "", "Description of the new tree")
	maxRootDuration    = flag.Duration("max_root_duration", 0, "Interval after which a new signed root is produced despite no submissions; zero means never")

	privateKeyFormat = flag.String("private_key_format", "PrivateKey", "Type of private key to be used (PrivateKey, PEMKeyFile, or PKCS11ConfigFile)")
	pemKeyPath       = flag.String("pem_key_path", "", "Path to the private key PEM file")
	pemKeyPassword   = flag.String("pem_key_password", "", "Password of the private key PEM file")
	pkcs11ConfigPath = flag.String("pkcs11_config_path", "", "Path to the PKCS #11 key configuration file")

	configFile = flag.String("config", "", "Config file containing flags, file contents can be overridden by command line flags")
)

// createOpts contains all user-supplied options required to run the program.
// It's meant to facilitate tests and focus flag reads to a single point.
type createOpts struct {
	addr                                                                                     string
	treeState, treeType, hashStrategy, hashAlgorithm, sigAlgorithm, displayName, description string
	maxRootDuration                                                                          time.Duration
	privateKeyType, pemKeyPath, pemKeyPass, pkcs11ConfigPath                                 string
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

	ctr := &trillian.CreateTreeRequest{Tree: &trillian.Tree{
		TreeState:          trillian.TreeState(ts),
		TreeType:           trillian.TreeType(tt),
		HashStrategy:       trillian.HashStrategy(hs),
		HashAlgorithm:      sigpb.DigitallySigned_HashAlgorithm(ha),
		SignatureAlgorithm: sigpb.DigitallySigned_SignatureAlgorithm(sa),
		DisplayName:        opts.displayName,
		Description:        opts.description,
		PrivateKey:         pk,
		MaxRootDuration:    ptypes.DurationProto(opts.maxRootDuration),
	}}
	return ctr, nil
}

func newPK(opts *createOpts) (*any.Any, error) {
	switch opts.privateKeyType {
	case "PEMKeyFile":
		if opts.pemKeyPath == "" {
			return nil, errors.New("empty pem_key_path")
		}
		if opts.pemKeyPass == "" {
			return nil, fmt.Errorf("empty password for PEM key file %q", opts.pemKeyPath)
		}
		pemKey := &keyspb.PEMKeyFile{
			Path:     opts.pemKeyPath,
			Password: opts.pemKeyPass,
		}
		return ptypes.MarshalAny(pemKey)
	case "PrivateKey":
		if opts.pemKeyPath == "" {
			return nil, errors.New("empty pem_key_path")
		}
		pemSigner, err := keys.NewFromPrivatePEMFile(
			opts.pemKeyPath, opts.pemKeyPass)
		if err != nil {
			return nil, err
		}
		der, err := keys.MarshalPrivateKey(pemSigner)
		if err != nil {
			return nil, err
		}
		return ptypes.MarshalAny(&keyspb.PrivateKey{Der: der})
	case "PKCS11ConfigFile":
		if opts.pkcs11ConfigPath == "" {
			return nil, errors.New("empty PKCS11 config file path")
		}
		configBytes, err := ioutil.ReadFile(opts.pkcs11ConfigPath)
		if err != nil {
			return nil, err
		}
		var config pkcs11key.Config
		if err = json.Unmarshal(configBytes, &config); err != nil {
			return nil, err
		}
		return ptypes.MarshalAny(&keyspb.PKCS11Config{
			TokenLabel:      config.TokenLabel,
			Pin:             config.PIN,
			PrivateKeyLabel: config.PrivateKeyLabel,
		})
	default:
		return nil, fmt.Errorf("unknown private key type: %v", opts.privateKeyType)
	}
}

func newOptsFromFlags() *createOpts {
	return &createOpts{
		addr:             *adminServerAddr,
		treeState:        *treeState,
		treeType:         *treeType,
		hashStrategy:     *hashStrategy,
		hashAlgorithm:    *hashAlgorithm,
		sigAlgorithm:     *signatureAlgorithm,
		displayName:      *displayName,
		description:      *description,
		maxRootDuration:  *maxRootDuration,
		privateKeyType:   *privateKeyFormat,
		pemKeyPath:       *pemKeyPath,
		pemKeyPass:       *pemKeyPassword,
		pkcs11ConfigPath: *pkcs11ConfigPath,
	}
}

func main() {
	flag.Parse()

	if *configFile != "" {
		if err := cmd.ParseFlagFile(*configFile); err != nil {
			glog.Exitf("Failed to load flags from config file %q: %s", *configFile, err)
		}
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
