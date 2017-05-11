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

package integration

import (
	"context"
	"crypto"
	"database/sql"
	"net"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql" // Load MySQL driver

	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/server"
	"github.com/google/trillian/storage/sql/coresql"
	"github.com/google/trillian/storage/sql/mysql"
	stestonly "github.com/google/trillian/storage/testonly"
	"github.com/google/trillian/testonly"
	"github.com/google/trillian/util"
	"google.golang.org/grpc"
)

var (
	sequencerWindow = time.Duration(0)
	batchSize       = 50
	// SequencerInterval is the time between runs of the sequencer.
	SequencerInterval = 100 * time.Millisecond
	timeSource        = util.SystemTimeSource{}
	publicKeyPath     = testonly.RelativeToPackage("../../testdata/log-rpc-server.pubkey.pem")
	privateKeyInfo    = &keyspb.PEMKeyFile{
		Path:     testonly.RelativeToPackage("../../testdata/log-rpc-server.privkey.pem"),
		Password: "towel",
	}
)

// LogEnv is a test environment that contains both a log server and a connection to it.
type LogEnv struct {
	registry        extension.Registry
	pendingTasks    *sync.WaitGroup
	grpcServer      *grpc.Server
	logServer       *server.TrillianLogRPCServer
	LogOperation    server.LogOperation
	Sequencer       *server.LogOperationManager
	sequencerCancel context.CancelFunc
	ClientConn      *grpc.ClientConn
	DB              *sql.DB
	// PublicKey is the public key that verifies responses from this server.
	PublicKey crypto.PublicKey
}

// listen opens a random high numbered port for listening.
func listen() (string, net.Listener, error) {
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		return "", nil, err
	}
	_, port, err := net.SplitHostPort(lis.Addr().String())
	if err != nil {
		return "", nil, err
	}
	addr := "localhost:" + port
	return addr, lis, nil
}

// NewLogEnv creates a fresh DB, log server, and client. The numSequencers parameter
// indicates how many sequencers to run in parallel; if numSequencers is zero a
// manually-controlled test sequencer is used.
// testID should be unique to each unittest package so as to allow parallel tests.
func NewLogEnv(ctx context.Context, numSequencers int, testID string) (*LogEnv, error) {
	db, err := GetTestDB(testID)
	if err != nil {
		return nil, err
	}

	registry := extension.Registry{
		AdminStorage:  coresql.NewAdminStorage(mysql.NewWrapper(db)),
		SignerFactory: keys.PEMSignerFactory{},
		LogStorage:    coresql.NewLogStorage(mysql.NewWrapper(db)),
	}

	// Create Log Server.
	grpcServer := grpc.NewServer()
	logServer := server.NewTrillianLogRPCServer(registry, timeSource)
	trillian.RegisterTrillianLogServer(grpcServer, logServer)

	// Create Sequencer.
	sequencerManager := server.NewSequencerManager(registry, sequencerWindow)
	var wg sync.WaitGroup
	var sequencerTask *server.LogOperationManager
	ctx, cancel := context.WithCancel(ctx)
	info := server.LogOperationInfo{
		Registry:    registry,
		BatchSize:   batchSize,
		NumWorkers:  numSequencers,
		RunInterval: SequencerInterval,
		TimeSource:  timeSource,
	}
	// Start a live sequencer in a goroutine.
	sequencerTask = server.NewLogOperationManager(info, sequencerManager)
	wg.Add(1)
	go func(wg *sync.WaitGroup, om *server.LogOperationManager) {
		defer wg.Done()
		om.OperationLoop(ctx)
	}(&wg, sequencerTask)

	// Listen and start server.
	addr, lis, err := listen()
	if err != nil {
		cancel()
		return nil, err
	}
	wg.Add(1)
	go func(wg *sync.WaitGroup, grpcServer *grpc.Server, lis net.Listener) {
		defer wg.Done()
		grpcServer.Serve(lis)
	}(&wg, grpcServer, lis)

	// Connect to the server.
	cc, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		cancel()
		return nil, err
	}

	publicKey, err := keys.NewFromPublicPEMFile(publicKeyPath)
	if err != nil {
		cancel()
		return nil, err
	}

	return &LogEnv{
		registry:        registry,
		pendingTasks:    &wg,
		grpcServer:      grpcServer,
		logServer:       logServer,
		ClientConn:      cc,
		DB:              db,
		PublicKey:       publicKey,
		LogOperation:    sequencerManager,
		Sequencer:       sequencerTask,
		sequencerCancel: cancel,
	}, nil
}

// Close shuts down the server.
func (env *LogEnv) Close() {
	if env.sequencerCancel != nil {
		env.sequencerCancel()
	}
	env.ClientConn.Close()
	env.grpcServer.GracefulStop()
	env.pendingTasks.Wait()
	env.DB.Close()
}

// CreateLog creates a log and signs the first empty tree head.
func (env *LogEnv) CreateLog() (int64, error) {
	ctx := context.Background()
	tx, err := env.registry.AdminStorage.Begin(ctx)
	if err != nil {
		return 0, err
	}

	tree := stestonly.LogTree
	tree.PrivateKey, err = ptypes.MarshalAny(privateKeyInfo)
	if err != nil {
		return 0, err
	}

	tree, err = tx.CreateTree(ctx, tree)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	// Sign the first empty tree head.
	env.Sequencer.OperationSingle(ctx)
	return tree.TreeId, nil
}
