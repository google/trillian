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

package util

import "context"

// MasterElection provides operations for determining if a local instance is the current
// master for a particular election.
type MasterElection interface {
	// Start kicks off the process of mastership election.
	Start(context.Context) error
	// WaitForMastership blocks until the current instance is master for this election.
	WaitForMastership(context.Context) error
	// IsMaster returns whether the current instance is master.
	IsMaster(context.Context) (bool, error)
	// ResignAndRestart releases mastership, and re-joins the election.
	ResignAndRestart(context.Context) error
	// Close permanently stops the mastership election process.
	Close(context.Context) error
}

// ElectionFactory encapsulates the creation of a MasterElection instance for a treeID.
type ElectionFactory interface {
	NewElection(ctx context.Context, treeID int64) (MasterElection, error)
}

// NoopElection is a stub implementation that tells every instance that it is master.
type NoopElection struct {
	treeID     int64
	instanceID string
}

// Start kicks off the process of mastership election.
func (ne *NoopElection) Start(ctx context.Context) error {
	return nil
}

// WaitForMastership blocks until the current instance is master for this election.
func (ne *NoopElection) WaitForMastership(ctx context.Context) error {
	return nil
}

// IsMaster returns whether the current instance is master.
func (ne *NoopElection) IsMaster(ctx context.Context) (bool, error) {
	return true, nil
}

// ResignAndRestart releases mastership, and re-joins the election.
func (ne *NoopElection) ResignAndRestart(ctx context.Context) error {
	return nil
}

// Close permanently stops the mastership election process.
func (ne *NoopElection) Close(ctx context.Context) error {
	return nil
}

// NoopElectionFactory creates NoopElection instances.
type NoopElectionFactory struct {
	InstanceID string
}

// NewElection creates a specific NoopElection instance.
func (nf NoopElectionFactory) NewElection(ctx context.Context, treeID int64) (MasterElection, error) {
	return &NoopElection{instanceID: nf.InstanceID, treeID: treeID}, nil
}
