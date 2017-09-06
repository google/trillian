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

// Package etcd holds an etcd-specific implementation of the
// util.MasterElection interface.
package etcd

import (
	"context"
	"fmt"
	"strings"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/clientv3/concurrency"
	"github.com/golang/glog"
	"github.com/google/trillian/util"
)

// MasterElection is an implementation of util.MasterElection based on etcd.
type MasterElection struct {
	instanceID string
	treeID     int64
	lockFile   string
	client     *clientv3.Client
	session    *concurrency.Session
	election   *concurrency.Election
}

// Start commences election operation.
func (eme *MasterElection) Start(ctx context.Context) error {
	return nil
}

// WaitForMastership blocks until the current instance is master.
func (eme *MasterElection) WaitForMastership(ctx context.Context) error {
	return eme.election.Campaign(ctx, eme.instanceID)
}

// IsMaster returns whether the current instance is the master.
func (eme *MasterElection) IsMaster(ctx context.Context) (bool, error) {
	leader, err := eme.election.Leader(ctx)
	if err != nil {
		return false, err
	}
	return leader == eme.instanceID, nil
}

// ResignAndRestart releases mastership, and re-joins the election.
func (eme *MasterElection) ResignAndRestart(ctx context.Context) error {
	return eme.election.Resign(ctx)
}

// Close terminates election operation.
func (eme *MasterElection) Close(ctx context.Context) error {
	_ = eme.ResignAndRestart(ctx)
	if err := eme.session.Close(); err != nil {
		glog.Errorf("error closing session: %v", err)
	}
	return eme.client.Close()
}

// ElectionFactory creates etcd.MasterElection instances.
type ElectionFactory struct {
	client     *clientv3.Client
	instanceID string
	lockDir    string
}

// NewElectionFactory builds an election factory that uses the given parameters.
func NewElectionFactory(instanceID string, client *clientv3.Client, lockDir string) *ElectionFactory {
	return &ElectionFactory{
		client:     client,
		instanceID: instanceID,
		lockDir:    lockDir,
	}
}

// NewElection creates a specific etcd.MasterElection instance.
func (ef ElectionFactory) NewElection(ctx context.Context, treeID int64) (util.MasterElection, error) {
	session, err := concurrency.NewSession(ef.client)
	if err != nil {
		return nil, fmt.Errorf("failed to create etcd session: %v", err)
	}
	lockFile := fmt.Sprintf("%s/%d", strings.TrimRight(ef.lockDir, "/"), treeID)
	election := concurrency.NewElection(session, lockFile)

	eme := MasterElection{
		instanceID: ef.instanceID,
		treeID:     treeID,
		lockFile:   lockFile,
		client:     ef.client,
		session:    session,
		election:   election,
	}
	glog.Infof("MasterElection created: %+v", eme)
	return &eme, nil
}
