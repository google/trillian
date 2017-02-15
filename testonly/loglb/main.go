// Copyright 2017 Google Inc. All Rights Reserved.
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
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/util"

	"github.com/golang/protobuf/ptypes/empty"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

var backendsFlag = flag.String("backends", "", "Comma-separated list of backends")
var serverPortFlag = flag.Int("port", 8090, "Port to serve log RPC requests on")
var exportRPCMetrics = flag.Bool("export_metrics", true, "If true starts HTTP server and exports stats")
var httpPortFlag = flag.Int("http_port", 8091, "Port to serve HTTP metrics on")

type backendConn struct {
	server string
	conn   *grpc.ClientConn
	client trillian.TrillianLogClient
}

type randomLoadBalancer []backendConn

func newRandomLoadBalancer(serverCfg string) (*randomLoadBalancer, error) {
	servers := strings.Split(serverCfg, ",")
	if len(servers) == 0 || (len(servers) == 1 && servers[0] == "") {
		return nil, fmt.Errorf("no backends specified")
	}
	lb := randomLoadBalancer(make([]backendConn, len(servers)))
	for i, s := range servers {
		lb[i].server = s
		var err error
		if lb[i].conn, err = grpc.Dial(s, grpc.WithInsecure(), grpc.WithBlock()); err != nil {
			return nil, fmt.Errorf("could not connect to rpc server %s: %v", s, err)
		}
		lb[i].client = trillian.NewTrillianLogClient(lb[i].conn)
	}
	return &lb, nil
}

func (lb randomLoadBalancer) close() {
	for _, bc := range lb {
		bc.conn.Close()
	}
}

func (lb randomLoadBalancer) pick() *backendConn {
	if len(lb) == 0 {
		return nil
	}
	return &lb[rand.Intn(len(lb))]
}

// Implement each of the methods in the TrillianLogServer interface.

func (lb *randomLoadBalancer) QueueLeaf(ctx context.Context, req *trillian.QueueLeafRequest) (*empty.Empty, error) {
	bc := lb.pick()
	glog.V(3).Infof("forward QueueLeaf request to backend %s", bc.server)
	return bc.client.QueueLeaf(ctx, req)
}

func (lb *randomLoadBalancer) QueueLeaves(ctx context.Context, req *trillian.QueueLeavesRequest) (*trillian.QueueLeavesResponse, error) {
	bc := lb.pick()
	glog.V(3).Infof("forward QueueLeaves request to backend %s", bc.server)
	return bc.client.QueueLeaves(ctx, req)
}

func (lb *randomLoadBalancer) GetInclusionProof(ctx context.Context, req *trillian.GetInclusionProofRequest) (*trillian.GetInclusionProofResponse, error) {
	bc := lb.pick()
	glog.V(3).Infof("forward GetInclusionProof request to backend %s", bc.server)
	return bc.client.GetInclusionProof(ctx, req)
}

func (lb *randomLoadBalancer) GetInclusionProofByHash(ctx context.Context, req *trillian.GetInclusionProofByHashRequest) (*trillian.GetInclusionProofByHashResponse, error) {
	bc := lb.pick()
	glog.V(3).Infof("forward GetInclusionProofByHash request to backend %s", bc.server)
	return bc.client.GetInclusionProofByHash(ctx, req)
}

func (lb *randomLoadBalancer) GetConsistencyProof(ctx context.Context, req *trillian.GetConsistencyProofRequest) (*trillian.GetConsistencyProofResponse, error) {
	bc := lb.pick()
	glog.V(3).Infof("forward GetConsistencyProof request to backend %s", bc.server)
	return bc.client.GetConsistencyProof(ctx, req)
}

func (lb *randomLoadBalancer) GetLatestSignedLogRoot(ctx context.Context, req *trillian.GetLatestSignedLogRootRequest) (*trillian.GetLatestSignedLogRootResponse, error) {
	bc := lb.pick()
	glog.V(3).Infof("forward GetLatestSignedLogRoot request to backend %s", bc.server)
	return bc.client.GetLatestSignedLogRoot(ctx, req)
}

