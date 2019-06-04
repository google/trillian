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

// Package integration provides test-only code for performing integrated
// tests of Trillian functionality.
package integration

import (
	"context"
	"database/sql"
	"net"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc"

	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys/der"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/log"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/server"
	"github.com/google/trillian/server/admin"
	"github.com/google/trillian/server/interceptor"
	"github.com/google/trillian/storage/mysql"
	"github.com/google/trillian/storage/testdb"
	"github.com/google/trillian/util/clock"

	_ "github.com/go-sql-driver/mysql"                   // Load MySQL driver
	_ "github.com/google/trillian/crypto/keys/der/proto" // Register PrivateKey ProtoHandler
)

var (
	sequencerWindow = time.Duration(0)
	batchSize       = 50
	// SequencerInterval is the time between runs of the sequencer.
	SequencerInterval = 500 * time.Millisecond
	timeSource        = clock.System
)

// LogEnv is a test environment that contains both a log server and a connection to it.
type LogEnv struct {
	registry        extension.Registry
	pendingTasks    *sync.WaitGroup
	grpcServer      *grpc.Server
	adminServer     *admin.Server
	logServer       *server.TrillianLogRPCServer
	LogOperation    log.Operation
	Sequencer       *log.OperationManager
	sequencerCancel context.CancelFunc
	ClientConn      *grpc.ClientConn // TODO(gbelvin): Deprecate.

	Address string
	Log     trillian.TrillianLogClient
	Admin   trillian.TrillianAdminClient
	DB      *sql.DB
}

// NewLogEnv creates a fresh DB, log server, and client. The numSequencers parameter
// indicates how many sequencers to run in parallel; if numSequencers is zero a
// manually-controlled test sequencer is used.
// TODO(codingllama): Remove 3rd parameter (need to coordinate with
// github.com/google/certificate-transparency-go)
func NewLogEnv(ctx context.Context, numSequencers int, _ string) (*LogEnv, error) {
	return NewLogEnvWithGRPCOptions(ctx, numSequencers, nil, nil)
}

// NewLogEnvWithGRPCOptions works the same way as NewLogEnv, but allows callers to also set additional grpc.ServerOption and grpc.DialOption values.
func NewLogEnvWithGRPCOptions(ctx context.Context, numSequencers int, serverOpts []grpc.ServerOption, clientOpts []grpc.DialOption) (*LogEnv, error) {
	db, err := testdb.NewTrillianDB(ctx)
	if err != nil {
		return nil, err
	}

	registry := extension.Registry{
		AdminStorage: mysql.NewAdminStorage(db),
		LogStorage:   mysql.NewLogStorage(db, nil),
		QuotaManager: quota.Noop(),
		NewKeyProto: func(ctx context.Context, spec *keyspb.Specification) (proto.Message, error) {
			return der.NewProtoFromSpec(spec)
		},
	}

	ret, err := NewLogEnvWithRegistryAndGRPCOptions(ctx, numSequencers, registry, serverOpts, clientOpts)
	if err != nil {
		db.Close()
		return nil, err
	}
	ret.DB = db
	return ret, nil
}

// NewLogEnvWithRegistry uses the passed in Registry to create a log server,
// and client. The numSequencers parameter indicates how many sequencers to
// run in parallel; if numSequencers is zero a manually-controlled test
// sequencer is used.
func NewLogEnvWithRegistry(ctx context.Context, numSequencers int, registry extension.Registry) (*LogEnv, error) {
	return NewLogEnvWithRegistryAndGRPCOptions(ctx, numSequencers, registry, nil, nil)
}

// NewLogEnvWithRegistryAndGRPCOptions works the same way as NewLogEnv, but allows callers to also set additional grpc.ServerOption and grpc.DialOption values.
func NewLogEnvWithRegistryAndGRPCOptions(ctx context.Context, numSequencers int, registry extension.Registry, serverOpts []grpc.ServerOption, clientOpts []grpc.DialOption) (*LogEnv, error) {
	// Create the GRPC Server.
	serverOpts = append(serverOpts, grpc.UnaryInterceptor(interceptor.ErrorWrapper))
	grpcServer := grpc.NewServer(serverOpts...)

	// Setup the Admin Server.
	adminServer := admin.New(registry, nil)
	trillian.RegisterTrillianAdminServer(grpcServer, adminServer)

	// Setup the Log Server.
	logServer := server.NewTrillianLogRPCServer(registry, timeSource)
	trillian.RegisterTrillianLogServer(grpcServer, logServer)

	// Create Sequencer.
	sequencerManager := log.NewSequencerManager(registry, sequencerWindow)
	var wg sync.WaitGroup
	var sequencerTask *log.OperationManager
	ctx, cancel := context.WithCancel(ctx)
	info := log.OperationInfo{
		Registry:    registry,
		BatchSize:   batchSize,
		NumWorkers:  numSequencers,
		RunInterval: SequencerInterval,
		TimeSource:  timeSource,
	}
	// Start a live sequencer in a goroutine.
	sequencerTask = log.NewOperationManager(info, sequencerManager)
	wg.Add(1)
	go func(wg *sync.WaitGroup, om *log.OperationManager) {
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
		if err := grpcServer.Serve(lis); err != nil {
			glog.Errorf("gRPC server stopped: %v", err)
			glog.Flush()
		}
	}(&wg, grpcServer, lis)

	// Connect to the server.
	if clientOpts == nil {
		clientOpts = []grpc.DialOption{grpc.WithInsecure()}
	}

	cc, err := grpc.Dial(addr, clientOpts...)
	if err != nil {
		cancel()
		return nil, err
	}

	return &LogEnv{
		registry:        registry,
		pendingTasks:    &wg,
		grpcServer:      grpcServer,
		adminServer:     adminServer,
		logServer:       logServer,
		Address:         addr,
		ClientConn:      cc,
		Log:             trillian.NewTrillianLogClient(cc),
		Admin:           trillian.NewTrillianAdminClient(cc),
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
	if env.DB != nil {
		env.DB.Close()
	}
}
