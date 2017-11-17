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

package server

import (
	"database/sql"
	"net"
	"net/http"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/server/admin"
	"github.com/google/trillian/util"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/naming"
	"google.golang.org/grpc/reflection"

	etcdnaming "github.com/coreos/etcd/clientv3/naming"
)

const (
	// DefaultTreeDeleteThreshold is the suggested threshold for tree deletion.
	// It represents the minimum time a tree has to remain Deleted before being hard-deleted.
	DefaultTreeDeleteThreshold = 7 * 24 * time.Hour

	// DefaultTreeDeleteMinInterval is the suggested min interval between tree GC sweeps.
	// A tree GC sweep consists of listing deleted trees older than the deletion threshold and
	// hard-deleting them.
	// Actual runs happen randomly between [minInterval,2*minInterval).
	DefaultTreeDeleteMinInterval = 4 * time.Hour
)

// Main encapsulates the data and logic to start a Trillian server (Log or Map).
type Main struct {
	// Endpoints for RPC and HTTP/REST servers.
	// HTTP/REST is optional, if empty it'll not be bound.
	RPCEndpoint, HTTPEndpoint string
	DB                        *sql.DB
	Registry                  extension.Registry
	Server                    *grpc.Server

	// RegisterHandlerFn is called to register REST-proxy handlers.
	RegisterHandlerFn func(context.Context, *runtime.ServeMux, string, []grpc.DialOption) error
	// RegisterServerFn is called to register RPC servers.
	RegisterServerFn func(*grpc.Server, extension.Registry) error

	// AllowedTreeTypes determines which types of trees may be created through the Admin Server
	// bound by Main. nil means unrestricted.
	AllowedTreeTypes []trillian.TreeType

	TreeGCEnabled         bool
	TreeDeleteThreshold   time.Duration
	TreeDeleteMinInterval time.Duration
}

// Run starts the configured server. Blocks until the server exits.
func (m *Main) Run(ctx context.Context) error {
	glog.CopyStandardLogTo("WARNING")

	defer m.Server.GracefulStop()
	defer m.DB.Close()

	if err := m.RegisterServerFn(m.Server, m.Registry); err != nil {
		return err
	}
	trillian.RegisterTrillianAdminServer(m.Server, admin.New(m.Registry, m.AllowedTreeTypes))
	reflection.Register(m.Server)

	if endpoint := m.HTTPEndpoint; endpoint != "" {
		mux := runtime.NewServeMux()
		opts := []grpc.DialOption{grpc.WithInsecure()}
		if err := m.RegisterHandlerFn(ctx, mux, m.RPCEndpoint, opts); err != nil {
			return err
		}
		if err := trillian.RegisterTrillianAdminHandlerFromEndpoint(ctx, mux, m.RPCEndpoint, opts); err != nil {
			return err
		}
		glog.Infof("HTTP server starting on %v", endpoint)

		go http.ListenAndServe(endpoint, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			switch {
			case req.RequestURI == "/metrics":
				promhttp.Handler().ServeHTTP(w, req)
			default:
				mux.ServeHTTP(w, req)
			}
		}))
	}

	glog.Infof("RPC server starting on %v", m.RPCEndpoint)
	lis, err := net.Listen("tcp", m.RPCEndpoint)
	if err != nil {
		return err
	}
	go util.AwaitSignal(m.Server.Stop)

	if m.TreeGCEnabled {
		go func() {
			glog.Info("Deleted tree GC started")
			gc := admin.NewDeletedTreeGC(
				m.Registry.AdminStorage,
				m.TreeDeleteThreshold,
				m.TreeDeleteMinInterval,
				m.Registry.MetricFactory)
			gc.Run(ctx)
		}()
	}

	if err := m.Server.Serve(lis); err != nil {
		glog.Errorf("RPC server terminated: %v", err)
	}

	glog.Infof("Stopping server, about to exit")
	glog.Flush()

	// Give things a few seconds to tidy up
	time.Sleep(time.Second * 5)

	return nil
}

// AnnounceSelf announces this binary's presence to etcd.  Returns a function that
// should be called on process exit.
// AnnounceSelf does nothing if client is nil.
func AnnounceSelf(ctx context.Context, client *clientv3.Client, etcdService, endpoint string) func() {
	if client == nil {
		return func() {}
	}

	res := etcdnaming.GRPCResolver{Client: client}

	// Get a lease so our entry self-destructs.
	leaseRsp, err := client.Grant(ctx, 30)
	if err != nil {
		glog.Exitf("Failed to get lease from etcd: %v", err)
	}
	client.KeepAlive(ctx, leaseRsp.ID)

	update := naming.Update{Op: naming.Add, Addr: endpoint}
	res.Update(ctx, etcdService, update, clientv3.WithLease(leaseRsp.ID))
	glog.Infof("Announcing our presence in %v with %+v", etcdService, update)

	bye := naming.Update{Op: naming.Delete, Addr: endpoint}
	return func() {
		// Use a background context because the original context may have been cancelled.
		glog.Infof("Removing our presence in %v with %+v", etcdService, bye)
		ctx := context.Background()
		res.Update(ctx, etcdService, bye)
		client.Revoke(ctx, leaseRsp.ID)
	}
}
