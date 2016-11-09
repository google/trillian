package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/server"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/mysql"
	"github.com/google/trillian/util"
	"google.golang.org/grpc"
)

var mysqlURIFlag = flag.String("mysql_uri", "test:zaphod@tcp(127.0.0.1:3306)/test",
	"uri to use with mysql storage")
var serverPortFlag = flag.Int("port", 8090, "Port to serve log RPC requests on")
var exportRPCMetrics = flag.Bool("exportMetrics", true, "If true starts HTTP server and exports stats")
var httpPortFlag = flag.Int("http_port", 8091, "Port to serve HTTP metrics on")
var sequencerSleepBetweenRunsFlag = flag.Duration("sequencer_sleep_between_runs", time.Second*10, "Time to pause after each sequencing pass through all logs")
var signerSleepBetweenRunsFlag = flag.Duration("signer_sleep_between_runs", time.Second*120, "Time to pause after each signing pass through all logs")
var batchSizeFlag = flag.Int("batch_size", 50, "Max number of leaves to process per batch")

// TODO(Martin2112): Single private key doesn't really work for multi tenant and we can't use
// an HSM interface in this way. Deferring these issues for later.
var privateKeyFile = flag.String("private_key_file", "", "File containing a PEM encoded private key")
var privateKeyPassword = flag.String("private_key_password", "", "Password for server private key")

// Must hold this lock before accessing the storage map
var storageMapGuard sync.Mutex

// Map from tree ID to storage impl for that log
var storageMap = make(map[int64]storage.LogStorage)

// TODO(Martin2112): Needs to be able to swap out for different storage type
func simpleMySQLStorageProvider(treeID int64) (storage.LogStorage, error) {
	return mysql.NewLogStorage(treeID, *mysqlURIFlag)
}

// TODO(Martin2112): Could pull this out as a wrapper so it can be used elsewhere
func getStorageForLog(logID int64) (storage.LogStorage, error) {
	storageMapGuard.Lock()
	defer storageMapGuard.Unlock()

	s, ok := storageMap[logID]

	if !ok {
		glog.Infof("Creating new storage for log: %d", logID)

		var err error
		s, err = simpleMySQLStorageProvider(logID)

		if err != nil {
			return s, err
		}

		storageMap[logID] = s
	}

	return s, nil
}

func checkDatabaseAccessible(dbURI string) error {
	// TODO(Martin2112): Have to pass a tree ID when we just want metadata. API mismatch
	storage, err := mysql.NewLogStorage(int64(0), dbURI)

	if err != nil {
		// This is probably something fundamentally wrong
		return err
	}

	tx, err := storage.Begin()

	if err != nil {
		// Out of resources maybe?
		return err
	}

	defer tx.Commit()

	// Pull the log ids, we don't care about the result, we just want to know that it works
	_, err = tx.GetActiveLogIDs()

	return err
}

func startRPCServer(listener net.Listener, port int, provider server.LogStorageProviderFunc) *grpc.Server {
	// Create and publish the RPC stats objects
	statsInterceptor := monitoring.NewRPCStatsInterceptor(util.SystemTimeSource{}, "ct", "example")
	statsInterceptor.Publish()

	// Create the server, using the interceptor to record stats on the requests
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(statsInterceptor.Interceptor()))

	logServer := server.NewTrillianLogRPCServer(provider)
	trillian.RegisterTrillianLogServer(grpcServer, logServer)

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
	glog.Infof("Signal received: %v", sig)

	// Bring down the RPC server, which will unblock main
	rpcServer.Stop()
}

func main() {
	flag.Parse()

	done := make(chan struct{})

	glog.Info("**** Log Server Starting ****")

	// First make sure we can access the database, quit if not
	if err := checkDatabaseAccessible(*mysqlURIFlag); err != nil {
		glog.Errorf("Could not access storage, check db configuration and flags")
		os.Exit(1)
	}

	// Load up our private key, exit if this fails to work
	// TODO(Martin2112): This will need to be changed for multi tenant as we'll need at
	// least one key per tenant, possibly more.
	keyManager, err := crypto.LoadPasswordProtectedPrivateKey(*privateKeyFile, *privateKeyPassword)

	if err != nil {
		glog.Fatalf("Failed to load server key: %v", err)
	}

	// Start HTTP server (optional)
	if *exportRPCMetrics {
		err := startHTTPServer(*httpPortFlag)

		if err != nil {
			glog.Fatalf("Failed to start http server on port %d: %v", *httpPortFlag, err)
			os.Exit(1)
		}
	}

	// Set up the listener for the server
	glog.Infof("Creating RPC server starting on port: %d", *serverPortFlag)
	// TODO(Martin2112): More flexible listen address configuration
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *serverPortFlag))

	if err != nil {
		glog.Errorf("Failed to listen on the server port: %d, because: %v", *serverPortFlag, err)
		os.Exit(1)
	}

	// Start the sequencing loop, which will run until we terminate the process. This controls
	// both sequencing and signing.
	// TODO(Martin2112): Should respect read only mode and the flags in tree control etc
	sequencerManager := server.NewLogOperationManager(done, getStorageForLog, *batchSizeFlag, *sequencerSleepBetweenRunsFlag, *signerSleepBetweenRunsFlag, util.SystemTimeSource{}, server.NewSequencerManager(keyManager))
	go sequencerManager.OperationLoop()

	// Bring up the RPC server and then block until we get a signal to stop
	rpcServer := startRPCServer(lis, *serverPortFlag, getStorageForLog)
	go awaitSignal(rpcServer)
	err = rpcServer.Serve(lis)

	if err != nil {
		glog.Errorf("RPC server terminated on port %d: %v", *serverPortFlag, err)
		os.Exit(1)
	}

	// Shut down everything we previously started, rpc server is already down
	close(done)

	// Give things a few seconds to tidy up
	glog.Infof("Stopping server, about to exit")
	time.Sleep(time.Second * 5)
}
