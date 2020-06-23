// Copyright 2016 Google LLC. All Rights Reserved.
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

// Package monitoring provides monitoring functionality.
package monitoring

import (
	"context"
	"fmt"
	"time"

	"github.com/google/trillian/util/clock"
	"google.golang.org/grpc"
)

const traceSpanRoot = "/trillian/mon/"

// RPCStatsInterceptor provides a gRPC interceptor that records statistics about the RPCs passing through it.
type RPCStatsInterceptor struct {
	prefix            string
	timeSource        clock.TimeSource
	ReqCount          Counter
	ReqSuccessCount   Counter
	ReqSuccessLatency Histogram
	ReqErrorCount     Counter
	ReqErrorLatency   Histogram
}

// NewRPCStatsInterceptor creates a new RPCStatsInterceptor for the given application/component, with
// a specified time source.
func NewRPCStatsInterceptor(timeSource clock.TimeSource, prefix string, mf MetricFactory) *RPCStatsInterceptor {
	if mf == nil {
		mf = InertMetricFactory{}
	}
	interceptor := RPCStatsInterceptor{
		prefix:            prefix,
		timeSource:        timeSource,
		ReqCount:          mf.NewCounter(prefixedName(prefix, "rpc_requests"), "Number of requests", "method"),
		ReqSuccessCount:   mf.NewCounter(prefixedName(prefix, "rpc_success"), "Number of successful requests", "method"),
		ReqSuccessLatency: mf.NewHistogram(prefixedName(prefix, "rpc_success_latency"), "Latency of successful requests in seconds", "method"),
		ReqErrorCount:     mf.NewCounter(prefixedName(prefix, "rpc_errors"), "Number of errored requests", "method"),
		ReqErrorLatency:   mf.NewHistogram(prefixedName(prefix, "rpc_error_latency"), "Latency of errored requests in seconds", "method"),
	}
	return &interceptor
}

func prefixedName(prefix, name string) string {
	return fmt.Sprintf("%s_%s", prefix, name)
}

func (r *RPCStatsInterceptor) recordFailureLatency(labels []string, startTime time.Time) {
	latency := clock.SecondsSince(r.timeSource, startTime)
	r.ReqErrorCount.Inc(labels...)
	r.ReqErrorLatency.Observe(latency, labels...)
}

// Interceptor returns a UnaryServerInterceptor that can be registered with an RPC server and
// will record request counts / errors and latencies for that servers handlers
func (r *RPCStatsInterceptor) Interceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		labels := []string{info.FullMethod}

		// This interceptor wraps the request handler so we should track the
		// additional latency it imposes.
		ctx, spanEnd := StartSpan(ctx, traceSpanRoot)
		defer spanEnd()

		// Increase the request count for the method and start the clock
		r.ReqCount.Inc(labels...)
		startTime := r.timeSource.Now()

		defer func() {
			if rec := recover(); rec != nil {
				// If we reach here then the handler exited via panic, count it as a server failure
				r.recordFailureLatency(labels, startTime)
				panic(rec)
			}
		}()

		// Invoke the actual operation
		rsp, err := handler(ctx, req)

		// Record success / failure and latency
		if err != nil {
			r.recordFailureLatency(labels, startTime)
		} else {
			latency := clock.SecondsSince(r.timeSource, startTime)
			r.ReqSuccessCount.Inc(labels...)
			r.ReqSuccessLatency.Observe(latency, labels...)
		}

		// Pass the result of the handler invocation back
		return rsp, err
	}
}
