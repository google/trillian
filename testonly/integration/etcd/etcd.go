// Copyright 2017 Google LLC. All Rights Reserved.
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

// Package etcd contains a helper to start an embedded etcd server.
package etcd

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
)

const (

	// MaxEtcdStartAttempts is the max number of start attempts made before it fails.
	MaxEtcdStartAttempts = 3

	defaultTimeout = 5 * time.Second
	tempDirPrefix  = "etcdquota-test-"
)

// StartEtcd returns a started, ready to use embedded etcd, along with a client and a cleanup
// function (that must be defer-called). There's no need to defer-close etcd of client, cleanup
// closes both.
//
// A temp directory and random ports are used to setup etcd.
func StartEtcd() (e *embed.Etcd, c *clientv3.Client, cleanup func(), err error) {
	var dir string
	dir, err = os.MkdirTemp("", tempDirPrefix)
	if err != nil {
		return
	}

	cleanup = func() {
		if c != nil {
			c.Close()
		}
		if e != nil {
			e.Close()
		}
		os.RemoveAll(dir)
	}

	for i := 0; i < MaxEtcdStartAttempts; i++ {
		e, err = tryStartEtcd(dir)
		if err == nil {
			break
		}
		if strings.Contains(err.Error(), "address already in use") {
			continue
		}
		cleanup()
		return
	}
	if e == nil {
		cleanup()
		err = errors.New("failed to start etcd: too many attempts")
		return
	}

	select {
	case <-e.Server.ReadyNotify():
		// OK
	case <-time.After(defaultTimeout):
		cleanup()
		err = errors.New("timed out waiting for etcd to start")
		return
	}

	c, err = clientv3.New(clientv3.Config{
		Endpoints:   []string{e.Config().LCUrls[0].String()},
		DialTimeout: defaultTimeout,
	})
	if err != nil {
		cleanup()
	}
	return
}

func tryStartEtcd(dir string) (*embed.Etcd, error) {
	p1, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, err
	}
	p1.Close()

	p2, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, err
	}
	p2.Close()

	// OK to ignore err, it'll error below if parsing fails
	clientURL, _ := url.Parse("http://" + p1.Addr().String())
	peerURL, _ := url.Parse("http://" + p2.Addr().String())

	cfg := embed.NewConfig()
	cfg.Dir = dir
	cfg.LCUrls = []url.URL{*clientURL} // listen client URLS
	cfg.ACUrls = []url.URL{*clientURL} // advertise client URLS
	cfg.LPUrls = []url.URL{*peerURL}   // listen peer URLS
	cfg.APUrls = []url.URL{*peerURL}   // advertise peer URLS
	cfg.InitialCluster = fmt.Sprintf("default=%v", peerURL)
	cfg.Logger = "zap"

	return embed.StartEtcd(cfg)
}
