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

package rpcflags

import (
	"context"
	"encoding/pem"
	"flag"
	"os"
	"testing"

	"github.com/google/trillian"
	"github.com/google/trillian/testonly/flagsaver"
	"github.com/google/trillian/testonly/integration"
	"github.com/google/trillian/testonly/setup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func TestNewClientDialOptionsFromFlagsWithTLSCertFileNotSet(t *testing.T) {
	// Set up Trillian servers
	const numSequencers = 2
	serverOpts := []grpc.ServerOption{}
	clientOpts := []grpc.DialOption{grpc.WithInsecure()}
	logEnv, err := integration.NewLogEnvWithGRPCOptions(context.Background(), numSequencers, serverOpts, clientOpts)
	if err != nil {
		t.Fatal(err)
	}
	defer logEnv.Close()

	dialOpts, err := NewClientDialOptionsFromFlags()
	if err != nil {
		t.Errorf("Got an unexpected error: %v", err)
	}

	// Check that the returned dial options can be used to connect and make
	// requests against the Trillian services created above.
	conn, err := grpc.Dial(logEnv.Address, dialOpts...)
	if err != nil {
		t.Errorf("failed to dial %v: %v", logEnv.Address, err)
	}
	defer conn.Close()

	adminClient := trillian.NewTrillianAdminClient(conn)
	if _, err = adminClient.ListTrees(context.Background(), &trillian.ListTreesRequest{}); err != nil {
		t.Errorf("failed to request trees from the Admin Server: %v", err)
	}
}

func TestNewClientDialOptionsFromFlagsWithTLSCertFileMissing(t *testing.T) {
	defer flagsaver.Save().MustRestore()
	if err := flag.Set("tls_cert_file", "/a/missing/file"); err != nil {
		t.Errorf("Failed to set flag: %v", err)
	}

	dialOpts, err := NewClientDialOptionsFromFlags()
	if err == nil {
		t.Errorf("Expected to get an error due to the file not being found")
	}

	if _, ok := err.(*os.PathError); !ok {
		t.Errorf("Expected to get an os.PathError due to the file not being found, instead got: %v", err)
	}

	if dialOpts != nil {
		t.Errorf("Expected returned dialOpts to be nil, instead got: %v", dialOpts)
	}
}

func TestNewClientDialOptionsFromFlagsWithTLSCertFileSet(t *testing.T) {
	// Create new TLS certificates for the test services, and write the Client
	// certificate to a file (so we can refer to it using the flag).
	crtFile, cleanupCrtFile := setup.TempFile(t, "test.crt.")
	defer cleanupCrtFile()

	tlsCert := setup.NewTLSCertificate(t)

	// Certificate file and client dial options.
	err := pem.Encode(crtFile, &pem.Block{Type: "CERTIFICATE", Bytes: tlsCert.Certificate[0]})
	if err != nil {
		t.Fatalf("Failed to encode the test TLS certificate %v", err)
	}
	clientCreds, err := credentials.NewClientTLSFromFile(crtFile.Name(), "")
	if err != nil {
		t.Fatalf("Failed to get credentials: %v", err)
	}

	// Set up Trillian servers (with TLS enabled)
	const numSequencers = 0 // we don't actually need any sequencers.
	serverCreds := credentials.NewServerTLSFromCert(&tlsCert)
	serverOpts := []grpc.ServerOption{grpc.Creds(serverCreds)}
	clientOpts := []grpc.DialOption{grpc.WithTransportCredentials(clientCreds)}
	logEnv, err := integration.NewLogEnvWithGRPCOptions(context.Background(), numSequencers, serverOpts, clientOpts)
	if err != nil {
		t.Fatal(err)
	}
	defer logEnv.Close()

	// Set up the flag.
	defer flagsaver.Save().MustRestore()
	err = flag.Set("tls_cert_file", crtFile.Name())
	if err != nil {
		t.Errorf("Failed to set -tls_cert_file flag: %v", err)
	}

	dialOpts, err := NewClientDialOptionsFromFlags()
	if err != nil {
		t.Errorf("Got an unexpected error: %v", err)
	}

	// Check that the returned dial options can be used to connect and make
	// requests against the Trillian services created above.
	conn, err := grpc.Dial(logEnv.Address, dialOpts...)
	if err != nil {
		t.Errorf("failed to dial %v: %v", logEnv.Address, err)
	}
	defer conn.Close()

	adminClient := trillian.NewTrillianAdminClient(conn)
	if _, err := adminClient.ListTrees(context.Background(), &trillian.ListTreesRequest{}); err != nil {
		t.Errorf("failed to request trees from the Admin Server: %v", err)
	}
}
