// Copyright 2016 Google LLC. All Rights Reserved.
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

// The trillian_log_signer binary runs the log signing code.
package main

import (
	"context"
	"flag"
	"fmt"
	_ "net/http/pprof" // Register pprof HTTP handlers.
	"os"
	"runtime/pprof"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian/cmd"
	"github.com/google/trillian/cmd/internal/serverutil"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/log"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/monitoring/opencensus"
	"github.com/google/trillian/monitoring/prometheus"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/quota/etcd"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/util"
	"github.com/google/trillian/util/clock"
	"github.com/google/trillian/util/election"
	"github.com/google/trillian/util/election2"
	etcdelect "github.com/google/trillian/util/election2/etcd"
	etcdutil "github.com/google/trillian/util/etcd"
	"google.golang.org/grpc"

	tpb "github.com/google/trillian"

	// Register key ProtoHandlers
	_ "github.com/google/trillian/crypto/keys/der/proto"
	_ "github.com/google/trillian/crypto/keys/pem/proto"
	_ "github.com/google/trillian/crypto/keys/pkcs11/proto"

	// Register supported storage providers.
	_ "github.com/google/trillian/storage/cloudspanner"
	_ "github.com/google/trillian/storage/mysql"

	// Load hashers
	_ "github.com/google/trillian/merkle/rfc6962"

	// Load MySQL quota provider
	_ "github.com/google/trillian/quota/mysqlqm"
)

var (
	rpcEndpoint              = flag.String("rpc_endpoint", "localhost:8090", "Endpoint for RPC requests (host:port)")
	httpEndpoint             = flag.String("http_endpoint", "localhost:8091", "Endpoint for HTTP (host:port, empty means disabled)")
	tlsCertFile              = flag.String("tls_cert_file", "", "Path to the TLS server certificate. If unset, the server will use unsecured connections.")
	tlsKeyFile               = flag.String("tls_key_file", "", "Path to the TLS server key. If unset, the server will use unsecured connections.")
	sequencerIntervalFlag    = flag.Duration("sequencer_interval", 100*time.Millisecond, "Time between each sequencing pass through all logs")
	batchSizeFlag            = flag.Int("batch_size", 1000, "Max number of leaves to process per batch")
	numSeqFlag               = flag.Int("num_sequencers", 10, "Number of sequencer workers to run in parallel")
	sequencerGuardWindowFlag = flag.Duration("sequencer_guard_window", 0, "If set, the time elapsed before submitted leaves are eligible for sequencing")
	forceMaster              = flag.Bool("force_master", false, "If true, assume master for all logs")
	etcdHTTPService          = flag.String("etcd_http_service", "trillian-logsigner-http", "Service name to announce our HTTP endpoint under")
	lockDir                  = flag.String("lock_file_path", "/test/multimaster", "etcd lock file directory path")
	healthzTimeout           = flag.Duration("healthz_timeout", time.Second*5, "Timeout used during healthz checks")

	quotaSystem         = flag.String("quota_system", "mysql", fmt.Sprintf("Quota system to use. One of: %v", quota.Providers()))
	quotaIncreaseFactor = flag.Float64("quota_increase_factor", log.QuotaIncreaseFactor,
		"Increase factor for tokens replenished by sequencing-based quotas (1 means a 1:1 relationship between sequenced leaves and replenished tokens)."+
			"Only effective for --quota_system=etcd.")

	preElectionPause   = flag.Duration("pre_election_pause", 1*time.Second, "Maximum time to wait before starting elections")
	masterHoldInterval = flag.Duration("master_hold_interval", 60*time.Second, "Minimum interval to hold mastership for")
	masterHoldJitter   = flag.Duration("master_hold_jitter", 120*time.Second, "Maximal random addition to --master_hold_interval")

	configFile = flag.String("config", "", "Config file containing flags, file contents can be overridden by command line flags")

	// Profiling related flags.
	cpuProfile = flag.String("cpuprofile", "", "If set, write CPU profile to this file")
	memProfile = flag.String("memprofile", "", "If set, write memory profile to this file")
)

