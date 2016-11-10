package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/examples/ct"
	"github.com/google/trillian/util"
	"google.golang.org/grpc"
)

// TODO(Martin2112): We still have the treeid / log ID thing to think about + security etc.
var logIDFlag = flag.Int64("log_id", 1, "The log id (tree id) to send to the backend")
var rpcBackendFlag = flag.String("log_rpc_server", "localhost:8090", "Backend Log RPC server to use")
var rpcDeadlineFlag = flag.Duration("rpc_deadline", time.Second*10, "Deadline for backend RPC requests")
var serverPortFlag = flag.Int("port", 6962, "Port to serve CT log requests on")
var trustedRootPEMFlag = flag.String("trusted_roots", "", "File containing one or more concatenated trusted root certs in PEM format")
var privateKeyPasswordFlag = flag.String("private_key_password", "", "Password for log private key")
var privateKeyPEMFlag = flag.String("private_key_file", "", "PEM file containing log private key")
var publicKeyPEMFlag = flag.String("public_key_file", "", "PEM file containing log public key")

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

func loadLogKeys() (crypto.KeyManager, error) {
	if len(*privateKeyPEMFlag) == 0 {
		return nil, errors.New("the --private_key_file flag must be set to reference a valid PEM file")
	}
	if len(*publicKeyPEMFlag) == 0 {
		return nil, errors.New("the --public_key_file flag must be set to reference a valid PEM file")
	}

	logKeyManager := crypto.NewPEMKeyManager()

	privateKeyPEM, err := ioutil.ReadFile(*privateKeyPEMFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to load private key file: %v", err)
	}

	err = logKeyManager.LoadPrivateKey(string(privateKeyPEM), *privateKeyPasswordFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	publicKeyPEM, err := ioutil.ReadFile(*publicKeyPEMFlag)
	if err != nil {
		return nil, fmt.Errorf("failed to load public key file: %v", err)
	}

	err = logKeyManager.LoadPublicKey(string(publicKeyPEM))

	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %v", err)
	}

	return logKeyManager, nil
}

func awaitSignal() {
	// Arrange notification for the standard set of signals used to terminate a server
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Now block main and wait for a signal
	sig := <-sigs
	glog.Warningf("Signal received: %v", sig)
	glog.Flush()

	// Terminate the process
	os.Exit(1)
}

func main() {
	flag.Parse()
	glog.CopyStandardLogTo("WARNING")
	glog.Info("**** CT HTTP Server Starting ****")

	// Load the set of trusted root certs before bringing up any servers
	trustedRoots, err := loadTrustedRoots()
	if err != nil {
		glog.Fatalf("Failed to read trusted roots: %v", err)
	}

	// And load our keys
	logKeyManager, err := loadLogKeys()
	if err != nil {
		glog.Fatalf("Failed to load keys for log: %v", err)
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
	logContext := ct.NewLogContext(*logIDFlag, trustedRoots, client, logKeyManager, *rpcDeadlineFlag, new(util.SystemTimeSource))
	logContext.RegisterHandlers()

	// Bring up the HTTP server and serve until we get a signal not to.
	go awaitSignal()
	server := http.Server{Addr: fmt.Sprintf("localhost:%d", *serverPortFlag), Handler: nil}
	err = server.ListenAndServe()
	glog.Warningf("Server exited: %v", err)
	glog.Flush()
}
