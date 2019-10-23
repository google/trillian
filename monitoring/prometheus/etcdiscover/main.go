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

// The etcdiscover binary monitors etcd to track the set of instances that
// support a gRPC service, and updates a file so that Prometheus can track
// those instances.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/coreos/etcd/clientv3"
	etcdnaming "github.com/coreos/etcd/clientv3/naming"
	"github.com/golang/glog"
	"github.com/google/trillian/util"
	"google.golang.org/grpc/naming"
)

var (
	etcdServers  = flag.String("etcd_servers", "", "Comma-separated list of etcd servers")
	etcdServices = flag.String("etcd_services", "", "Comma-separated list of service names to monitor for endpoints")
	targetFile   = flag.String("target", "", "File to update with service endpoint locations")
)

type serviceInstanceInfo struct {
	servers  []string
	services []string
	target   string

	mu        sync.RWMutex
	watcher   map[string]naming.Watcher // nolint: megacheck
	instances map[string]map[string]bool
}

func newServiceInstanceInfo(etcdServers, etcdServices, target string) *serviceInstanceInfo {
	s := serviceInstanceInfo{
		servers:   strings.Split(etcdServers, ","),
		services:  strings.Split(etcdServices, ","),
		watcher:   make(map[string]naming.Watcher), // nolint: megacheck
		target:    target,
		instances: make(map[string]map[string]bool),
	}
	for _, service := range s.services {
		s.instances[service] = make(map[string]bool)
	}
	return &s
}

// Watch starts a collection of goroutines (one per service) that monitor etcd for
// changes in the endpoints serving the services. Blocks until Close() called.
func (s *serviceInstanceInfo) Watch() {
	var wg sync.WaitGroup
	for _, service := range s.services {
		wg.Add(1)
		go func(service string) {
			defer wg.Done()
			s.watchService(service)
		}(service)
	}
	wg.Wait()
}

// Close terminates monitoring.
func (s *serviceInstanceInfo) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, service := range s.services {
		glog.Infof("close watcher for %s", service)
		if s.watcher[service] != nil {
			s.watcher[service].Close()
		}
	}
}

type prometheusJobInfo struct {
	Targets []string          `json:"targets,omitempty"`
	Labels  map[string]string `json:"labels,omitempty"`
}

// Export produces a JSON format description of the services and their endpoints
// in a format suitable for use as Prometheus targets.
func (s *serviceInstanceInfo) Export() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	jobs := make([]*prometheusJobInfo, len(s.services))
	for i, service := range s.services {
		info := prometheusJobInfo{
			Labels: map[string]string{"job": service},
		}
		for endpoint, present := range s.instances[service] {
			if present {
				info.Targets = append(info.Targets, endpoint)
			}
		}
		jobs[i] = &info
	}
	return json.MarshalIndent(jobs, "", "\t")
}

// Update updates the target file with the current state.
func (s *serviceInstanceInfo) Update() {
	jsonData, err := s.Export()
	if err != nil {
		glog.Errorf("failed to export JSON data: %v", err)
		return
	}
	if s.target == "" {
		fmt.Printf("State:\n%s\n", jsonData)
		return
	}
	glog.V(1).Infof("Writing current state:\n%s", string(jsonData))

	// Write to a temporary file.
	tempFile, err := ioutil.TempFile(filepath.Dir(s.target), "pending-"+path.Base(s.target))
	if err != nil {
		glog.Errorf("failed to create tempfile: %v", err)
		return
	}
	if _, err := tempFile.Write(jsonData); err != nil {
		glog.Errorf("failed to write JSON data to tempfile %q: %v", tempFile.Name(), err)
	}
	if err := tempFile.Close(); err != nil {
		glog.Errorf("failed to close JSON file: %v", err)
	}

	// Rename the temporary file to the target so it is updated more atomically.
	if err := os.Rename(tempFile.Name(), s.target); err != nil {
		glog.Errorf("failed to rename tempfile %q to %q: %v", tempFile.Name(), s.target, err)
	}
}

func (s *serviceInstanceInfo) watchService(service string) {
	cfg := clientv3.Config{Endpoints: s.servers, DialTimeout: 5 * time.Second}
	client, err := clientv3.New(cfg)
	if err != nil {
		glog.Exitf("Failed to connect to etcd at %v: %v", s.servers, err)
	}
	res := &etcdnaming.GRPCResolver{Client: client}
	watcher, err := res.Resolve(service)
	if err != nil {
		glog.Exitf("Failed to watch %s for updates: %v", service, err)
	}

	// Save the watcher so external code can Close() it.
	s.mu.Lock()
	s.watcher[service] = watcher
	s.mu.Unlock()

	for {
		updates, err := watcher.Next()
		if err != nil {
			glog.Errorf("Failed on Next(): %v", err)
			return
		}
		for _, update := range updates {
			switch update.Op {
			case naming.Add:
				glog.V(1).Infof("Add(%s, +%s)", service, update.Addr)
				s.mu.Lock()
				s.instances[service][update.Addr] = true
				s.mu.Unlock()
			case naming.Delete:
				glog.V(1).Infof("Delete(%s, -%s)", service, update.Addr)
				s.mu.Lock()
				s.instances[service][update.Addr] = false
				s.mu.Unlock()
			}
		}
		s.Update()
	}
}

func main() {
	flag.Parse()
	defer glog.Flush()

	if *etcdServers == "" {
		glog.Exitf("No etcd servers configured with --etcd_servers")
	}
	if *etcdServices == "" {
		glog.Exitf("No etcd services configured with --etcd_services")
	}

	state := newServiceInstanceInfo(*etcdServers, *etcdServices, *targetFile)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go util.AwaitSignal(ctx, func() {
		state.Close()
	})
	state.Watch()
}
