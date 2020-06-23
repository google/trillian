// Copyright 2016 Google LLC. All Rights Reserved.
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

package util

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/golang/glog"
)

// StartHTTPServer starts an HTTP server on the given address.
func StartHTTPServer(addr, certFile, keyFile string) error {
	sock, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	go func() {
		glog.Info("HTTP server starting")
		// Let http.ServeTLS handle the error case when only one of the flags is set.
		if certFile != "" || keyFile != "" {
			err = http.ServeTLS(sock, nil, certFile, keyFile)
		} else {
			err = http.Serve(sock, nil)
		}
		if err != nil {
			glog.Errorf("HTTP server stopped: %v", err)
		}
	}()

	return nil
}

// AwaitSignal waits for standard termination signals, then runs the given
// function. Can early return if the passed in context is canceled, in which
// case the function is not run.
func AwaitSignal(ctx context.Context, doneFn func()) {
	// Subscribe for the standard set of signals used to terminate a server.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigs)

	// Wait for a signal or context cancellation.
	select {
	case sig := <-sigs:
		glog.Warningf("Signal received: %v", sig)
		doneFn()
	case <-ctx.Done():
		glog.Infof("AwaitSignal canceled: %v", ctx.Err())
	}
}
