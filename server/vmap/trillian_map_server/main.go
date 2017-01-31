package main

import (
	"flag"
	"fmt"
	"net"
	"os"

	"net/http"
	_ "net/http/pprof"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/extension/builtin"
	"github.com/google/trillian/server/vmap"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var serverPortFlag = flag.Int("port", 8090, "Port to serve log RPC requests on")
var exportRPCMetrics = flag.Bool("export_metrics", true, "If true starts HTTP server and exports stats")
var httpPortFlag = flag.Int("http_port", 8091, "Port to serve HTTP metrics on")

// TODO(Martin2112): Single private key doesn't really work for multi tenant and we can't use
// an HSM interface in this way. Deferring these issues for later.
var privateKeyFile = flag.String("private_key_file", "", "File containing a PEM encoded private key")
var privateKeyPassword = flag.String("private_key_password", "", "Password for server private key")

// TODO(codingllama): Consider moving to server creation
func checkDatabaseAccessible(registry extension.Registry) error {
	mapStorage, err := registry.GetMapStorage()
	if err != nil {
		return err
	}

	// TODO(codingllama): We shouldn't use a mapID here
	tx, err := mapStorage.BeginForTree(context.Background(), 0)
	if err != nil {
		return err
	}

	// TODO(codingllama): Add some sort of liveness ping here
	return tx.Commit()
}

func startRPCServer(registry extension.Registry) *grpc.Server {
	grpcServer := grpc.NewServer()
	mapServer := vmap.NewTrillianMapServer(registry)
	trillian.RegisterTrillianMapServer(grpcServer, mapServer)
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

func main() {
	flag.Parse()
	glog.CopyStandardLogTo("WARNING")
	glog.Info("**** Map RPC Server Starting ****")

	registry, err := builtin.NewDefaultExtensionRegistry()
	if err != nil {
		glog.Fatalf("Failed create extension registry: %v", err)
	}

	// First make sure we can access the database, quit if not
	if err := checkDatabaseAccessible(registry); err != nil {
		glog.Errorf("Could not access storage, check db configuration and flags: %v", err)
		os.Exit(1)
	}

	// Load up our private key, exit if this fails to work
	// TODO(Martin2112): This will need to be changed for multi tenant as we'll need at
	// least one key per tenant, possibly more.
	if _, err = crypto.LoadPasswordProtectedPrivateKey(*privateKeyFile, *privateKeyPassword); err != nil {
		glog.Fatalf("Failed to load map server key: %v", err)
	}

	// Start HTTP server (optional)
	if *exportRPCMetrics {
		glog.Infof("Creating HTP server starting on port: %d", *httpPortFlag)
		if err := startHTTPServer(*httpPortFlag); err != nil {
			glog.Fatalf("Failed to start http server on port %d: %v", *httpPortFlag, err)
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

	// Bring up the RPC server and then block until we get a signal to stop
	rpcServer := startRPCServer(registry)
	defer glog.Flush()
	if err = rpcServer.Serve(lis); err != nil {
		glog.Errorf("RPC server terminated on port %d: %v", *serverPortFlag, err)
	}
}
