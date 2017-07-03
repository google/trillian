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
	"sync"

	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/server"
	"github.com/google/trillian/storage/mysql"
	stestonly "github.com/google/trillian/storage/testonly"
	"google.golang.org/grpc"
)

// MapEnv is a map server and connected client.
type MapEnv struct {
	registry     extension.Registry
	pendingTasks *sync.WaitGroup
	grpcServer   *grpc.Server
	mapServer    *server.TrillianMapServer
	ClientConn   *grpc.ClientConn
	DB           *sql.DB
	Tree         *trillian.Tree
}

// NewMapEnv creates a fresh DB, map server, and client.
func NewMapEnv(ctx context.Context, testID string) (*MapEnv, error) {
	db, err := GetTestDB(testID)
	if err != nil {
		return nil, err
	}

	registry := extension.Registry{
		AdminStorage:  mysql.NewAdminStorage(db),
		MapStorage:    mysql.NewMapStorage(db),
		QuotaManager:  quota.Noop(),
		SignerFactory: &keys.DefaultSignerFactory{},
	}

	ret, err := NewMapEnvWithRegistry(ctx, testID, registry)
	if err != nil {
		return nil, err
	}
	ret.DB = db
	return ret, nil
}

// NewMapEnvWithRegistry uses the passed in Registry to create a map server and
// client.  testID should be unique to each unittest package so as to allow
// parallel tests.
func NewMapEnvWithRegistry(ctx context.Context, testID string, registry extension.Registry) (*MapEnv, error) {

	// Create Map Server.
	grpcServer := grpc.NewServer()
	mapServer := server.NewTrillianMapServer(registry)
	trillian.RegisterTrillianMapServer(grpcServer, mapServer)

	// Listen and start server.
	addr, lis, err := listen()
	if err != nil {
		return nil, err
	}
	go grpcServer.Serve(lis)

	// Connect to the server.
	cc, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	return &MapEnv{
		registry:   registry,
		grpcServer: grpcServer,
		mapServer:  mapServer,
		ClientConn: cc,
	}, nil
}

// Close shuts down the server.
func (env *MapEnv) Close() {
	env.ClientConn.Close()
	env.grpcServer.GracefulStop()
	if env.DB != nil {
		env.DB.Close()
	}
}

// CreateMap creates a map and signs the first empty tree head.
func (env *MapEnv) CreateMap(hashStrategy trillian.HashStrategy) (*trillian.Tree, error) {
	ctx := context.Background()
	tx, err := env.registry.AdminStorage.Begin(ctx)
	if err != nil {
		return nil, err
	}

	tree := stestonly.MapTree
	tree.HashStrategy = hashStrategy
	tree.PrivateKey, err = ptypes.MarshalAny(privateKeyInfo)
	if err != nil {
		return nil, err
	}

	tree, err = tx.CreateTree(ctx, tree)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return tree, nil
}
