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
	"flag"
	"fmt"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql" // Load MySQL driver

	"github.com/golang/glog"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/monitoring/metric"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/server"
	"github.com/google/trillian/storage/sql/coresql"
	"github.com/google/trillian/storage/sql/coresql/db"
	"github.com/google/trillian/util"
	"github.com/google/trillian/util/etcd"
	"golang.org/x/net/context"
)

var (
	httpEndpoint             = flag.String("http_endpoint", "localhost:8091", "Endpoint for HTTP (host:port, empty means disabled)")
	sequencerIntervalFlag    = flag.Duration("sequencer_interval", time.Second*10, "Time between each sequencing pass through all logs")
	batchSizeFlag            = flag.Int("batch_size", 50, "Max number of leaves to process per batch")
	numSeqFlag               = flag.Int("num_sequencers", 10, "Number of sequencer workers to run in parallel")
	sequencerGuardWindowFlag = flag.Duration("sequencer_guard_window", 0, "If set, the time elapsed before submitted leaves are eligible for sequencing")
	dumpMetricsInterval      = flag.Duration("dump_metrics_interval", 0, "If greater than 0, how often to dump metrics to the logs.")
	forceMaster              = flag.Bool("force_master", false, "If true, assume master for all logs")
	etcdServers              = flag.String("etcd_servers", "localhost:2379", "A comma-separated list of etcd servers")
	lockDir                  = flag.String("lock_file_path", "/test/multimaster", "etcd lock file directory path")
	preElectionPause         = flag.Duration("pre_election_pause", 1*time.Second, "Maximum time to wait before starting elections")
	masterCheckInterval      = flag.Duration("master_check_interval", 5*time.Second, "Interval between checking mastership still held")
	masterHoldInterval       = flag.Duration("master_hold_interval", 60*time.Second, "Minimum interval to hold mastership for")
	resignOdds               = flag.Int("resign_odds", 10, "Chance of resigning mastership after each check, the N in 1-in-N")
	dbDriver                 = flag.String("db_driver", "mysql", "Name of database driver to use (must be known to us)")
	dbURI                    = flag.String("db_uri", "test:zaphod@tcp(127.0.0.1:3306)/test", "Connection URI for database")
)

func main() {
	flag.Parse()
	glog.CopyStandardLogTo("WARNING")
	glog.Info("**** Log Signer Starting ****")

	// Enable dumping of metrics to the log at regular interval,
	// if requested.
	if *dumpMetricsInterval > 0 {
		go metric.DumpToLog(context.Background(), *dumpMetricsInterval)
	}

	// First make sure we can access the database, quit if not
	wrap, err := db.OpenDB(*dbDriver, *dbURI)
	if err != nil {
		glog.Exitf("Failed to open database: %v", err)
	}
	defer wrap.DB().Close()

	ctx, cancel := context.WithCancel(context.Background())
	go util.AwaitSignal(cancel)

	hostname, _ := os.Hostname()
	instanceID := fmt.Sprintf("%s.%d", hostname, os.Getpid())
	var electionFactory util.ElectionFactory
	if *forceMaster {
		glog.Warning("**** Acting as master for all logs ****")
		electionFactory = util.NoopElectionFactory{InstanceID: instanceID}
	} else {
		electionFactory = etcd.NewElectionFactory(instanceID, *etcdServers, *lockDir)
	}

	registry := extension.Registry{
		AdminStorage:    coresql.NewAdminStorage(wrap),
		SignerFactory:   keys.PEMSignerFactory{},
		LogStorage:      coresql.NewLogStorage(wrap),
		ElectionFactory: electionFactory,
		QuotaManager:    quota.Noop(),
	}

	// Start HTTP server (optional)
	if *httpEndpoint != "" {
		glog.Infof("Creating HTTP server starting on %v", *httpEndpoint)
		if err := util.StartHTTPServer(*httpEndpoint); err != nil {
			glog.Exitf("Failed to start HTTP server on %v: %v", *httpEndpoint, err)
		}
	}

	// Start the sequencing loop, which will run until we terminate the process. This controls
	// both sequencing and signing.
	// TODO(Martin2112): Should respect read only mode and the flags in tree control etc
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
