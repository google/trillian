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
// $ ./createtree --admin_server=host:port
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

	privateKeyFormat = flag.String("private_key_format", "", "Type of protobuf message to send the key as (PrivateKey, PEMKeyFile, or PKCS11ConfigFile). If empty, a key will be generated for you by Trillian.")
	pemKeyPath       = flag.String("pem_key_path", "", "Path to the private key PEM file")
	pemKeyPassword   = flag.String("pem_key_password", "", "Password of the private key PEM file")
	pkcs11ConfigPath = flag.String("pkcs11_config_path", "", "Path to the PKCS #11 key configuration file")

	configFile = flag.String("config", "", "Config file containing flags, file contents can be overridden by command line flags")
)

func createTree(ctx context.Context) (*trillian.Tree, error) {
	if *adminServerAddr == "" {
		return nil, errors.New("empty --admin_server, please provide the Admin server host:port")
	}

	req, err := newRequest()
	if err != nil {
		return nil, err
	}

	conn, err := grpc.Dial(*adminServerAddr, grpc.WithInsecure())
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

func newRequest() (*trillian.CreateTreeRequest, error) {
	ts, ok := trillian.TreeState_value[*treeState]
	if !ok {
		return nil, fmt.Errorf("unknown TreeState: %v", *treeState)
	}

	tt, ok := trillian.TreeType_value[*treeType]
	if !ok {
		return nil, fmt.Errorf("unknown TreeType: %v", *treeType)
	}

	hs, ok := trillian.HashStrategy_value[*hashStrategy]
	if !ok {
		return nil, fmt.Errorf("unknown HashStrategy: %v", *hashStrategy)
	}

	ha, ok := sigpb.DigitallySigned_HashAlgorithm_value[*hashAlgorithm]
	if !ok {
		return nil, fmt.Errorf("unknown HashAlgorithm: %v", *hashAlgorithm)
	}

	sa, ok := sigpb.DigitallySigned_SignatureAlgorithm_value[*signatureAlgorithm]
	if !ok {
		return nil, fmt.Errorf("unknown SignatureAlgorithm: %v", *signatureAlgorithm)
	}

	ctr := &trillian.CreateTreeRequest{Tree: &trillian.Tree{
		TreeState:          trillian.TreeState(ts),
		TreeType:           trillian.TreeType(tt),
		HashStrategy:       trillian.HashStrategy(hs),
		HashAlgorithm:      sigpb.DigitallySigned_HashAlgorithm(ha),
		SignatureAlgorithm: sigpb.DigitallySigned_SignatureAlgorithm(sa),
		DisplayName:        *displayName,
		Description:        *description,
		MaxRootDuration:    ptypes.DurationProto(*maxRootDuration),
	}}

	if *privateKeyFormat != "" {
		pk, err := newPK(*privateKeyFormat)
		if err != nil {
			return nil, err
		}
		ctr.Tree.PrivateKey = pk
	} else {
		// Cannot continue if options specifying a key were provided but
		// privateKeyType is not set, as there's no way to know what protobuf
		// message type was intended.
		if *pemKeyPath != "" || *pemKeyPassword != "" || *pkcs11ConfigPath != "" {
			return nil, errors.New("must specify private key format")
		}

		// If no key flags were provided at all, get Trillian to generate a key.
		ctr.KeySpec = &keyspb.Specification{}

		switch sigpb.DigitallySigned_SignatureAlgorithm(sa) {
		case sigpb.DigitallySigned_ECDSA:
			ctr.KeySpec.Params = &keyspb.Specification_EcdsaParams{
				EcdsaParams: &keyspb.Specification_ECDSA{},
			}
		case sigpb.DigitallySigned_RSA:
			ctr.KeySpec.Params = &keyspb.Specification_RsaParams{
				RsaParams: &keyspb.Specification_RSA{},
			}
		default:
			return nil, fmt.Errorf("unsupported signature algorithm: %v", sa)
		}
	}

	return ctr, nil
}

func newPK(keyFormat string) (*any.Any, error) {
	switch keyFormat {
	case "PEMKeyFile":
		if *pemKeyPath == "" {
			return nil, errors.New("empty pem_key_path")
		}
		if *pemKeyPassword == "" {
			return nil, fmt.Errorf("empty password for PEM key file %q", *pemKeyPath)
		}
		pemKey := &keyspb.PEMKeyFile{
			Path:     *pemKeyPath,
			Password: *pemKeyPassword,
		}
		return ptypes.MarshalAny(pemKey)
	case "PrivateKey":
		if *pemKeyPath == "" {
			return nil, errors.New("empty pem_key_path")
		}
		pemSigner, err := keys.NewFromPrivatePEMFile(
			*pemKeyPath, *pemKeyPassword)
		if err != nil {
			return nil, err
		}
		der, err := keys.MarshalPrivateKey(pemSigner)
		if err != nil {
			return nil, err
		}
		return ptypes.MarshalAny(&keyspb.PrivateKey{Der: der})
	case "PKCS11ConfigFile":
		if *pkcs11ConfigPath == "" {
			return nil, errors.New("empty PKCS11 config file path")
		}
		configBytes, err := ioutil.ReadFile(*pkcs11ConfigPath)
		if err != nil {
			return nil, err
		}
		var config pkcs11key.Config
		if err = json.Unmarshal(configBytes, &config); err != nil {
			return nil, err
		}
		pubKeyBytes, err := ioutil.ReadFile(config.PublicKeyPath)
		if err != nil {
			return nil, err
		}
		return ptypes.MarshalAny(&keyspb.PKCS11Config{
			TokenLabel: config.TokenLabel,
			Pin:        config.PIN,
			PublicKey:  string(pubKeyBytes),
		})
	default:
		return nil, fmt.Errorf("unknown private key type: %v", keyFormat)
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
	tree, err := createTree(ctx)
	if err != nil {
		glog.Exitf("Failed to create tree: %v", err)
	}

	// DO NOT change the output format, scripts are meant to depend on it.
	// If you really want to change it, provide an output_format flag and
	// keep the default as-is.
	fmt.Println(tree.TreeId)
}
