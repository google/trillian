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

var serverPortFlag = flag.Int("port", 8090, "Port to serve log RPC requests on")
var exportRPCMetrics = flag.Bool("export_metrics", true, "If true starts HTTP server and exports stats")
var httpPortFlag = flag.Int("http_port", 8091, "Port to serve HTTP metrics on")

var sequencerSleepBetweenRunsFlag = flag.Duration("sequencer_sleep_between_runs", time.Second*10, "Time to pause after each sequencing pass through all logs")
var signerIntervalFlag = flag.Duration("signer_interval", time.Second*120, "Time after which a new STH is created even if no leaves added")
var batchSizeFlag = flag.Int("batch_size", 50, "Max number of leaves to process per batch")
var sequencerGuardWindowFlag = flag.Duration("sequencer_guard_window", 0, "If set, the time elapsed before submitted leaves are eligible for sequencing")

// TODO(Martin2112): Single private key doesn't really work for multi tenant and we can't use
// an HSM interface in this way. Deferring these issues for later.
var privateKeyFile = flag.String("private_key_file", "", "File containing a PEM encoded private key")
var privateKeyPassword = flag.String("private_key_password", "", "Password for server private key")

// TODO(codingllama): Consider moving to server creation
func checkDatabaseAccessible(registry extension.Registry) error {
	logStorage, err := registry.GetLogStorage()
	if err != nil {
		return err
	}

	tx, err := logStorage.Snapshot(context.Background())
	if err != nil {
		return err
	}

	// Pull the log ids, we don't care about the result, we just want to know that it works
	if _, err := tx.GetActiveLogIDs(); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

func startRPCServer(registry extension.Registry) *grpc.Server {
	// Create and publish the RPC stats objects
	statsInterceptor := monitoring.NewRPCStatsInterceptor(util.SystemTimeSource{}, "ct", "example")
	statsInterceptor.Publish()

	// Create the server, using the interceptor to record stats on the requests
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(statsInterceptor.Interceptor()))

	logServer := server.NewTrillianLogRPCServer(registry, new(util.SystemTimeSource))
	trillian.RegisterTrillianLogServer(grpcServer, logServer)

	reflection.Register(grpcServer)

	return grpcServer
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

	if err := checkDatabaseAccessible(registry); err != nil {
		glog.Errorf("Could not access storage, check db configuration and flags: %v", err)
		os.Exit(1)
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
		glog.Errorf("Failed to listen on the server port: %d, because: %v", *serverPortFlag, err)
		os.Exit(1)
	}

	// Start the sequencing loop, which will run until we terminate the process. This controls
	// both sequencing and signing.
	// TODO(Martin2112): Should respect read only mode and the flags in tree control etc
	ctx, cancel := context.WithCancel(context.Background())

	sequencerManager := server.NewSequencerManager(keyManager, registry, *sequencerGuardWindowFlag)
	sequencerTask := server.NewLogOperationManager(ctx, registry, *batchSizeFlag, *sequencerSleepBetweenRunsFlag, *signerIntervalFlag, util.SystemTimeSource{}, sequencerManager)
	go sequencerTask.OperationLoop()

	// Bring up the RPC server and then block until we get a signal to stop
	rpcServer := startRPCServer(registry)
	go awaitSignal(rpcServer)
	err = rpcServer.Serve(lis)
	if err != nil {
		glog.Errorf("RPC server terminated on port %d: %v", *serverPortFlag, err)
		os.Exit(1)
	}

	// Shut down everything we previously started, rpc server is already down
	cancel()

	// Give things a few seconds to tidy up
	glog.Infof("Stopping server, about to exit")
	glog.Flush()
	time.Sleep(time.Second * 5)
}
