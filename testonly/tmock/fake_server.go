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

package tmock

import (
	"net"

	"github.com/google/trillian"
	"google.golang.org/grpc"
)

// Server implements the TrillianAdminServer CreateTree, and
// the TrillianMapServer InitMap RPCs.
// The remaining RPCs are not implemented.
type Server struct {
	*MockTrillianAdminServer
	*MockTrillianLogServer
	*MockTrillianMapServer
}

// StartServer starts a server on a random port.
// Returns the started server, the listener it's using for connection and a
// close function that must be defer-called on the scope the server is meant to
// stop.
func StartServer(server *Server) (net.Listener, func(), error) {
	grpcServer := grpc.NewServer()
	trillian.RegisterTrillianAdminServer(grpcServer, server)
	trillian.RegisterTrillianLogServer(grpcServer, server)
	trillian.RegisterTrillianMapServer(grpcServer, server)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	go grpcServer.Serve(lis)

	stopFn := func() {
		grpcServer.Stop()
		lis.Close()
	}

	return lis, stopFn, nil
}
