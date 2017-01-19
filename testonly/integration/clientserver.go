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

package integration

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"net"
	"strings"

	"github.com/google/trillian"
	"github.com/google/trillian/extension/builtin"
	"github.com/google/trillian/server"
	"github.com/google/trillian/util"
	"google.golang.org/grpc"
)

const (
	createSQLFile = "../storage/mysql/storage.sql"
	mysqlURI      = "root@tcp(127.0.0.1:3306)/"
)

// Env is a test environment that contains both a server and a connection to it.
type Env struct {
	grpcServer *grpc.Server
	logServer  *server.TrillianLogRPCServer
	ClientConn *grpc.ClientConn
}

// listen opens a random high numbered port for listening.
func listen() (string, net.Listener, error) {
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		return "", nil, err
	}
	_, port, err := net.SplitHostPort(lis.Addr().String())
	if err != nil {
		return "", nil, err
	}
	addr := "localhost:" + port
	return addr, lis, nil
}

// resetDB drops and recreates the test database.
// TODO(gdbelvin): convert to CreateDB when registry can take a db object.
func resetDB(testID string) error {
	var mysqlURIdb = fmt.Sprintf("root@tcp(127.0.0.1:3306)/log_unittest_%v", testID)
	builtin.MySQLURIFlag = &mysqlURIdb

	db, err := sql.Open("mysql", mysqlURI)
	if err != nil {
		return err
	}
	defer db.Close()

	resetSQL := []string{
		fmt.Sprintf("DROP DATABASE IF EXISTS log_unittest_%v;", testID),
		fmt.Sprintf("CREATE DATABASE log_unittest_%v;", testID),
		fmt.Sprintf("GRANT ALL ON log_unittest_%v.* TO 'log_unittest'@'localhost' IDENTIFIED BY 'zaphod';", testID),
	}
	for _, sql := range resetSQL {
		if _, err := db.Exec(sql); err != nil {
			return err
		}
	}

	// Reconnect to use the new test database.
	dbTest, err := sql.Open("mysql", mysqlURIdb)
	if err != nil {
		return err
	}
	defer dbTest.Close()

	createSQL, err := ioutil.ReadFile(createSQLFile)
	if err != nil {
		return err
	}

	sqlSlice := strings.Split(string(createSQL), ";\n")
	// Omit the last element of the slice, since it will be "".
	for _, sql := range sqlSlice[:len(sqlSlice)-1] {
		if _, err := dbTest.Exec(sql); err != nil {
			return err
		}
	}
	return nil
}

// NewEnv creates a fresh DB, log server, and client.
// testID should be unique to each unittest package so as to allow parallel tests.
func NewEnv(testID string) (*Env, error) {
	err := resetDB(testID)
	if err != nil {
		return nil, err
	}

	timesource := &util.SystemTimeSource{}
	registry, err := builtin.NewDefaultExtensionRegistry()
	if err != nil {
		return nil, err
	}

	grpcServer := grpc.NewServer()
	logServer := server.NewTrillianLogRPCServer(registry, timesource)
	trillian.RegisterTrillianLogServer(grpcServer, logServer)

	// Listen and start server.
	addr, lis, err := listen()
	if err != nil {
		return nil, err
	}
	go grpcServer.Serve(lis)

	// Connect to the server.
	cc, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	return &Env{
		grpcServer: grpcServer,
		logServer:  logServer,
		ClientConn: cc,
	}, nil
}

// Close shuts down the server.
func (env *Env) Close() {
	env.grpcServer.Stop()
}
