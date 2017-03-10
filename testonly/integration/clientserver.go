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
	"database/sql"
	"fmt"
	"io/ioutil"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/extension/builtin"
	"github.com/google/trillian/server"
	"github.com/google/trillian/storage/mysql"
	"github.com/google/trillian/storage/testonly"
	to "github.com/google/trillian/testonly"
	"github.com/google/trillian/util"
	"google.golang.org/grpc"
)

const (
	createSQLFile = "../storage/mysql/storage.sql"
	mysqlRootURI  = "root@tcp(127.0.0.1:3306)/"
)

var (
	sequencerWindow  = time.Duration(0)
	batchSize        = 50
	sleepBetweenRuns = 100 * time.Millisecond
	timeSource       = util.SystemTimeSource{}
	// PublicKey returns the public key that verifies responses from this server.
	PublicKey, _ = keys.NewFromPublicPEM(to.DemoPublicKey)
)

// LogEnv is a test environment that contains both a log server and a connection to it.
type LogEnv struct {
	pendingTasks    *sync.WaitGroup
	grpcServer      *grpc.Server
	logServer       *server.TrillianLogRPCServer
	LogOperation    server.LogOperation
	Sequencer       *server.LogOperationManager
	sequencerCancel context.CancelFunc
	ClientConn      *grpc.ClientConn
	DB              *sql.DB
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

// getTestDB drops and recreates the test database.
// Returns a database connection to the test database.
func getTestDB(testID string) (*sql.DB, error) {
	var testDBURI = fmt.Sprintf("root@tcp(127.0.0.1:3306)/log_unittest_%v", testID)

	// Drop existing database.
	dbRoot, err := sql.Open("mysql", mysqlRootURI)
	if err != nil {
		return nil, err
	}
	defer dbRoot.Close()
	resetSQL := []string{
		fmt.Sprintf("DROP DATABASE IF EXISTS log_unittest_%v;", testID),
		fmt.Sprintf("CREATE DATABASE log_unittest_%v;", testID),
		fmt.Sprintf("GRANT ALL ON log_unittest_%v.* TO 'log_unittest'@'localhost' IDENTIFIED BY 'zaphod';", testID),
	}
	for _, sql := range resetSQL {
		if _, err := dbRoot.Exec(sql); err != nil {
			return nil, err
		}
	}

	// Create new database.
	dbTest, err := sql.Open("mysql", testDBURI)
	if err != nil {
		return nil, err
	}
	createSQL, err := ioutil.ReadFile(createSQLFile)
	if err != nil {
		return nil, err
	}
	sqlSlice := strings.Split(string(createSQL), ";\n")
	// Omit the last element of the slice, since it will be "".
	for _, sql := range sqlSlice[:len(sqlSlice)-1] {
		if _, err := dbTest.Exec(sql); err != nil {
			return nil, err
		}
	}

	return dbTest, nil
}

// NewLogEnv creates a fresh DB, log server, and client. The numSequencers parameter
// indicates how many sequencers to run in parallel; if numSequencers is zero a
// manually-controlled test sequencer is used.
// testID should be unique to each unittest package so as to allow parallel tests.
func NewLogEnv(ctx context.Context, numSequencers int, testID string) (*LogEnv, error) {
	db, err := getTestDB(testID)
	if err != nil {
		return nil, err
	}

	key, err := keys.NewFromPrivatePEM(to.DemoPrivateKey, to.DemoPrivateKeyPass)
	if err != nil {
		return nil, err
	}

	registry, err := builtin.NewExtensionRegistry(db, crypto.NewSigner(key))
	if err != nil {
		return nil, err
	}

	// Create Log Server.
	grpcServer := grpc.NewServer()
	logServer := server.NewTrillianLogRPCServer(registry, timeSource)
	trillian.RegisterTrillianLogServer(grpcServer, logServer)

	// Create Sequencer.
	sequencerManager := server.NewSequencerManager(registry, sequencerWindow)
	var wg sync.WaitGroup
	var sequencerTask *server.LogOperationManager
	var cancel context.CancelFunc
	if numSequencers == 0 {
		// Test sequencer that needs manual triggering (with env.Sequencer.OperationLoop()).
		sequencerTask = server.NewLogOperationManagerForTest(ctx, registry,
			batchSize, sleepBetweenRuns, timeSource, sequencerManager)
	} else {
		// Start a live sequencer in a goroutine.
		var ctx2 context.Context
		ctx2, cancel = context.WithCancel(ctx)
		sequencerTask = server.NewLogOperationManager(ctx2, registry,
			batchSize, numSequencers, sleepBetweenRuns, timeSource, sequencerManager)
		wg.Add(1)
		go func(wg *sync.WaitGroup, om *server.LogOperationManager) {
			defer wg.Done()
			om.OperationLoop()
		}(&wg, sequencerTask)
	}

	// Listen and start server.
	addr, lis, err := listen()
	if err != nil {
		if cancel != nil {
			cancel()
		}
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
		if cancel != nil {
			cancel()
		}
		return nil, err
	}
	return &LogEnv{
		pendingTasks:    &wg,
		grpcServer:      grpcServer,
		logServer:       logServer,
		ClientConn:      cc,
		DB:              db,
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
	s := mysql.NewAdminStorage(env.DB)
	ctx := context.Background()
	tx, err := s.Begin(ctx)
	if err != nil {
		return 0, err
	}
	tree, err := tx.CreateTree(ctx, testonly.LogTree)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	// Sign the first empty tree head.
	env.Sequencer.OperationSingle()
	return tree.TreeId, nil
}
