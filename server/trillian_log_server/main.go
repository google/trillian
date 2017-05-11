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
	_ "net/http/pprof"
	"strings"
	"time"
	"github.com/coreos/etcd/clientv3"
	etcdnaming "github.com/coreos/etcd/clientv3/naming"
	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/server"
	"github.com/google/trillian/server/interceptor"
	"github.com/google/trillian/storage/sql/coresql"
	"github.com/google/trillian/storage/sql/coresql/db"
	"github.com/google/trillian/util"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/naming"
)

var (
	rpcEndpoint         = flag.String("rpc_endpoint", "localhost:8090", "Endpoint for RPC requests (host:port)")
	httpEndpoint        = flag.String("http_endpoint", "localhost:8091", "Endpoint for HTTP metrics and REST requests on (host:port, empty means disabled)")
	dbDriver            = flag.String("db_driver", "mysql", "Name of database driver to use (must be known to us)")
	dbURI               = flag.String("db_uri", "test:zaphod@tcp(127.0.0.1:3306)/test", "Connection URI for database")
	dumpMetricsInterval = flag.Duration("dump_metrics_interval", 0, "If greater than 0, how often to dump metrics to the logs.")
	etcdServers         = flag.String("etcd_servers", "", "A comma-separated list of etcd servers; no etcd registration if empty")
	etcdService         = flag.String("etcd_service", "trillian-log", "Service name to announce ourselves under")
)

func main() {
	flag.Parse()
	ctx := context.Background()

	// First make sure we can access the database, quit if not
	wrap, err := db.OpenDB(*dbDriver, *dbURI)
	if err != nil {
		glog.Exitf("Failed to open database: %v", err)
	}
	// No defer: database ownership is delegated to server.Main

	if len(*etcdServers) > 0 {
		// Announce ourselves to etcd if so configured.
		cfg := clientv3.Config{Endpoints: strings.Split(*etcdServers, ","), DialTimeout: 5 * time.Second}
		client, err := clientv3.New(cfg)
		if err != nil {
			glog.Exitf("Failed to connect to etcd at %v: %v", *etcdServers, err)
		}
		res := etcdnaming.GRPCResolver{Client: client}

		update := naming.Update{Op: naming.Add, Addr: *rpcEndpoint}
		res.Update(ctx, *etcdService, update)
		glog.Infof("Announcing our presence to %v with %+v", *etcdService, update)
		bye := naming.Update{Op: naming.Delete, Addr: *rpcEndpoint}
		defer res.Update(ctx, *etcdService, bye)
	}

	registry := extension.Registry{
		AdminStorage:  coresql.NewAdminStorage(wrap),
		SignerFactory: keys.PEMSignerFactory{},
		LogStorage:    coresql.NewLogStorage(wrap),
		QuotaManager:  quota.Noop(),
	}

	ts := util.SystemTimeSource{}
	stats := monitoring.NewRPCStatsInterceptor(ts, "ct", "example")
	stats.Publish()
	ti := &interceptor.TrillianInterceptor{
		Admin:        registry.AdminStorage,
		QuotaManager: registry.QuotaManager,
	}
	s := grpc.NewServer(
		grpc.UnaryInterceptor(interceptor.WrapErrors(interceptor.Combine(stats.Interceptor(), ti.UnaryInterceptor))))
	// No defer: server ownership is delegated to server.Main

	m := server.Main{
		RPCEndpoint:  *rpcEndpoint,
		HTTPEndpoint: *httpEndpoint,
		DB:           wrap.DB(),
		Registry:     registry,
		Server:       s,
		RegisterHandlerFn: func(context.Context, *runtime.ServeMux, string, []grpc.DialOption) error {
			return nil
		},
		RegisterServerFn: func(s *grpc.Server, registry extension.Registry) error {
			logServer := server.NewTrillianLogRPCServer(registry, ts)
			if err := logServer.IsHealthy(); err != nil {
				return err
			}
			trillian.RegisterTrillianLogServer(s, logServer)
			return err
		},
		DumpMetricsInterval: *dumpMetricsInterval,
	}

	if err := m.Run(ctx); err != nil {
		glog.Exitf("Server exited with error: %v", err)
	}
}
