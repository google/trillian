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
	_ "expvar" // For HTTP server registration
	"flag"
	"fmt"
	"net/http"
	"os"

	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/examples/ct"
	"github.com/google/trillian/util"
	"google.golang.org/grpc"
)

// Global flags that affect all log instances.
var serverPortFlag = flag.Int("port", 6962, "Port to serve CT log requests on")
var rpcBackendFlag = flag.String("log_rpc_server", "localhost:8090", "Backend Log RPC server to use")
var rpcDeadlineFlag = flag.Duration("rpc_deadline", time.Second*10, "Deadline for backend RPC requests")
var logConfigFlag = flag.String("log_config", "", "File holding log config in JSON")

func main() {
	flag.Parse()
	// Get log config from file before we start.
	cfg, err := ct.LogConfigFromFile(*logConfigFlag)
	if err != nil {
		glog.Exitf("Failed to read log config: %v", err)
	}

	glog.CopyStandardLogTo("WARNING")
	glog.Info("**** CT HTTP Server Starting ****")

	// TODO(Martin2112): Support TLS and other stuff for RPC client and http server, this is just to
	// get started. Uses a blocking connection so we don't start serving before we're connected
	// to backend.
	conn, err := grpc.Dial(*rpcBackendFlag, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		glog.Exitf("Could not connect to rpc server: %v", err)
	}
	defer conn.Close()
	client := trillian.NewTrillianLogClient(conn)

	for _, c := range cfg {
		handlers, err := c.SetUpInstance(client, *rpcDeadlineFlag)
		if err != nil {
			glog.Exitf("Failed to set up log instance for %+v: %v", cfg, err)
		}
		for path, handler := range *handlers {
			http.Handle(path, handler)
		}
	}

	// Bring up the HTTP server and serve until we get a signal not to.
	go util.AwaitSignal(func() {
		os.Exit(1)
	})
	server := http.Server{Addr: fmt.Sprintf("localhost:%d", *serverPortFlag), Handler: nil}
	err = server.ListenAndServe()
	glog.Warningf("Server exited: %v", err)
	glog.Flush()
}
