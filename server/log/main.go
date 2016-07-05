package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/server"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/mysql"
	"google.golang.org/grpc"
)

var mysqlUriFlag = flag.String("mysql_uri", "test:zaphod@tcp(127.0.0.1:3306)/test",
	"uri to use with mysql storage")
var serverPortFlag = flag.Int("port", 8090, "Port to serve log requests on")
var sleepBetweenLogsFlag = flag.Duration("sleep_between_logs", time.Millisecond*100, "Time to pause after each log sequenced")
var sleepBetweenRunsFlag = flag.Duration("sleep_between_runs", time.Second*10, "Time to pause after each pass through all logs")
var batchSizeFlag = flag.Int("batch_size", 50, "Max number of leaves to process per batch")

// TODO(Martin2112): Single private key doesn't really work for multi tenant and we can't use
// an HSM interface in this way. Deferring these issues for later.
var privateKeyFile = flag.String("private_key_file", "", "File containing a PEM encoded private key")
var privateKeyPassword = flag.String("private_key_password", "", "Password for server private key")

// TODO(Martin2112): Needs a more realistic provider of log storage with some caching
// and ability to swap out for different storage type
func simpleMySqlStorageProvider(treeID int64) (storage.LogStorage, error) {
	return mysql.NewLogStorage(trillian.LogID{[]byte("TODO"), treeID}, *mysqlUriFlag)
}

func checkDatabaseAccessible(dbUri string) error {
	// TODO(Martin2112): Have to pass a tree ID when we just want metadata. API mismatch
	storage, err := mysql.NewLogStorage(trillian.LogID{[]byte("TODO"), int64(0)}, dbUri)

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

func loadPrivateKey(keyFile, keyPassword string) (crypto.KeyManager, error) {
	if len(keyFile) == 0 || len(keyPassword) == 0 {
		return nil, errors.New("private key file and password must be specified")
	}

	pemData, err := ioutil.ReadFile(keyFile)

	if err != nil {
		return nil, fmt.Errorf("failed to read data from key file: %s", keyFile)
	}

	km := crypto.NewPEMKeyManager()
	err = km.LoadPrivateKey(string(pemData[:]), keyPassword)

	if err != nil {
		return nil, err
	}

	return *km, nil
}

func startRpcServer(listener net.Listener, port int, provider server.LogStorageProviderFunc) *grpc.Server {
	grpcServer := grpc.NewServer()
	logServer := server.NewTrillianLogServer(provider)
	trillian.RegisterTrillianLogServer(grpcServer, logServer)

	return grpcServer
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
	if err := checkDatabaseAccessible(*mysqlUriFlag); err != nil {
		glog.Errorf("Could not access storage, check db configuration and flags")
		os.Exit(1)
	}

	// Load up our private key, exit if this fails to work
	keyManager, err := loadPrivateKey(*privateKeyFile, *privateKeyPassword)

	if err != nil {
		glog.Errorf("Failed to load server key: %v", err)
		os.Exit(1)
	}

	// Set up the listener for the server
	glog.Infof("Creating RPC server starting on port: %d", *serverPortFlag)
	// TODO(Martin2112): More flexible listen address configuration
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *serverPortFlag))

	if err != nil {
		glog.Errorf("Failed to listen on the server port: %d, because: %v", *serverPortFlag, err)
		os.Exit(1)
	}

	// Start the sequencing loop, which will run until we terminate the process
	// TODO(Martin2112): Should respect read only mode and the flags in tree control etc
	// TODO(Martin2112): Plug in Key manager and load key, this is in another branch atm
	// this is OK as we haven't added code to create a signer task yet
	sequencerManager := server.NewSequencerManager(keyManager, done, simpleMySqlStorageProvider, *batchSizeFlag, *sleepBetweenLogsFlag, *sleepBetweenRunsFlag)
	go sequencerManager.OperationLoop()

	// Bring up the RPC server and then block until we get a signal to stop
	rpcServer := startRpcServer(lis, *serverPortFlag, simpleMySqlStorageProvider)
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