func (lb *randomLoadBalancer) GetSequencedLeafCount(ctx context.Context, req *trillian.GetSequencedLeafCountRequest) (*trillian.GetSequencedLeafCountResponse, error) {
	bc := lb.pick()
	glog.V(3).Infof("forward GetSequencedLeafCount request to backend %s", bc.server)
	return bc.client.GetSequencedLeafCount(ctx, req)
}

func (lb *randomLoadBalancer) GetLeavesByIndex(ctx context.Context, req *trillian.GetLeavesByIndexRequest) (*trillian.GetLeavesByIndexResponse, error) {
	bc := lb.pick()
	glog.V(3).Infof("forward GetLeavesByIndex request to backend %s", bc.server)
	return bc.client.GetLeavesByIndex(ctx, req)
}

func (lb *randomLoadBalancer) GetLeavesByHash(ctx context.Context, req *trillian.GetLeavesByHashRequest) (*trillian.GetLeavesByHashResponse, error) {
	bc := lb.pick()
	glog.V(3).Infof("forward GetLeavesByHash request to backend %s", bc.server)
	return bc.client.GetLeavesByHash(ctx, req)
}

func (lb *randomLoadBalancer) GetEntryAndProof(ctx context.Context, req *trillian.GetEntryAndProofRequest) (*trillian.GetEntryAndProofResponse, error) {
	bc := lb.pick()
	glog.V(3).Infof("forward GetEntryAndProof request to backend %s", bc.server)
	return bc.client.GetEntryAndProof(ctx, req)
}

func (lb *randomLoadBalancer) startRPCServer(listener net.Listener, port int) *grpc.Server {
	// Create and publish the RPC stats objects
	statsInterceptor := monitoring.NewRPCStatsInterceptor(util.SystemTimeSource{}, "ct", "example")
	statsInterceptor.Publish()

	// Create the server, using the interceptor to record stats on the requests
	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(statsInterceptor.Interceptor()))
	trillian.RegisterTrillianLogServer(grpcServer, lb)

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
	glog.Warningf("Signal received: %v", sig)
	glog.Flush()

	// Bring down the RPC server, which will unblock main
	rpcServer.Stop()
}

func main() {
	flag.Parse()
	glog.CopyStandardLogTo("WARNING")
	glog.Info("**** Log RPC Load Balancer Starting ****")

	// Start HTTP server (optional)
	if *exportRPCMetrics {
		err := startHTTPServer(*httpPortFlag)
		if err != nil {
			glog.Exitf("Failed to start http server on port %d: %v", *httpPortFlag, err)
		}
	}

	// Set up the listener for the server
	glog.Infof("Creating listener for port: %d", *serverPortFlag)
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *serverPortFlag))
	if err != nil {
		glog.Errorf("Failed to listen on the server port: %d, because: %v", *serverPortFlag, err)
		os.Exit(1)
	}

	// Bring up the RPC server and then block until we get a signal to stop
	glog.Infof("Creating load balancer across %q", *backendsFlag)
	lb, err := newRandomLoadBalancer(*backendsFlag)
	if err != nil {
		glog.Errorf("Failed to create random load balancer for %q, because: %v", *backendsFlag, err)
		os.Exit(1)
	}
	defer lb.close()
	glog.Infof("Creating RPC server for port: %d", *serverPortFlag)
	rpcServer := lb.startRPCServer(lis, *serverPortFlag)
	go awaitSignal(rpcServer)

	err = rpcServer.Serve(lis)
	if err != nil {
		glog.Warningf("RPC server terminated on port %d: %v", *serverPortFlag, err)
		os.Exit(1)
	}

	// Give things a few seconds to tidy up
	glog.Infof("Stopping server, about to exit")
	glog.Flush()
	time.Sleep(time.Second * 5)
}
