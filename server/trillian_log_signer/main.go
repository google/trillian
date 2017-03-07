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
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian/extension/builtin"
	"github.com/google/trillian/server"
	"github.com/google/trillian/util"
	"golang.org/x/net/context"
)

var (
	exportRPCMetrics              = flag.Bool("export_metrics", true, "If true starts HTTP server and exports stats")
	httpPortFlag                  = flag.Int("http_port", 8091, "Port to serve HTTP metrics on")
	sequencerSleepBetweenRunsFlag = flag.Duration("sequencer_sleep_between_runs", time.Second*10, "Time to pause after each sequencing pass through all logs")
	batchSizeFlag                 = flag.Int("batch_size", 50, "Max number of leaves to process per batch")
	numSeqFlag                    = flag.Int("num_sequencers", 10, "Number of sequencers to run in parallel")
	sequencerGuardWindowFlag      = flag.Duration("sequencer_guard_window", 0, "If set, the time elapsed before submitted leaves are eligible for sequencing")
)

func startHTTPServer(port int) error {
	sock, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		return err
	}
	go func() {
		glog.Info("HTTP server starting")
		http.Serve(sock, nil)
	}()

	return nil
}

func awaitSignal(cancel context.CancelFunc) {
	// Arrange notification for the standard set of signals used for termination.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Now block main and wait for a signal
	sig := <-sigs
	glog.Warningf("Signal received: %v", sig)
	glog.Flush()

	// Cancel the main context.
	cancel()
}

func main() {
	flag.Parse()
	glog.CopyStandardLogTo("WARNING")
	glog.Info("**** Log Signer Starting ****")

	// First make sure we can access the database and keys, quit if not
	registry, err := builtin.NewDefaultExtensionRegistry()
	if err != nil {
		glog.Exitf("Failed to create extension registry: %v", err)
	}

	// Start HTTP server (optional)
	if *exportRPCMetrics {
		glog.Infof("Creating HTP server starting on port: %d", *httpPortFlag)
		if err := startHTTPServer(*httpPortFlag); err != nil {
			glog.Exitf("Failed to start http server on port %d: %v", *httpPortFlag, err)
		}
	}

	// Start the sequencing loop, which will run until we terminate the process. This controls
	// both sequencing and signing.
	// TODO(Martin2112): Should respect read only mode and the flags in tree control etc
	ctx, cancel := context.WithCancel(context.Background())
	go awaitSignal(cancel)

	sequencerManager := server.NewSequencerManager(registry, *sequencerGuardWindowFlag)
	sequencerTask := server.NewLogOperationManager(ctx, registry, *batchSizeFlag, *numSeqFlag, *sequencerSleepBetweenRunsFlag, util.SystemTimeSource{}, sequencerManager)
	sequencerTask.OperationLoop()

	// Give things a few seconds to tidy up
	glog.Infof("Stopping server, about to exit")
	glog.Flush()
	time.Sleep(time.Second * 5)
}
