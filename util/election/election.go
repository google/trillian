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

// Package election provides implementation of master election and tracking, as
// well as interface for plugging in a custom underlying mechanism.
package election

import (
	"context"
	"errors"
)

// TODO(pavelkalinnikov): Remove interfaces in this file, and their
// implementations, after election2 supports GetCurrentMaster method.

// MasterElection provides operations for determining if a local instance is
// the current master for a particular election.
type MasterElection interface {
	// Start kicks off this instance's participation in master election.
	Start(context.Context) error
	// WaitForMastership blocks until the current instance is the master.
	WaitForMastership(context.Context) error
	// IsMaster returns whether the current instance is the master.
	IsMaster(context.Context) (bool, error)
	// Resign releases mastership, and stops this instance from participating in
	// the master election.
	Resign(context.Context) error
	// Close releases all the resources associated with this MasterElection.
	Close(context.Context) error
	// GetCurrentMaster returns the instance ID of the current elected master, if
	// any. Implementations should allow election participants to specify their
	// instance ID string, participants should ensure that it is unique to them.
	// If there is currently no master, ErrNoMaster will be returned.
	GetCurrentMaster(context.Context) (string, error)
}

// ErrNoMaster indicates that there is currently no master elected.
var ErrNoMaster = errors.New("no master")

// Factory encapsulates the creation of a MasterElection instance for a resourceID.
// TreeID may be used as a resourceID.
type Factory interface {
	NewElection(ctx context.Context, resourceID string) (MasterElection, error)
}

// NoopElection is a stub implementation that tells every instance that it is master.
type NoopElection struct {
	resourceID string
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

// Resign releases mastership.
func (ne *NoopElection) Resign(ctx context.Context) error {
	return nil
}

// Close permanently stops the mastership election process.
func (ne *NoopElection) Close(ctx context.Context) error {
	return nil
}

// GetCurrentMaster returns ne.instanceID
func (ne *NoopElection) GetCurrentMaster(ctx context.Context) (string, error) {
	return ne.instanceID, nil
}

// NoopFactory creates NoopElection instances.
type NoopFactory struct {
	InstanceID string
}

// NewElection creates a specific NoopElection instance.
func (nf NoopFactory) NewElection(ctx context.Context, resourceID string) (MasterElection, error) {
	return &NoopElection{instanceID: nf.InstanceID, resourceID: resourceID}, nil
}
