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
	// UpdateRoot fetches and verifies the current SignedTreeRoot.
	// It checks signatures as well as consistency proofs from the last-seen root.
	UpdateRoot(ctx context.Context) error
	// Root provides the last root obtained by UpdateRoot.
	Root() trillian.SignedLogRoot
}
