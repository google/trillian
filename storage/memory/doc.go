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

// Package memory provides a simple in-process implementation of the tree- and
// log-storage interfaces.
//
// This implementation is intended SOLELY for use in integration tests which
// exercise properties of the higher levels of Trillian componened - e.g.
// an integration test which ensures that the Trillian Log is able to correctly
// handle a tree which contains duplicate leaves.
//
// The storage implementation is based on a BTree, which provides an ordered
// key-value space which can be used to store arbitrary items, as well as
// scan ranges of keys in order.
//
// The implementation does provide transaction-like semantics for the
// LogStorage interface, although conflict is avoided by each writable
// transaction exclusively locking the tree until it's committed or
// rolled-back.
//
// Currently, the Admin Storage does not honor transactional semantics.
package memory
