// Copyright 2019 Google Inc. All Rights Reserved.
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

// Package timeout enforces a maximum timeout on all outgoing rpcs.
package timeout

import (
	"context"
	"time"

	"google.golang.org/grpc"
)

// UnaryClientInterceptor returns a new maximum timeout client interceptor.
// This interceptor will set a timeout when no timeout has been set.
// If the timeout on parentCtx is longer than maxTimeout, it will be replaced with maxTimeout.
// If parentCtx has a timeout shorter than maxTimeout it will not be modified.
func UnaryClientInterceptor(maxTimeout time.Duration) grpc.UnaryClientInterceptor {
	return func(parentCtx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		callCtx, cancel := context.WithTimeout(parentCtx, maxTimeout)
		defer cancel()
		return invoker(callCtx, method, req, reply, cc, opts...)
	}
}
