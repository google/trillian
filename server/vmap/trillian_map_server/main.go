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

	_ "net/http/pprof"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/server/vmap"
	"github.com/google/trillian/storage"
	"github.com/google/trillian/storage/mysql"
	"google.golang.org/grpc"
)

var mysqlURIFlag = flag.String("mysql_uri", "test:zaphod@tcp(127.0.0.1:3306)/test",
	"uri to use with mysql storage")
var serverPortFlag = flag.Int("port", 8091, "Port to serve map requests on, port+1 is used to serve pprof requests too")

// TODO(Martin2112): Single private key doesn't really work for multi tenant and we can't use
// an HSM interface in this way. Deferring these issues for later.
var privateKeyFile = flag.String("private_key_file", "", "File containing a PEM encoded private key")
var privateKeyPassword = flag.String("private_key_password", "", "Password for server private key")

var mapMutex sync.Mutex
var mapStorage = make(map[int64]storage.MapStorage)

// TODO(Martin2112): Needs a more realistic provider of map storage with some caching
// and ability to swap out for different storage type
func simpleMySQLStorageProvider(treeID int64) (storage.MapStorage, error) {
	mapMutex.Lock()
	defer mapMutex.Unlock()

	s := mapStorage[treeID]
	if s == nil {
		var err error
		s, err = mysql.NewMapStorage(trillian.MapID{[]byte("TODO"), treeID}, *mysqlURIFlag)
		if err != nil {
			return nil, err
		}
		mapStorage[treeID] = s
	}
	return s, nil
}

func checkDatabaseAccessible(dbURI string) error {
	// TODO(Martin2112): Have to pass a tree ID when we just want metadata. API mismatch
	storage, err := mysql.NewMapStorage(trillian.MapID{[]byte("TODO"), int64(0)}, dbURI)

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

	// TODO(al): Add some sort of liveness ping here
	return nil
}

func startRPCServer(listener net.Listener, port int, provider vmap.MapStorageProviderFunc) *grpc.Server {
	grpcServer := grpc.NewServer()
	mapServer := vmap.NewTrillianMapServer(provider)
	trillian.RegisterTrillianMapServer(grpcServer, mapServer)

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

	go func() {
		glog.Infof("HTTP server exited: %v", http.ListenAndServe(fmt.Sprintf("localhost:%d", *serverPortFlag+1), nil))
	}()

	done := make(chan struct{})

	glog.Info("**** Map Server Starting ****")

	// First make sure we can access the database, quit if not
	if err := checkDatabaseAccessible(*mysqlURIFlag); err != nil {
		glog.Errorf("Could not access storage, check db configuration and flags")
		os.Exit(1)
	}

	// Load up our private key, exit if this fails to work
	// TODO(Martin2112): This will need to be changed for multi tenant as we'll need at
	// least one key per tenant, possibly more.
	_, err := crypto.LoadPasswordProtectedPrivateKey(*privateKeyFile, *privateKeyPassword)

	if err != nil {
		glog.Fatalf("Failed to load map server key: %v", err)
	}

	// Set up the listener for the server
	glog.Infof("Creating RPC server starting on port: %d", *serverPortFlag)
	// TODO(Martin2112): More flexible listen address configuration
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *serverPortFlag))

	if err != nil {
		glog.Errorf("Failed to listen on the server port: %d, because: %v", *serverPortFlag, err)
		os.Exit(1)
	}

	// Bring up the RPC server and then block until we get a signal to stop
	rpcServer := startRPCServer(lis, *serverPortFlag, simpleMySQLStorageProvider)
	go awaitSignal(rpcServer)
	err = rpcServer.Serve(lis)

	if err != nil {
		glog.Errorf("RPC server terminated on port %d: %v", *serverPortFlag, err)
		os.Exit(1)
	}

	// Shut down everything we previously started, rpc server is already down
	close(done)

	// Give things a few seconds to tidy up
	glog.Infof("Stopping map server, about to exit")
	time.Sleep(time.Second * 5)
}
