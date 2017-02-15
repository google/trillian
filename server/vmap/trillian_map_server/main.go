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
	_ "net/http/pprof"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/crypto"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/extension/builtin"
	"github.com/google/trillian/server/vmap"
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

func startRPCServer(registry extension.Registry) (*grpc.Server, error) {
	mapServer := vmap.NewTrillianMapServer(registry)
	if err := mapServer.IsHealthy(); err != nil {
		return nil, err
	}
	grpcServer := grpc.NewServer()
	trillian.RegisterTrillianMapServer(grpcServer, mapServer)
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

func main() {
	flag.Parse()
	glog.CopyStandardLogTo("WARNING")
	glog.Info("**** Map RPC Server Starting ****")

	registry, err := builtin.NewDefaultExtensionRegistry()
	if err != nil {
		glog.Exitf("Failed to create extension registry: %v", err)
	}

	// Load up our private key, exit if this fails to work
	// TODO(Martin2112): This will need to be changed for multi tenant as we'll need at
	// least one key per tenant, possibly more.
	if _, err = crypto.LoadPasswordProtectedPrivateKey(*privateKeyFile, *privateKeyPassword); err != nil {
		glog.Exitf("Failed to load map server key: %v", err)
	}

	// Start HTTP server (optional)
	if *exportRPCMetrics {
		glog.Infof("Creating HTP server starting on port: %d", *httpPortFlag)
		if err := startHTTPServer(*httpPortFlag); err != nil {
			glog.Exitf("Failed to start http server on port %d: %v", *httpPortFlag, err)
		}
	}

	// Set up the listener for the server
	glog.Infof("Creating RPC server starting on port: %d", *serverPortFlag)
	// TODO(Martin2112): More flexible listen address configuration
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *serverPortFlag))
	if err != nil {
		glog.Exitf("Failed to listen on the server port: %d, because: %v", *serverPortFlag, err)
	}

	// Bring up the RPC server and then block until we get a signal to stop
	rpcServer, err := startRPCServer(registry)
	if err != nil {
		glog.Exitf("Failed to start RPC server: %v", err)
	}
	defer glog.Flush()

	if err = rpcServer.Serve(lis); err != nil {
		glog.Errorf("RPC server terminated on port %d: %v", *serverPortFlag, err)
	}
}
