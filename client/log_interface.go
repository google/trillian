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

package client

import (
	"context"

	"github.com/google/trillian"
)

// VerifyingLogClient is a client that verifies output from Trillian.
type VerifyingLogClient interface {
	// AddLeaf adds data to the Trillian Log and blocks until an inclusion proof
	// is available. If no proof is available within the ctx deadline, DeadlineExceeded
	// is returned.
	AddLeaf(ctx context.Context, data []byte) error
	// VerifyInclusion ensures that data has been included in the log
	// via an inclusion proof.
	VerifyInclusion(ctx context.Context, data []byte) error
	// VerifyInclusionAtIndex ensures that data has been included in the log
	// via in inclusion proof for a particular index.
	VerifyInclusionAtIndex(ctx context.Context, data []byte, index int64) error
	// UpdateRoot fetches and verifies the current SignedTreeRoot.
	// It checks signatures as well as consistency proofs from the last-seen root.
	UpdateRoot(ctx context.Context) error
	// Root provides the last root obtained by UpdateRoot.
	Root() trillian.SignedLogRoot

	// The following methods do not perform cryptographic verification.

	// GetByIndex returns a single leaf. Does not verify the leaf's inclusion proof.
	GetByIndex(ctx context.Context, index int64) (*trillian.LogLeaf, error)
	// ListByIndex returns a contiguous range. Does not verify the leaf's inclusion proof.
	ListByIndex(ctx context.Context, start, count int64) ([]*trillian.LogLeaf, error)
}

// LogVerifier verifies responses from a Trillian Log.
type LogVerifier interface {
	// VerifyRoot verifies that newRoot is a valid append-only operation from trusted.
	// If trusted.TreeSize is zero, an append-only proof is not needed.
	VerifyRoot(trusted, newRoot *trillian.SignedLogRoot, consistency [][]byte) error
	// VerifyInclusionAtIndex verifies that the inclusion proof for data at index matches
	// the currently trusted root. The inclusion proof must be requested for Root().TreeSize.
	VerifyInclusionAtIndex(trusted *trillian.SignedLogRoot, data []byte, leafIndex int64, proof [][]byte) error
	// VerifyInclusionByHash verifies the inclusion proof for data
	VerifyInclusionByHash(trusted *trillian.SignedLogRoot, leafHash []byte, proof *trillian.Proof) error
}
