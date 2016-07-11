package main

import (
	"fmt"
	"flag"
	"net/http"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/ct_fe"
	"google.golang.org/grpc"
)

var rpcBackendFlag = flag.String("log_rpc_backend", "localhost:8090", "Backend Log RPC server to use")
var serverPortFlag = flag.Int("port", 8091, "Port to serve CT log requests on")

func main() {
	flag.Parse()

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
	handlers := ct_fe.NewCTRequestHandlers(client)
	handlers.RegisterCTHandlers()

	glog.Warningf("Server exited: %v", http.ListenAndServe(fmt.Sprintf("localhost:%d", *serverPortFlag), nil))
}
