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

	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys/der"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/server"
	"github.com/google/trillian/server/admin"
	"github.com/google/trillian/server/interceptor"
	"github.com/google/trillian/storage/mysql"
	"github.com/google/trillian/storage/testdb"
	"google.golang.org/grpc"

	_ "github.com/google/trillian/crypto/keys/der/proto" // Register PrivateKey ProtoHandler
)

// MapEnv is a map server and connected client.
type MapEnv struct {
	registry  extension.Registry
	mapServer *server.TrillianMapServer

	// Objects that need Close(), in order of creation.
	DB         *sql.DB
	grpcServer *grpc.Server
	clientConn *grpc.ClientConn

	// Public fields
	Map   trillian.TrillianMapClient
	Admin trillian.TrillianAdminClient
}

// NewMapEnvFromConn connects to a map server.
func NewMapEnvFromConn(addr string) (*MapEnv, error) {
	cc, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	return &MapEnv{
		clientConn: cc,
		Map:        trillian.NewTrillianMapClient(cc),
		Admin:      trillian.NewTrillianAdminClient(cc),
	}, nil
}

// NewMapEnv creates a fresh DB, map server, and client.
func NewMapEnv(ctx context.Context) (*MapEnv, error) {
	if provider := testdb.Default(); !provider.IsMySQL() {
		return nil, fmt.Errorf("map integration test has concurrent writes and needs MySQL. SQL driver is %v", provider.Driver)
	}

	db, err := testdb.NewTrillianDB(ctx)
	if err != nil {
		return nil, err
	}

	registry := extension.Registry{
		AdminStorage:  mysql.NewAdminStorage(db),
		MapStorage:    mysql.NewMapStorage(db),
		QuotaManager:  quota.Noop(),
		MetricFactory: monitoring.InertMetricFactory{},
		NewKeyProto: func(ctx context.Context, spec *keyspb.Specification) (proto.Message, error) {
			return der.NewProtoFromSpec(spec)
		},
	}

	ret, err := NewMapEnvWithRegistry(registry)
	if err != nil {
		db.Close()
		return nil, err
	}
	ret.DB = db
	return ret, nil
}

// NewMapEnvWithRegistry uses the passed in Registry to create a map server and
// client.
func NewMapEnvWithRegistry(registry extension.Registry) (*MapEnv, error) {
	addr, lis, err := listen()
	if err != nil {
		return nil, err
	}

	ti := interceptor.New(
		registry.AdminStorage, registry.QuotaManager, false /* quotaDryRun */, registry.MetricFactory)
	ci := interceptor.Combine(interceptor.ErrorWrapper, ti.UnaryInterceptor)

	// Create Map Server.
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(ci))
	mapServer := server.NewTrillianMapServer(registry)
	trillian.RegisterTrillianMapServer(grpcServer, mapServer)
	trillian.RegisterTrillianAdminServer(grpcServer, admin.New(registry, nil /* allowedTreeTypes */))
	go grpcServer.Serve(lis)

	// Connect to the server.
	cc, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		grpcServer.Stop()
		return nil, err
	}

	return &MapEnv{
		registry:   registry,
		mapServer:  mapServer,
		grpcServer: grpcServer,
		clientConn: cc,
		Map:        trillian.NewTrillianMapClient(cc),
		Admin:      trillian.NewTrillianAdminClient(cc),
	}, nil
}

// Close shuts down the server.
func (env *MapEnv) Close() {
	if env.clientConn != nil {
		env.clientConn.Close()
	}
	if env.grpcServer != nil {
		env.grpcServer.GracefulStop()
	}
	if env.DB != nil {
		env.DB.Close()
	}
}
