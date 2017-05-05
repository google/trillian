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

package quota

import (
	"context"
)

// MaxTokens is the maximum number of available tokens a quota may have.
const MaxTokens = int(^uint(0) >> 1) // MaxInt

// Group represents the scope of a token (Global, Tree or User).
type Group int

const (
	// Global is the Trillian-wide token scope (applies to all users and trees).
	// A global quota shortage for a certain kind of token means all requests of that kind will
	// be denied until the quota is replenished.
	Global Group = iota

	// Tree is the tree-wide token scope.
	Tree

	// User it the per-user token scope.
	// Users are defined according to each implementation.
	User
)

// Kind represents the purpose of each token (Read or Write).
type Kind int

const (
	// Read represents tokens used by non-modifying RPCs.
	Read Kind = iota

	// Write represents tokens used by modifying RPCs.
	Write
)

// Spec represents a combination of Group and Kind, with all additional data required to get / put
// tokens.
type Spec struct {
	// Group of the spec.
	Group

	// Kind of the spec.
	Kind

	// TreeID identifies the tree for specs of the Tree group.
	// Not used for other specs.
	TreeID int64

	// User identifies the user for specs of the User group.
	// Not used for other specs.
	User string
}

// Manager is the component responsible for the management of tokens.
type Manager interface {
	// GetUser returns the quota user, as defined by the manager implementation.
	// req is the RPC request message.
	GetUser(ctx context.Context, req interface{}) string

	// GetTokens acquires numTokens from all specs. Tokens are taken in the order specified by
	// specs.
	// Returns error if numTokens could not be acquired for all specs.
	GetTokens(ctx context.Context, numTokens int, specs []Spec) error

	// PeekTokens returns how many tokens are available for each spec, without acquiring any.
	// Infinite quotas should return MaxTokens.
	PeekTokens(ctx context.Context, specs []Spec) (map[Spec]int, error)

	// PutTokens adds numTokens for all specs.
	PutTokens(ctx context.Context, numTokens int, specs []Spec) error

	// ResetQuota resets the quota for all specs.
	ResetQuota(ctx context.Context, specs []Spec) error
}
