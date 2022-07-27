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

// Package election2 provides master election tools, and interfaces for
// plugging in a custom underlying mechanism.
//
// There are two important abstractions in this package: instance and resource.
//   - An instance is a single client of the library. An instance is represented
//     by an Election object (possibly multiple).
//   - A resource is something guarded by master election (e.g. a piece of data,
//     or operation). Each resource has at most one (most of the time; see note
//     below) master instance which is said to own this resource. A single
//     instance may own multiple resources (one resource per Election object).
//
// Note: Sometimes there can be more than 1 instance "believing" to own a
// resource. The reason is that the client code operates outside of the
// election mechanism (e.g. a distributed consensus group), so mastership
// updates can race with the operation.
//
// TODO(pavelkalinnikov): Merge this package with util/election.
package election2

import "context"

// Election controls an instance's participation in master election process.
// Note: Implementations are not intended to be thread-safe.
type Election interface {
	// Await blocks until the instance captures mastership. Returns immediately
	// if it is already the master. Returns an error if capturing fails, or the
	// passed in context is canceled before mastership is captured. If an error
	// is returned, the instance might still have become the master. Idempotent,
	// might be useful to retry in case of an error.
	Await(ctx context.Context) error

	// WithMastership returns a "mastership context" which remains active until
	// the instance stops being the master, or the passed in context is canceled.
	// If the instance is not the master during this call, returns an already
	// canceled context. In particular, this will happen if WithMastership is
	// called without a preceding Await.
	//
	// The resources used for maintaining the mastership context are released
	// when the latter gets canceled. This happens when the instance loses
	// mastership, calls Resign, an error occurs in mastership monitoring, or the
	// context passed in to WithMastership is explicitly canceled.
	//
	// If the passed in ctx is canceled, the instance does not resign mastership.
	// Use Resign or Close method for that.
	WithMastership(ctx context.Context) (context.Context, error)

	// Resign releases mastership for this instance. The instance can be elected
	// again using Await. Idempotent, might be useful to retry if fails.
	//
	// Note: Resign does not guarantee immediate cancelation of the context
	// returned from WithMastership. However, the latter will happen *eventually* if
	// resigning is successful. The caller can force mastership context
	// cancelation by explicitly canceling the context passed in to WithMastership.
	//
	// The caller is advised to tear down mastership-related work before invoking
	// Resign to have best protection against double-master situations.
	Resign(ctx context.Context) error

	// Close permanently stops participating in election, and releases the
	// resources. It does best effort on resigning despite potential cancelation
	// of the passed in context, so that other instances can overtake mastership
	// faster. No other method should be called after Close.
	//
	// Note: Does not guarantee immediate mastership context cancelation, see
	// Resign comment for details.
	Close(ctx context.Context) error
}

// Factory encapsulates the creation of an Election instance for a resource
// with the specified ID.
type Factory interface {
	NewElection(ctx context.Context, resourceID string) (Election, error)
}
