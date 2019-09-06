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

// Package main contains the implementation and entry point for the
// get_tree_public_key command.
//
// Example usage:
// $ ./get_tree_public_key --admin_server=host:port --log_id=logid
package main

import (
	"context"
	"encoding/pem"
	"flag"
	"fmt"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/client/rpcflags"
	"google.golang.org/grpc"
)

var (
	adminServerAddr = flag.String("admin_server", "", "Address of the gRPC Trillian Admin Server (host:port)")
	logID           = flag.Int64("log_id", 0, "Trillian LogID whose public key should be fetched")
)

func getPublicKeyPEM() (string, error) {
	flag.Parse()
	defer glog.Flush()

	dialOpts, err := rpcflags.NewClientDialOptionsFromFlags()
	if err != nil {
		return "", fmt.Errorf("failed to determine dial options: %w", err)
	}

	conn, err := grpc.Dial(*adminServerAddr, dialOpts...)
	if err != nil {
		return "", fmt.Errorf("failed to dial %v: %w", *adminServerAddr, err)
	}
	defer conn.Close()

	a := trillian.NewTrillianAdminClient(conn)
	tree, err := a.GetTree(context.Background(), &trillian.GetTreeRequest{TreeId: *logID})
	if err != nil {
		return "", fmt.Errorf("call to GetTree failed: %w", err)
	}

	if tree == nil {
		return "", fmt.Errorf("log %d not found", *logID)
	}

	publicKey := tree.GetPublicKey()
	if publicKey == nil || len(publicKey.GetDer()) == 0 {
		return "", fmt.Errorf("log %d does not have a public key", *logID)
	}

	publicKeyPEM := string(pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKey.GetDer(),
	}))

	return publicKeyPEM, nil
}

func main() {
	publicKeyPEM, err := getPublicKeyPEM()
	if err != nil {
		glog.Exitf("Could not get public key: %v", err)
	}

	// Do not add a newline (ex. using fmt.Println) because publicKeyPEM already
	// includes a trailing new line, and adding an extra line will result in the
	// PEM file being interpreted as having an extra PEM block, which can cause
	// errors.
	fmt.Print(publicKeyPEM)
}
