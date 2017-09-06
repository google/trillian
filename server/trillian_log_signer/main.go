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

// The trillian_log_signer binary runs the log signing code.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian/cmd"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/log"
	"github.com/google/trillian/monitoring/prometheus"
	"github.com/google/trillian/server"
	"github.com/google/trillian/storage/mysql"
	"github.com/google/trillian/util"
	"github.com/google/trillian/util/etcd"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/net/context"

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
	mySQLURI                 = flag.String("mysql_uri", "test:zaphod@tcp(127.0.0.1:3306)/test", "Connection URI for MySQL database")
	httpEndpoint             = flag.String("http_endpoint", "localhost:8091", "Endpoint for HTTP (host:port, empty means disabled)")
	sequencerIntervalFlag    = flag.Duration("sequencer_interval", time.Second*10, "Time between each sequencing pass through all logs")
	batchSizeFlag            = flag.Int("batch_size", 50, "Max number of leaves to process per batch")
	numSeqFlag               = flag.Int("num_sequencers", 10, "Number of sequencer workers to run in parallel")
	sequencerGuardWindowFlag = flag.Duration("sequencer_guard_window", 0, "If set, the time elapsed before submitted leaves are eligible for sequencing")
	forceMaster              = flag.Bool("force_master", false, "If true, assume master for all logs")
	etcdServers              = flag.String("etcd_servers", "", "A comma-separated list of etcd servers")
	etcdHTTPService          = flag.String("etcd_http_service", "trillian-logsigner-http", "Service name to announce our HTTP endpoint under")
	lockDir                  = flag.String("lock_file_path", "/test/multimaster", "etcd lock file directory path")

	quotaSystem         = flag.String("quota_system", "mysql", "Quota system to use. One of: \"noop\", \"mysql\" or \"etcd\"")
	quotaIncreaseFactor = flag.Float64("quota_increase_factor", log.QuotaIncreaseFactor,
		"Increase factor for tokens replenished by sequencing-based quotas (1 means a 1:1 relationship between sequenced leaves and replenished tokens)."+
			"Only effective for --quota_system=etcd.")

	preElectionPause    = flag.Duration("pre_election_pause", 1*time.Second, "Maximum time to wait before starting elections")
	masterCheckInterval = flag.Duration("master_check_interval", 5*time.Second, "Interval between checking mastership still held")
	masterHoldInterval  = flag.Duration("master_hold_interval", 60*time.Second, "Minimum interval to hold mastership for")
	resignOdds          = flag.Int("resign_odds", 10, "Chance of resigning mastership after each check, the N in 1-in-N")

	configFile = flag.String("config", "", "Config file containing flags, file contents can be overridden by command line flags")
)

func main() {
	flag.Parse()

	if *configFile != "" {
		if err := cmd.ParseFlagFile(*configFile); err != nil {
			glog.Exitf("Failed to load flags from config file %q: %s", *configFile, err)
		}
	}

	glog.CopyStandardLogTo("WARNING")
	glog.Info("**** Log Signer Starting ****")

	// First make sure we can access the database, quit if not
	db, err := mysql.OpenDB(*mySQLURI)
	if err != nil {
		glog.Exitf("Failed to open MySQL database: %v", err)
	}
	defer db.Close()

	client, err := etcd.NewClient(*etcdServers)
	if err != nil {
		glog.Exitf("Failed to connect to etcd at %v: %v", etcdServers, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go util.AwaitSignal(cancel)

	hostname, _ := os.Hostname()
	instanceID := fmt.Sprintf("%s.%d", hostname, os.Getpid())
	var electionFactory util.ElectionFactory
	switch {
	case *forceMaster:
		glog.Warning("**** Acting as master for all logs ****")
		electionFactory = util.NoopElectionFactory{InstanceID: instanceID}
	case client != nil:
		electionFactory = etcd.NewElectionFactory(instanceID, client, *lockDir)
	default:
		glog.Exit("Either --force_master or --etcd_servers must be supplied")
	}

	qm, err := server.NewQuotaManager(&server.QuotaParams{
		QuotaSystem: *quotaSystem,
		DB:          db,
		Client:      client,
	})
	if err != nil {
		glog.Exitf("Error creating quota manager: %v", err)
	}

	mf := prometheus.MetricFactory{}

	registry := extension.Registry{
		AdminStorage:    mysql.NewAdminStorage(db),
		LogStorage:      mysql.NewLogStorage(db, mf),
		ElectionFactory: electionFactory,
		QuotaManager:    qm,
		MetricFactory:   mf,
	}

	// Start HTTP server (optional)
	if *httpEndpoint != "" {
		// Announce our endpoint to etcd if so configured.
		unannounceHTTP := server.AnnounceSelf(ctx, client, *etcdHTTPService, *httpEndpoint)
		defer unannounceHTTP()

		glog.Infof("Creating HTTP server starting on %v", *httpEndpoint)
		http.Handle("/metrics", promhttp.Handler())
		if err := util.StartHTTPServer(*httpEndpoint); err != nil {
			glog.Exitf("Failed to start HTTP server on %v: %v", *httpEndpoint, err)
		}
	}

	// Start the sequencing loop, which will run until we terminate the process. This controls
	// both sequencing and signing.
	// TODO(Martin2112): Should respect read only mode and the flags in tree control etc
	log.QuotaIncreaseFactor = *quotaIncreaseFactor
	sequencerManager := server.NewSequencerManager(registry, *sequencerGuardWindowFlag)
	info := server.LogOperationInfo{
		Registry:            registry,
		BatchSize:           *batchSizeFlag,
		NumWorkers:          *numSeqFlag,
		RunInterval:         *sequencerIntervalFlag,
		TimeSource:          util.SystemTimeSource{},
		PreElectionPause:    *preElectionPause,
		MasterCheckInterval: *masterCheckInterval,
		MasterHoldInterval:  *masterHoldInterval,
		ResignOdds:          *resignOdds,
	}
	sequencerTask := server.NewLogOperationManager(info, sequencerManager)
	sequencerTask.OperationLoop(ctx)

	// Give things a few seconds to tidy up
	glog.Infof("Stopping server, about to exit")
	glog.Flush()
	time.Sleep(time.Second * 5)
}
