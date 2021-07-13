// Copyright 2018 Google LLC. All Rights Reserved.
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

// Package main contains the implementation and entry point for the deletetree
// command.
//
// Example usage:
// $ ./deletetree --admin_server=host:port --log_id=logid
package main

import (
	"context"
	"flag"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/client/rpcflags"
	"google.golang.org/grpc"
)

var (
	adminServerAddr = flag.String("admin_server", "", "Address of the gRPC Trillian Admin Server (host:port)")
	logID           = flag.Int64("log_id", 0, "Trillian LogID to delete")
	undeleteTree	= flag.Bool("undelete_tree", false, "Undelete the specified Trillian LogID (bool)")
)

func main() {
	flag.Parse()
	defer glog.Flush()

	dialOpts, err := rpcflags.NewClientDialOptionsFromFlags()
	if err != nil {
		glog.Exitf("Failed to determine dial options: %v", err)
	}

	conn, err := grpc.Dial(*adminServerAddr, dialOpts...)
	if err != nil {
		glog.Exitf("Failed to dial %v: %v", *adminServerAddr, err)
	}
	defer conn.Close()

	a := trillian.NewTrillianAdminClient(conn)
	if *undeleteTree == false {
		_, err = a.DeleteTree(context.Background(), &trillian.DeleteTreeRequest{TreeId: *logID})
		if err != nil {
			glog.Exitf("Delete failed: %v", err)
		}
	} else {
		_, err = a.UndeleteTree(context.Background(), &trillian.UndeleteTreeRequest{TreeId: *logID})
		if err != nil {
			glog.Exitf("Undelete failed: %v", err)
		}
	}
}
