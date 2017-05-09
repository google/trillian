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

package integration

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/examples/ct"
)

// CTLogEnv is a test environment that contains both a log server and a CT personality
// connected to it.
type CTLogEnv struct {
	logEnv       *LogEnv
	ctListener   net.Listener
	ctHTTPServer *http.Server
	CTAddr       string
}

// NewCTLogEnv creates a fresh DB, log server, and CT personality.
// testID should be unique to each unittest package so as to allow parallel tests.
// Created logIDs will be set to cfgs.
func NewCTLogEnv(ctx context.Context, cfgs []*ct.LogConfig, numSequencers int, testID string) (*CTLogEnv, error) {
	// Start log server and signer.
	logEnv, err := NewLogEnv(ctx, numSequencers, testID)
	if err != nil {
		return nil, fmt.Errorf("failed to create LogEnv: %v", err)
	}

	// Provision the logs.
	for _, cfg := range cfgs {
		logID, err := logEnv.CreateLog()
		if err != nil {
			return nil, fmt.Errorf("failed to provision log %d: %v", cfg.LogID, err)
		}
		cfg.LogID = logID
	}

	// Start the CT personality.
	addr, listener, err := listen()
	if err != nil {
		return nil, fmt.Errorf("failed to find an unused port for CT personality: %v", err)
	}
	server := http.Server{Addr: addr, Handler: nil}
	logEnv.pendingTasks.Add(1)
	go func(env *LogEnv, server *http.Server, listener net.Listener, cfgs []*ct.LogConfig) {
		defer env.pendingTasks.Done()
		client := trillian.NewTrillianLogClient(env.ClientConn)
		for _, cfg := range cfgs {
			handlers, err := cfg.SetUpInstance(client, 10*time.Second)
			if err != nil {
				glog.Fatalf("Failed to set up log instance for %+v: %v", cfg, err)
			}
			for path, handler := range *handlers {
				http.Handle(path, handler)
			}
		}
		server.Serve(listener)
	}(logEnv, &server, listener, cfgs)
	return &CTLogEnv{
		logEnv:       logEnv,
		ctListener:   listener,
		ctHTTPServer: &server,
		CTAddr:       addr,
	}, nil
}

// Close shuts down the servers.
func (env *CTLogEnv) Close() {
	env.ctListener.Close()
	env.logEnv.Close()
}