func main() {
	flag.Parse()
	defer glog.Flush()

	if *configFile != "" {
		if err := cmd.ParseFlagFile(*configFile); err != nil {
			glog.Exitf("Failed to load flags from config file %q: %s", *configFile, err)
		}
	}

	glog.CopyStandardLogTo("WARNING")
	glog.Info("**** Log Signer Starting ****")

	mf := prometheus.MetricFactory{}
	monitoring.SetStartSpan(opencensus.StartSpan)

	sp, err := storage.NewProviderFromFlags(mf)
	if err != nil {
		glog.Exitf("Failed to get storage provider: %v", err)
	}
	defer sp.Close()

	client, err := etcdutil.NewClientFromString(*etcd.Servers)
	if err != nil {
		glog.Exitf("Failed to connect to etcd at %v: %v", etcd.Servers, err)
	}
	if client != nil {
		defer client.Close()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go util.AwaitSignal(ctx, cancel)

	hostname, _ := os.Hostname()
	instanceID := fmt.Sprintf("%s.%d", hostname, os.Getpid())
	var electionFactory election2.Factory
	switch {
	case *forceMaster:
		glog.Warning("**** Acting as master for all logs ****")
		electionFactory = election2.NoopFactory{}
	case client != nil:
		electionFactory = etcdelect.NewFactory(instanceID, client, *lockDir)
	default:
		glog.Exit("Either --force_master or --etcd_servers must be supplied")
	}

	qm, err := quota.NewManager(*quotaSystem)
	if err != nil {
		glog.Exitf("Error creating quota manager: %v", err)
	}

	registry := extension.Registry{
		AdminStorage:    sp.AdminStorage(),
		LogStorage:      sp.LogStorage(),
		ElectionFactory: electionFactory,
		QuotaManager:    qm,
		MetricFactory:   mf,
	}

	// Start HTTP server (optional)
	if *httpEndpoint != "" {
		// Announce our endpoint to etcd if so configured.
		unannounceHTTP := serverutil.AnnounceSelf(ctx, client, *etcdHTTPService, *httpEndpoint)
		defer unannounceHTTP()
	}

	// Start the sequencing loop, which will run until we terminate the process. This controls
	// both sequencing and signing.
	// TODO(Martin2112): Should respect read only mode and the flags in tree control etc
	log.QuotaIncreaseFactor = *quotaIncreaseFactor
	sequencerManager := log.NewSequencerManager(registry, *sequencerGuardWindowFlag)
	info := log.OperationInfo{
		Registry:    registry,
		BatchSize:   *batchSizeFlag,
		NumWorkers:  *numSeqFlag,
		RunInterval: *sequencerIntervalFlag,
		TimeSource:  clock.System,
		ElectionConfig: election.RunnerConfig{
			PreElectionPause:   *preElectionPause,
			MasterHoldInterval: *masterHoldInterval,
			MasterHoldJitter:   *masterHoldJitter,
			TimeSource:         clock.System,
		},
	}
	sequencerTask := log.NewOperationManager(info, sequencerManager)
	go sequencerTask.OperationLoop(ctx)

	// Enable CPU profile if requested
	if *cpuProfile != "" {
		f := mustCreate(*cpuProfile)
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	m := serverutil.Main{
		RPCEndpoint:  *rpcEndpoint,
		HTTPEndpoint: *httpEndpoint,
		TLSCertFile:  *tlsCertFile,
		TLSKeyFile:   *tlsKeyFile,
		StatsPrefix:  "logsigner",
		DBClose:      sp.Close,
		Registry:     registry,
		RegisterServerFn: func(s *grpc.Server, _ extension.Registry) error {
			tpb.RegisterTrillianLogSequencerServer(s, &struct{}{})
			return nil
		},
		IsHealthy:       sp.AdminStorage().CheckDatabaseAccessible,
		HealthyDeadline: *healthzTimeout,
	}

	if err := m.Run(ctx); err != nil {
		glog.Exitf("Server exited with error: %v", err)
	}

	if *memProfile != "" {
		f := mustCreate(*memProfile)
		pprof.WriteHeapProfile(f)
	}

	// Give things a few seconds to tidy up
	glog.Infof("Stopping server, about to exit")
	time.Sleep(time.Second * 5)
}

func mustCreate(fileName string) *os.File {
	f, err := os.Create(fileName)
	if err != nil {
		glog.Fatal(err)
	}
	return f
}
