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

// The trillian_log_server binary runs the Trillian log server, and also
// provides an admin server.
package main

import (
	"flag"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/cmd"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/monitoring/prometheus"
	"github.com/google/trillian/server"
	"github.com/google/trillian/server/interceptor"
	"github.com/google/trillian/storage/mysql"
	"github.com/google/trillian/util"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	_ "net/http/pprof"

	_ "github.com/go-sql-driver/mysql"              // Load MySQL driver
	_ "github.com/google/trillian/merkle/objhasher" // Load hashers
	_ "github.com/google/trillian/merkle/rfc6962"   // Load hashers

	mysqlq "github.com/google/trillian/quota/mysql"
)

var (
	mySQLURI           = flag.String("mysql_uri", "test:zaphod@tcp(127.0.0.1:3306)/test", "Connection URI for MySQL database")
	rpcEndpoint        = flag.String("rpc_endpoint", "localhost:8090", "Endpoint for RPC requests (host:port)")
	httpEndpoint       = flag.String("http_endpoint", "localhost:8091", "Endpoint for HTTP metrics and REST requests on (host:port, empty means disabled)")
	etcdServers        = flag.String("etcd_servers", "", "A comma-separated list of etcd servers; no etcd registration if empty")
	etcdService        = flag.String("etcd_service", "trillian-logserver", "Service name to announce ourselves under")
	etcdHTTPService    = flag.String("etcd_http_service", "trillian-logserver-http", "Service name to announce our HTTP endpoint under")
	maxUnsequencedRows = flag.Int("max_unsequenced_rows", mysqlq.DefaultMaxUnsequenced, "Max number of unsequenced rows before rate limiting kicks in")
	quotaDryRun        = flag.Bool("quota_dry_run", false, "If true no requests are blocked due to lack of tokens")

	pkcs11ModulePath = flag.String("pkcs11_module_path", "", "Path to the PKCS#11 module to use for keys that use the PKCS#11 interface")

	configFile = flag.String("config", "", "Config file containing flags, file contents can be overridden by command line flags")
)

func main() {
	flag.Parse()

	if *configFile != "" {
		if err := cmd.ParseFlagFile(*configFile); err != nil {
			glog.Exitf("Failed to load flags from config file %q: %s", *configFile, err)
		}
	}

	ctx := context.Background()

	// First make sure we can access the database, quit if not
	db, err := mysql.OpenDB(*mySQLURI)
	if err != nil {
		glog.Exitf("Failed to open database: %v", err)
	}
	// No defer: database ownership is delegated to server.Main

	// Announce our endpoints to etcd if so configured.
	unannounce := server.AnnounceSelf(ctx, *etcdServers, *etcdService, *rpcEndpoint)
	if unannounce != nil {
		defer unannounce()
	}
	if *httpEndpoint != "" {
		unannounceHTTP := server.AnnounceSelf(ctx, *etcdServers, *etcdHTTPService, *httpEndpoint)
		if unannounceHTTP != nil {
			defer unannounceHTTP()
		}
	}

	mf := prometheus.MetricFactory{}
	sf := &keys.DefaultSignerFactory{}
	if *pkcs11ModulePath != "" {
		sf.SetPKCS11Module(*pkcs11ModulePath)
	}

	registry := extension.Registry{
		AdminStorage:  mysql.NewAdminStorage(db),
		LogStorage:    mysql.NewLogStorage(db, mf),
		SignerFactory: sf,
		QuotaManager:  &mysqlq.QuotaManager{DB: db, MaxUnsequencedRows: *maxUnsequencedRows},
		MetricFactory: mf,
	}

	ts := util.SystemTimeSource{}
	stats := monitoring.NewRPCStatsInterceptor(ts, "log", registry.MetricFactory)
	ti := interceptor.New(
		registry.AdminStorage, registry.QuotaManager, *quotaDryRun, registry.MetricFactory)
	netInterceptor := interceptor.Combine(stats.Interceptor(), interceptor.ErrorWrapper, ti.UnaryInterceptor)
	s := grpc.NewServer(grpc.UnaryInterceptor(netInterceptor))
	// No defer: server ownership is delegated to server.Main

	m := server.Main{
		RPCEndpoint:       *rpcEndpoint,
		HTTPEndpoint:      *httpEndpoint,
		DB:                db,
		Registry:          registry,
		Server:            s,
		RegisterHandlerFn: trillian.RegisterTrillianLogHandlerFromEndpoint,
		RegisterServerFn: func(s *grpc.Server, registry extension.Registry) error {
			logServer := server.NewTrillianLogRPCServer(registry, ts)
			if err := logServer.IsHealthy(); err != nil {
				return err
			}
			trillian.RegisterTrillianLogServer(s, logServer)
			return err
		},
	}

	if err := m.Run(ctx); err != nil {
		glog.Exitf("Server exited with error: %v", err)
	}
}
