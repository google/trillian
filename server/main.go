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
	"context"
	"database/sql"
	"net"
	"net/http"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/server/admin"
	"github.com/google/trillian/util"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
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
}

// Run starts the configured server. Blocks until the server exits.
func (m *Main) Run(ctx context.Context) error {
	glog.CopyStandardLogTo("WARNING")

	defer m.Server.GracefulStop()
	defer m.DB.Close()

	if err := m.RegisterServerFn(m.Server, m.Registry); err != nil {
		return err
	}
	trillian.RegisterTrillianAdminServer(m.Server, admin.New(m.Registry))
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

	if err := m.Server.Serve(lis); err != nil {
		glog.Errorf("RPC server terminated: %v", err)
	}

	glog.Infof("Stopping server, about to exit")
	glog.Flush()

	// Give things a few seconds to tidy up
	time.Sleep(time.Second * 5)

	return nil
}
