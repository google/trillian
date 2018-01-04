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
	"context"
	"flag"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian"
	"github.com/google/trillian/cmd"
	"github.com/google/trillian/crypto/keys/der"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/monitoring/prometheus"
	"github.com/google/trillian/quota/cacheqm"
	"github.com/google/trillian/quota/etcd/quotaapi"
	"github.com/google/trillian/quota/etcd/quotapb"
	"github.com/google/trillian/quota/mysqlqm"
	"github.com/google/trillian/server"
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
	_ "github.com/google/trillian/merkle/objhasher"
	_ "github.com/google/trillian/merkle/rfc6962"
)

var (
	mySQLURI        = flag.String("mysql_uri", "test:zaphod@tcp(127.0.0.1:3306)/test", "Connection URI for MySQL database")
	rpcEndpoint     = flag.String("rpc_endpoint", "localhost:8090", "Endpoint for RPC requests (host:port)")
	httpEndpoint    = flag.String("http_endpoint", "localhost:8091", "Endpoint for HTTP metrics and REST requests on (host:port, empty means disabled)")
	tlsCertFile     = flag.String("tls_cert_file", "", "Path to the TLS server certificate. If unset, the server will use unsecured connections.")
	tlsKeyFile      = flag.String("tls_key_file", "", "Path to the TLS server key. If unset, the server will use unsecured connections.")
	etcdServers     = flag.String("etcd_servers", "", "A comma-separated list of etcd servers; no etcd registration if empty")
	etcdService     = flag.String("etcd_service", "trillian-logserver", "Service name to announce ourselves under")
	etcdHTTPService = flag.String("etcd_http_service", "trillian-logserver-http", "Service name to announce our HTTP endpoint under")

	quotaDryRun       = flag.Bool("quota_dry_run", false, "If true no requests are blocked due to lack of tokens")
	quotaSystem       = flag.String("quota_system", "mysql", "Quota system to use. One of: \"noop\", \"mysql\" or \"etcd\"")
	quotaMinBatchSize = flag.Int("quota_min_batch_size", cacheqm.DefaultMinBatchSize, "Minimum number of tokens to request from the quota system. "+
		"Zero or lower means disabled. Applicable for etcd quotas.")
	quotaMaxCacheEntries = flag.Int("quota_max_cache_entries", cacheqm.DefaultMaxCacheEntries, "Max number of quota specs in the quota cache. "+
		"Zero or lower means disabled. Applicable for etcd quotas.")
	maxUnsequencedRows = flag.Int("max_unsequenced_rows", mysqlqm.DefaultMaxUnsequenced, "Max number of unsequenced rows before rate limiting kicks in. "+
		"Only effective for quota_system=mysql.")

	treeGCEnabled            = flag.Bool("tree_gc", true, "If true, tree garbage collection (hard-deletion) is periodically performed")
	treeDeleteThreshold      = flag.Duration("tree_delete_threshold", server.DefaultTreeDeleteThreshold, "Minimum period a tree has to remain deleted before being hard-deleted")
	treeDeleteMinRunInterval = flag.Duration("tree_delete_min_run_interval", server.DefaultTreeDeleteMinInterval, "Minimum interval between tree garbage collection sweeps. Actual runs happen randomly between [minInterval,2*minInterval).")

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

	client, err := etcd.NewClient(*etcdServers)
	if err != nil {
		glog.Exitf("Failed to connect to etcd at %v: %v", etcdServers, err)
	}

	// Announce our endpoints to etcd if so configured.
	unannounce := server.AnnounceSelf(ctx, client, *etcdService, *rpcEndpoint)
	defer unannounce()
	if *httpEndpoint != "" {
		unannounceHTTP := server.AnnounceSelf(ctx, client, *etcdHTTPService, *httpEndpoint)
		defer unannounceHTTP()
	}

	qm, err := server.NewQuotaManager(&server.QuotaParams{
		QuotaSystem:        *quotaSystem,
		DB:                 db,
		MaxUnsequencedRows: *maxUnsequencedRows,
		Client:             client,
		MinBatchSize:       *quotaMinBatchSize,
		MaxCacheEntries:    *quotaMaxCacheEntries,
	})
	if err != nil {
		glog.Exitf("Error creating quota manager: %v", err)
	}

	mf := prometheus.MetricFactory{}

	registry := extension.Registry{
		AdminStorage:  mysql.NewAdminStorage(db),
		LogStorage:    mysql.NewLogStorage(db, mf),
		QuotaManager:  qm,
		MetricFactory: mf,
		NewKeyProto: func(ctx context.Context, spec *keyspb.Specification) (proto.Message, error) {
			return der.NewProtoFromSpec(spec)
		},
	}

	m := server.Main{
		RPCEndpoint:  *rpcEndpoint,
		HTTPEndpoint: *httpEndpoint,
		TLSCertFile:  *tlsCertFile,
		TLSKeyFile:   *tlsKeyFile,
		StatsPrefix:  "log",
		QuotaDryRun:  *quotaDryRun,
		DB:           db,
		Registry:     registry,
		RegisterHandlerFn: func(ctx netcontext.Context, mux *runtime.ServeMux, endpoint string, opts []grpc.DialOption) error {
			if err := trillian.RegisterTrillianLogHandlerFromEndpoint(ctx, mux, endpoint, opts); err != nil {
				return err
			}
			if *quotaSystem == server.QuotaEtcd {
				return quotapb.RegisterQuotaHandlerFromEndpoint(ctx, mux, endpoint, opts)
			}
			return nil
		},
		RegisterServerFn: func(s *grpc.Server, registry extension.Registry) error {
			ts := util.SystemTimeSource{}
			logServer := server.NewTrillianLogRPCServer(registry, ts)
			if err := logServer.IsHealthy(); err != nil {
				return err
			}
			trillian.RegisterTrillianLogServer(s, logServer)
			if *quotaSystem == server.QuotaEtcd {
				quotapb.RegisterQuotaServer(s, quotaapi.NewServer(client))
			}
			return nil
		},
		AllowedTreeTypes:      []trillian.TreeType{trillian.TreeType_LOG},
		TreeGCEnabled:         *treeGCEnabled,
		TreeDeleteThreshold:   *treeDeleteThreshold,
		TreeDeleteMinInterval: *treeDeleteMinRunInterval,
	}

	if err := m.Run(ctx); err != nil {
		glog.Exitf("Server exited with error: %v", err)
	}
}
