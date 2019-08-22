// Package timeout enforces a maximum timeout on all outgoing rpcs.
package timeout

import (
	"context"
	"time"

	"google.golang.org/grpc"
)

// UnaryClientInterceptor returns a new maximum timout client interceptor.
func UnaryClientInterceptor(maxTimeout time.Duration) grpc.UnaryClientInterceptor {
	return func(parentCtx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		callCtx, cancel := context.WithTimeout(parentCtx, maxTimeout)
		defer cancel()
		return invoker(callCtx, method, req, reply, cc, opts...)
	}
}
