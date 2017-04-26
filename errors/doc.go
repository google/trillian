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

// Package errors defines an error representation that associates an error
// message to an error code.
//
// It's meant to allow translation to other kinds of errors (e.g., gRPC errors)
// without information loss, while at the same time keeping the implementation
// independent of specific libraries.
//
// Errors created by this package are meant to be user-visible, therefore care
// must be taken to ensure that both messages and error codes are chosen
// according to the perspective of the RPC caller. For example, a function that
// reads a file may be tempted to return an error with a NotFound code, but from
// the perspective of the RPC caller, FailedPrecondition would be clearer (as it
// indicates a bad argument on the request).
package errors
