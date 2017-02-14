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
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/extension/builtin"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/server"
	"github.com/google/trillian/util"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var (
	serverPortFlag                = flag.Int("port", 8090, "Port to serve log RPC requests on")
	exportRPCMetrics              = flag.Bool("export_metrics", true, "If true starts HTTP server and exports stats")
	httpPortFlag                  = flag.Int("http_port", 8091, "Port to serve HTTP metrics on")
	sequencerSleepBetweenRunsFlag = flag.Duration("sequencer_sleep_between_runs", time.Second*10, "Time to pause after each sequencing pass through all logs")
	batchSizeFlag                 = flag.Int("batch_size", 50, "Max number of leaves to process per batch")
	numSeqFlag                    = flag.Int("num_sequencers", 10, "Number of sequencers to run in parallel")
	sequencerGuardWindowFlag      = flag.Duration("sequencer_guard_window", 0, "If set, the time elapsed before submitted leaves are eligible for sequencing")

	// TODO(Martin2112): Single private key doesn't really work for multi tenant and we can't use
	// an HSM interface in this way. Deferring these issues for later.
	privateKeyFile     = flag.String("private_key_file", "", "File containing a PEM encoded private key")
	privateKeyPassword = flag.String("private_key_password", "", "Password for server private key")
)

func startRPCServer(registry extension.Registry) (*grpc.Server, error) {
	logServer := server.NewTrillianLogRPCServer(registry, new(util.SystemTimeSource))
	if err := logServer.IsHealthy(); err != nil {
		return nil, err
	}

	// Create and publish the RPC stats objects
	statsInterceptor := monitoring.NewRPCStatsInterceptor(util.SystemTimeSource{}, "ct", "example")
	statsInterceptor.Publish()

	// Create the server, using the interceptor to record stats on the requests
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(statsInterceptor.Interceptor()))
	trillian.RegisterTrillianLogServer(grpcServer, logServer)
	reflection.Register(grpcServer)

	return grpcServer, nil
}

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

func awaitSignal(rpcServer *grpc.Server) {
	// Arrange notification for the standard set of signals used to terminate a server
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Now block main and wait for a signal
	sig := <-sigs
	glog.Warningf("Signal received: %v", sig)
	glog.Flush()

	// Bring down the RPC server, which will unblock main
	rpcServer.Stop()
}

func main() {
	flag.Parse()
	glog.CopyStandardLogTo("WARNING")
	glog.Info("**** Log RPC Server Starting ****")

	// First make sure we can access the database, quit if not
	registry, err := builtin.NewDefaultExtensionRegistry()
	if err != nil {
		glog.Fatalf("Failed create extension registry: %v", err)
	}

	// Load up our private key, exit if this fails to work
	// TODO(Martin2112): This will need to be changed for multi tenant as we'll need at
	// least one key per tenant, possibly more.
	keyManager, err := crypto.LoadPasswordProtectedPrivateKey(*privateKeyFile, *privateKeyPassword)
	if err != nil {
		glog.Fatalf("Failed to load log server key: %v", err)
	}

	// Start HTTP server (optional)
	if *exportRPCMetrics {
		glog.Infof("Creating HTP server starting on port: %d", *httpPortFlag)
		if err := startHTTPServer(*httpPortFlag); err != nil {
			glog.Fatalf("Failed to start http server on port %d: %v", *httpPortFlag, err)
		}
	}

	// Set up the listener for the server
	// TODO(Martin2112): More flexible listen address configuration
	glog.Infof("Creating RPC server starting on port: %d", *serverPortFlag)
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *serverPortFlag))
	if err != nil {
		glog.Exitf("Failed to listen on the server port: %d, because: %v", *serverPortFlag, err)
	}

	// Start the sequencing loop, which will run until we terminate the process. This controls
	// both sequencing and signing.
	// TODO(Martin2112): Should respect read only mode and the flags in tree control etc
	ctx, cancel := context.WithCancel(context.Background())

	sequencerManager := server.NewSequencerManager(keyManager, registry, *sequencerGuardWindowFlag)
	sequencerTask := server.NewLogOperationManager(ctx, registry, *batchSizeFlag, *numSeqFlag, *sequencerSleepBetweenRunsFlag, util.SystemTimeSource{}, sequencerManager)
	go sequencerTask.OperationLoop()

	// Bring up the RPC server and then block until we get a signal to stop
	rpcServer, err := startRPCServer(registry)
	if err != nil {
		glog.Exitf("Failed to start RPC server: %v", err)
	}
	go awaitSignal(rpcServer)

	if err := rpcServer.Serve(lis); err != nil {
		glog.Fatalf("RPC server terminated on port %d: %v", *serverPortFlag, err)
	}

	// Shut down everything we previously started, rpc server is already down
	cancel()

	// Give things a few seconds to tidy up
	glog.Infof("Stopping server, about to exit")
	glog.Flush()
	time.Sleep(time.Second * 5)
}
