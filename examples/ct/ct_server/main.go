package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/examples/ct"
	"google.golang.org/grpc"
)

var rpcBackendFlag = flag.String("log_rpc_backend", "localhost:8090", "Backend Log RPC server to use")
var serverPortFlag = flag.Int("port", 8091, "Port to serve CT log requests on")
var trustedRootPEMFlag = flag.String("trusted_roots", "", "File containing one or more concatenated trusted root certs in PEM format")

func loadTrustedRoots() (*ct.PEMCertPool, error) {
	if len(*trustedRootPEMFlag) == 0 {
		return nil, errors.New("the --trusted_roots flag must be set to reference a valid PEM file")
	}

	trustedRoots := ct.NewPEMCertPool()

	// The set of root data should never be particularly large and we have to keep it in memory
	// anyway to validate submissions
	rootData, err := ioutil.ReadFile(*trustedRootPEMFlag)

	if err != nil {
		return nil, err
	}

	if !trustedRoots.AppendCertsFromPEM(rootData) {
		return nil, err
	}

	if len(trustedRoots.Subjects()) == 0 {
		return nil, errors.New("trusted root certificate pool is empty?")
	}

	return trustedRoots, nil
}

func main() {
	flag.Parse()

	// Load the set of trusted root certs before bringing up any servers
	trustedRoots, err := loadTrustedRoots()

	if err != nil {
		glog.Fatalf("Failed to read trusted roots: %v", err)
	}

	// TODO(Martin2112): Support TLS and other stuff for RPC client and http server, this is just to
	// get started. Uses a blocking connection so we don't start serving before we're connected
	// to backend.
	conn, err := grpc.Dial(*rpcBackendFlag, grpc.WithInsecure(), grpc.WithBlock())

	if err != nil {
		glog.Fatalf("Could not connect to rpc server: %v", err)
	}

	defer conn.Close()
	client := trillian.NewTrillianLogClient(conn)

	// Create and register the handlers using the RPC client we just set up
	handlers := ct.NewCTRequestHandlers(trustedRoots, client)
	handlers.RegisterCTHandlers()

	glog.Warningf("Server exited: %v", http.ListenAndServe(fmt.Sprintf("localhost:%d", *serverPortFlag), nil))
}
