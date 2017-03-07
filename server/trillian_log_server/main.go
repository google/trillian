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
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/extension/builtin"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/server"
	"github.com/google/trillian/server/admin"
	"github.com/google/trillian/util"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var (
	serverPortFlag   = flag.Int("port", 8090, "Port to serve log RPC requests on")
	exportRPCMetrics = flag.Bool("export_metrics", true, "If true starts HTTP server and exports stats")
	httpPortFlag     = flag.Int("http_port", 8091, "Port to serve HTTP metrics on")
)

func startRPCServer(registry extension.Registry) (*grpc.Server, error) {
	// Create and publish the RPC stats objects
	statsInterceptor := monitoring.NewRPCStatsInterceptor(util.SystemTimeSource{}, "ct", "example")
	statsInterceptor.Publish()

	// Create the server, using the interceptor to record stats on the requests
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(statsInterceptor.Interceptor()))

	logServer := server.NewTrillianLogRPCServer(registry, new(util.SystemTimeSource))
	if err := logServer.IsHealthy(); err != nil {
		return nil, err
	}
	trillian.RegisterTrillianLogServer(grpcServer, logServer)

	adminServer := admin.New()
	trillian.RegisterTrillianAdminServer(grpcServer, adminServer)

	reflection.Register(grpcServer)
	return grpcServer, nil
}

func main() {
	flag.Parse()
	glog.CopyStandardLogTo("WARNING")
	glog.Info("**** Log RPC Server Starting ****")

	// First make sure we can access the database and keys, quit if not
	registry, err := builtin.NewDefaultExtensionRegistry()
	if err != nil {
		glog.Exitf("Failed to create extension registry: %v", err)
	}

	// Start HTTP server (optional)
	if *exportRPCMetrics {
		glog.Infof("Creating HTP server starting on port: %d", *httpPortFlag)
		if err := util.StartHTTPServer(*httpPortFlag); err != nil {
			glog.Exitf("Failed to start http server on port %d: %v", *httpPortFlag, err)
		}
	}

	// Set up the listener for the server
	// TODO(Martin2112): More flexible listen address configuration
	glog.Infof("Creating RPC server starting on port: %d", *serverPortFlag)
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *serverPortFlag))
	if err != nil {
		glog.Exitf("Failed to listen on the server port: %d, because: %v", *serverPortFlag, err)
	}

	// Bring up the RPC server and then block until we get a signal to stop
	rpcServer, err := startRPCServer(registry)
	if err != nil {
		glog.Exitf("Failed to start RPC server: %v", err)
	}
	go util.AwaitSignal(func() {
		// Bring down the RPC server, which will unblock main
		rpcServer.Stop()
	})

	if err := rpcServer.Serve(lis); err != nil {
		glog.Errorf("RPC server terminated on port %d: %v", *serverPortFlag, err)
	}

	// Give things a few seconds to tidy up
	glog.Infof("Stopping server, about to exit")
	glog.Flush()
	time.Sleep(time.Second * 5)
}
