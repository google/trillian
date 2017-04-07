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
	"time"

	_ "github.com/go-sql-driver/mysql" // Load MySQL driver

	"github.com/golang/glog"
	"github.com/google/trillian/crypto/keys"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/monitoring/metric"
	"github.com/google/trillian/server"
	"github.com/google/trillian/storage/mysql"
	"github.com/google/trillian/util"
	"golang.org/x/net/context"
)

var (
	mySQLURI                 = flag.String("mysql_uri", "test:zaphod@tcp(127.0.0.1:3306)/test", "Connection URI for MySQL database")
	exportRPCMetrics         = flag.Bool("export_metrics", true, "If true starts HTTP server and exports stats")
	httpPortFlag             = flag.Int("http_port", 8091, "Port to serve HTTP metrics on")
	sequencerIntervalFlag    = flag.Duration("sequencer_interval", time.Second*10, "Time between each sequencing pass through all logs")
	batchSizeFlag            = flag.Int("batch_size", 50, "Max number of leaves to process per batch")
	numSeqFlag               = flag.Int("num_sequencers", 10, "Number of sequencer workers to run in parallel")
	sequencerGuardWindowFlag = flag.Duration("sequencer_guard_window", 0, "If set, the time elapsed before submitted leaves are eligible for sequencing")
	dumpMetricsInterval      = flag.Duration("dump_metrics_interval", 0, "If greater than 0, how often to dump metrics to the logs.")
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
	db, err := mysql.OpenDB(*mySQLURI)
	if err != nil {
		glog.Exitf("Failed to open MySQL database: %v", err)
	}
	defer db.Close()

	registry := extension.Registry{
		AdminStorage:  mysql.NewAdminStorage(db),
		SignerFactory: keys.PEMSignerFactory{},
		LogStorage:    mysql.NewLogStorage(db),
	}

	// Start HTTP server (optional)
	if *exportRPCMetrics {
		glog.Infof("Creating HTTP server starting on port: %d", *httpPortFlag)
		if err := util.StartHTTPServer(*httpPortFlag); err != nil {
			glog.Exitf("Failed to start HTTP server on port %d: %v", *httpPortFlag, err)
		}
	}

	// Start the sequencing loop, which will run until we terminate the process. This controls
	// both sequencing and signing.
	// TODO(Martin2112): Should respect read only mode and the flags in tree control etc
	ctx, cancel := context.WithCancel(context.Background())
	go util.AwaitSignal(cancel)

	sequencerManager := server.NewSequencerManager(registry, *sequencerGuardWindowFlag)
	sequencerTask := server.NewLogOperationManager(registry, *batchSizeFlag, *numSeqFlag, *sequencerIntervalFlag, util.SystemTimeSource{}, sequencerManager)
	sequencerTask.OperationLoop(ctx)

	// Give things a few seconds to tidy up
	glog.Infof("Stopping server, about to exit")
	glog.Flush()
	time.Sleep(time.Second * 5)
}
