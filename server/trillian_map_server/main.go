// Copyright 2016 Google Inc. All Rights Reserved.
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

package main

import (
	"context"
	"flag"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/cmd"
	"github.com/google/trillian/crypto/keys/der"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/monitoring/prometheus"
	"github.com/google/trillian/quota/etcd/quotaapi"
	"github.com/google/trillian/quota/etcd/quotapb"
	"github.com/google/trillian/quota/mysqlqm"
	"github.com/google/trillian/server"
	"github.com/google/trillian/server/interceptor"
	"github.com/google/trillian/storage/mysql"
	"github.com/google/trillian/util"
	"github.com/google/trillian/util/etcd"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"google.golang.org/grpc"

	netcontext "golang.org/x/net/context"

	// Register pprof HTTP handlers
	_ "net/http/pprof"
	// Load MySQL driver
	_ "github.com/go-sql-driver/mysql"
	// Register key ProtoHandlers
	_ "github.com/google/trillian/crypto/keys/der/proto"
	_ "github.com/google/trillian/crypto/keys/pem/proto"
	_ "github.com/google/trillian/crypto/keys/pkcs11/proto"
	// Load hashers
	_ "github.com/google/trillian/merkle/coniks"
	_ "github.com/google/trillian/merkle/maphasher"
)

var (
	mySQLURI     = flag.String("mysql_uri", "test:zaphod@tcp(127.0.0.1:3306)/test", "Connection URI for MySQL database")
	rpcEndpoint  = flag.String("rpc_endpoint", "localhost:8090", "Endpoint for RPC requests (host:port)")
	httpEndpoint = flag.String("http_endpoint", "localhost:8091", "Endpoint for HTTP metrics and REST requests on (host:port, empty means disabled)")
	etcdServers  = flag.String("etcd_servers", "", "A comma-separated list of etcd servers; no etcd registration if empty")

	quotaDryRun        = flag.Bool("quota_dry_run", false, "If true no requests are blocked due to lack of tokens")
	quotaSystem        = flag.String("quota_system", "mysql", "Quota system to use. One of: \"noop\", \"mysql\" or \"etcd\"")
	maxUnsequencedRows = flag.Int("max_unsequenced_rows", mysqlqm.DefaultMaxUnsequenced, "Max number of unsequenced rows before rate limiting kicks in. "+
		"Only effective for quota_system=mysql.")

	configFile = flag.String("config", "", "Config file containing flags, file contents can be overridden by command line flags")
)

func main() {
	flag.Parse()

	if *configFile != "" {
		if err := cmd.ParseFlagFile(*configFile); err != nil {
			glog.Exitf("Failed to load flags from config file %q: %s", *configFile, err)
		}
	}

	db, err := mysql.OpenDB(*mySQLURI)
	if err != nil {
		glog.Exitf("Failed to open database: %v", err)
	}
	// No defer: database ownership is delegated to server.Main

	client, err := etcd.NewClient(*etcdServers)
	if err != nil {
		glog.Exitf("Failed to connect to etcd at %v: %v", etcdServers, err)
	}

	qm, err := server.NewQuotaManager(&server.QuotaParams{
		QuotaSystem:        *quotaSystem,
		DB:                 db,
		MaxUnsequencedRows: *maxUnsequencedRows,
		Client:             client,
	})
	if err != nil {
		glog.Exitf("Error creating quota manager: %v", err)
	}

	registry := extension.Registry{
		AdminStorage:  mysql.NewAdminStorage(db),
		MapStorage:    mysql.NewMapStorage(db),
		QuotaManager:  qm,
		MetricFactory: prometheus.MetricFactory{},
		NewKeyProto: func(ctx context.Context, spec *keyspb.Specification) (proto.Message, error) {
			return der.NewProtoFromSpec(spec)
		},
	}

	ts := util.SystemTimeSource{}
	stats := monitoring.NewRPCStatsInterceptor(ts, "map", registry.MetricFactory)
	ti := interceptor.New(
		registry.AdminStorage, registry.QuotaManager, *quotaDryRun, registry.MetricFactory)
	netInterceptor := interceptor.Combine(stats.Interceptor(), interceptor.ErrorWrapper, ti.UnaryInterceptor)
	s := grpc.NewServer(grpc.UnaryInterceptor(netInterceptor))
	// No defer: server ownership is delegated to server.Main

	m := server.Main{
		RPCEndpoint:  *rpcEndpoint,
		HTTPEndpoint: *httpEndpoint,
		DB:           db,
		Registry:     registry,
		Server:       s,
		RegisterHandlerFn: func(ctx netcontext.Context, mux *runtime.ServeMux, endpoint string, opts []grpc.DialOption) error {
			if err := trillian.RegisterTrillianMapHandlerFromEndpoint(ctx, mux, endpoint, opts); err != nil {
				return err
			}
			if *quotaSystem == server.QuotaEtcd {
				return quotapb.RegisterQuotaHandlerFromEndpoint(ctx, mux, endpoint, opts)
			}
			return nil
		},
		RegisterServerFn: func(s *grpc.Server, registry extension.Registry) error {
			mapServer := server.NewTrillianMapServer(registry)
			if err := mapServer.IsHealthy(); err != nil {
				return err
			}
			trillian.RegisterTrillianMapServer(s, mapServer)
			if *quotaSystem == server.QuotaEtcd {
				quotapb.RegisterQuotaServer(s, quotaapi.NewServer(client))
			}
			return nil
		},
	}

	ctx := context.Background()
	if err := m.Run(ctx); err != nil {
		glog.Exitf("Server exited with error: %v", err)
	}
}
