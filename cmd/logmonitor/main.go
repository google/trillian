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

// The logmonitor binary monitors a Trillian log to verify that it is satisfying
// the append-only property.
package main

import (
	"context"
	"crypto"
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/client"
	"github.com/google/trillian/client/rpcflags"
	"github.com/google/trillian/cmd"
	"github.com/google/trillian/crypto/keys/pem"
	"github.com/google/trillian/merkle/objhasher"
	"github.com/google/trillian/merkle/rfc6962"
	"github.com/google/trillian/types"
	"google.golang.org/grpc"
)

var supportedDatabases = []string{
	"mssql",
	"mysql",
	"postgres",
	"sqlite3",
}

// Flags
var (
	serverAddr       = flag.String("server", "", "Address of the gRPC Trillian Server (host:port)")
	logID            = flag.Int64("log_id", 0, "The ID of the Trillian log to be monitored")
	logPublicKeyFile = flag.String("log_public_key", "", "The RSA public key for the Trillian log to be monitored")

	// Database flags
	database             = flag.String("database", "sqlite3", fmt.Sprintf("Database to use. One of: %v", strings.Join(supportedDatabases, ", ")))
	databaseURI          = flag.String("database_uri", "", "The database connection URI")
	waitBetweenDBUpdates = flag.Duration("database_updates", 5*time.Second, "How long to wait between database updates")

	configFile = flag.String("config", "", "Config file containing flags, file contents can be overridden by command line flags")
)

// Errors
var (
	errServerMissing       = errors.New("server is missing: use the -server flag to set it")
	errLogIDMissing        = errors.New("log ID is missing: use the -log_id flag to set it")
	errPublicKeyMissing    = errors.New("the Trillian log public key path is missing: use the -log_public_key flag to set it")
	errDatabaseMissing     = fmt.Errorf("database is missing: use the -database flag to set it to one of: %v", strings.Join(supportedDatabases, ", "))
	errUnsupportedDatabase = fmt.Errorf("selected database set by the -database flag is unsupported: use one of: %v", strings.Join(supportedDatabases, ", "))
)

func verifyFlags() error {
	if *serverAddr == "" {
		return errServerMissing
	}

	if *logID == 0 {
		return errLogIDMissing
	}

	if *logPublicKeyFile == "" {
		return errPublicKeyMissing
	}

	if *database == "" {
		return errDatabaseMissing
	}

	databaseIsSupported := false
	for _, d := range supportedDatabases {
		if *database == d {
			databaseIsSupported = true
			break
		}
	}
	if !databaseIsSupported {
		return errUnsupportedDatabase
	}

	return nil
}

func newLogMonitorFromFlags() (*LogMonitor, error) {
	if err := verifyFlags(); err != nil {
		return nil, err
	}

	db, err := Open(*database, *databaseURI)
	if err != nil {
		return nil, err
	}

	if err := Setup(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	startingRoot, err := GetCurrentRootFor(db, *logID)
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	logClient, err := newLogClientFromFlags(*startingRoot)
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	monitor := &LogMonitor{
		logID:                *logID,
		logClient:            logClient,
		db:                   db,
		waitBetweenDBUpdates: *waitBetweenDBUpdates,
	}
	return monitor, nil
}

func newLogClientFromFlags(startingRoot types.LogRootV1) (*client.LogClient, error) {
	publicKey, err := pem.ReadPublicKeyFile(*logPublicKeyFile)
	if err != nil {
		return nil, err
	}

	dialOpts, err := rpcflags.NewClientDialOptionsFromFlags()
	if err != nil {
		return nil, fmt.Errorf("failed to determine dial options: %v", err)
	}

	conn, err := grpc.Dial(*serverAddr, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to dial %s: %v", *serverAddr, err)
	}

	grpcClient := trillian.NewTrillianLogClient(conn)
	hasher := objhasher.NewLogHasher(rfc6962.New(crypto.SHA256))
	verifier := client.NewLogVerifier(hasher, publicKey, crypto.SHA256)
	return client.New(*logID, grpcClient, verifier, startingRoot), nil
}

func runMonitor(ctx context.Context) error {
	flag.Parse()

	if *configFile != "" {
		if err := cmd.ParseFlagFile(*configFile); err != nil {
			return fmt.Errorf("Failed to load flags from config file %q: %s", *configFile, err)
		}
	}

	m, err := newLogMonitorFromFlags()
	if err != nil {
		return fmt.Errorf("Failed to initialize monitor: %v", err)
	}

	if err := m.Run(ctx); err != nil {
		return fmt.Errorf("Encountered an error: %v", err)
	}

	return nil
}

func main() {
	defer glog.Flush()
	err := runMonitor(context.Background())
	if err != nil {
		glog.Exit(err)
	}
}
