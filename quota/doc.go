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

// Package quota defines Trillian's Quota Management service.
//
// The objective of the quota service is to protect Trillian from traffic peaks, rejecting requests
// that may put servers out of capacity or indirectly cause MMDs (maximum merge delays) to be
// missed.
//
// Each Trillian request, be it either a read or write request, requires certain tokens to be
// allowed to continue. Tokens exist at multiple layers: per-user, per-tree and global tokens.
// For example, a TrillianLog.QueueLeaves request consumes a Write token from User, Tree and Global
// quotas. If any of those quotas is out of tokens, the request is denied with a ResourceExhausted
// error code.
//
// Tokens are replenished according to each implementation. For example, User tokens may replenish
// over time, whereas {Write, Tree} tokens may replenish as sequencing happens. Implementations are
// free to ignore (effectively whitelisting) certain specs of tokens (e.g., only support Global and
// ignore User and Tree tokens).
//
// Quota users are defined according to each implementation. Note that quota users don't need to
// match authentication/authorization users; implementations are allowed their own representation of
// users.
package quota
