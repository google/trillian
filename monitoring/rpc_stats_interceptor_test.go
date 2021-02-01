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

package monitoring_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/util/clock"
	"google.golang.org/grpc"
)

// Arbitrary time for use in tests
var fakeTime = time.Date(2016, 10, 3, 12, 38, 27, 36, time.UTC)

type recordingUnaryHandler struct {
	// ctx and req are recorded on invocation
	ctx context.Context
	req interface{}
	// rsp and err are returned on invocation
	rsp interface{}
	err error
}

func (r recordingUnaryHandler) handler() grpc.UnaryHandler {
	return func(ctx context.Context, req interface{}) (interface{}, error) {
		r.ctx = ctx
		r.req = req
		return r.rsp, r.err
	}
}

func TestSingleRequests(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		handler    recordingUnaryHandler
		timeSource clock.PredefinedFake
	}{
		// This is an OK request with 500ms latency
		{
			name:    "ok_request",
			method:  "getmethod",
			handler: recordingUnaryHandler{req: "OK", err: nil},
			timeSource: clock.PredefinedFake{
				Base:   fakeTime,
				Delays: []time.Duration{0, time.Millisecond * 500},
			},
		},
		// This is an errored request with 3000ms latency
		{
			name:    "error_request",
			method:  "setmethod",
			handler: recordingUnaryHandler{err: errors.New("bang")},
			timeSource: clock.PredefinedFake{
				Base:   fakeTime,
				Delays: []time.Duration{0, time.Millisecond * 3000},
			},
		},
	}

	for _, test := range tests {
		prefix := fmt.Sprintf("test_%s", test.name)
		stats := monitoring.NewRPCStatsInterceptor(&test.timeSource, prefix, monitoring.InertMetricFactory{})
		i := stats.Interceptor()

		// Invoke the test handler wrapped by the interceptor.
		got, err := i(context.Background(), "wibble", &grpc.UnaryServerInfo{FullMethod: test.method}, test.handler.handler())

		// Check the interceptor passed through the results.
		if got != test.handler.rsp || (err != nil) != (test.handler.err != nil) {
			t.Errorf("interceptor(%s)=%v,%v; want %v,%v", test.name, got, err, test.handler.rsp, test.handler.err)
		}

		// Now check the resulting state of the metrics.
		if got, want := stats.ReqCount.Value(test.method), 1.0; got != want {
			t.Errorf("stats.ReqCount=%v; want %v", got, want)
		}
		wantLatency := test.timeSource.Delays[1].Seconds()
		wantErrors := 0.0
		wantSuccess := 0.0
		if test.handler.err == nil {
			wantSuccess = 1.0
		} else {
			wantErrors = 1.0
		}
		if got := stats.ReqSuccessCount.Value(test.method); got != wantSuccess {
			t.Errorf("stats.ReqSuccessCount=%v; want %v", got, wantSuccess)
		}
		if got := stats.ReqErrorCount.Value(test.method); got != wantErrors {
			t.Errorf("stats.ReqErrorCount=%v; want %v", got, wantSuccess)
		}

		if gotCount, gotSum := stats.ReqSuccessLatency.Info(test.method); gotCount != uint64(wantSuccess) {
			t.Errorf("stats.ReqSuccessLatency.Count=%v; want %v", gotCount, wantSuccess)
		} else if gotSum != wantLatency*wantSuccess {
			t.Errorf("stats.ReqSuccessLatency.Sum=%v; want %v", gotSum, wantLatency*wantSuccess)
		}
		if gotCount, gotSum := stats.ReqErrorLatency.Info(test.method); gotCount != uint64(wantErrors) {
			t.Errorf("stats.ReqErrorLatency.Count=%v; want %v", gotCount, wantErrors)
		} else if gotSum != wantLatency*wantErrors {
			t.Errorf("stats.ReqErrorLatency.Sum=%v; want %v", gotSum, wantLatency*wantErrors)
		}
	}
}

func TestMultipleOKRequestsTotalLatency(t *testing.T) {
	// We're going to make 3 requests so set up the time source appropriately
	ts := clock.PredefinedFake{
		Base: fakeTime,
		Delays: []time.Duration{
			0,
			time.Millisecond * 500,
			0,
			time.Millisecond * 2000,
			0,
			time.Millisecond * 1337,
		},
	}
	handler := recordingUnaryHandler{rsp: "OK", err: nil}
	stats := monitoring.NewRPCStatsInterceptor(&ts, "test_multi_ok", monitoring.InertMetricFactory{})
	i := stats.Interceptor()

	for r := 0; r < 3; r++ {
		rsp, err := i(context.Background(), "wibble", &grpc.UnaryServerInfo{FullMethod: "testmethod"}, handler.handler())
		if rsp != "OK" || err != nil {
			t.Fatalf("interceptor()=%v,%v; want 'OK',nil", rsp, err)
		}
	}
	count, sum := stats.ReqSuccessLatency.Info("testmethod")
	if wantCount, wantSum := uint64(3), time.Duration(3837*time.Millisecond).Seconds(); count != wantCount || sum != wantSum {
		t.Errorf("stats.ReqSuccessLatency.Info=%v,%v; want %v,%v", count, sum, wantCount, wantSum)
	}
}

func TestMultipleErrorRequestsTotalLatency(t *testing.T) {
	// We're going to make 3 requests so set up the time source appropriately
	ts := clock.PredefinedFake{
		Base: fakeTime,
		Delays: []time.Duration{
			0,
			time.Millisecond * 427,
			0,
			time.Millisecond * 1066,
			0,
			time.Millisecond * 1123,
		},
	}
	handler := recordingUnaryHandler{rsp: "", err: errors.New("bang")}
	stats := monitoring.NewRPCStatsInterceptor(&ts, "test_multi_err", monitoring.InertMetricFactory{})
	i := stats.Interceptor()

	for r := 0; r < 3; r++ {
		_, err := i(context.Background(), "wibble", &grpc.UnaryServerInfo{FullMethod: "testmethod"}, handler.handler())
		if err == nil {
			t.Fatalf("interceptor()=_,%v; want _,'bang'", err)
		}
	}

	count, sum := stats.ReqErrorLatency.Info("testmethod")
	if wantCount, wantSum := uint64(3), 2.6160; count != wantCount || sum != wantSum {
		t.Errorf("stats.ReqSuccessLatency.Info=%v,%v; want %v,%v", count, sum, wantCount, wantSum)
	}
}

func TestCanInitializeNilMetricFactory(t *testing.T) {
	ts := clock.PredefinedFake{
		Base:   fakeTime,
		Delays: []time.Duration{},
	}
	monitoring.NewRPCStatsInterceptor(&ts, "test_nil_metric_factory", nil)
	// Should reach here without throwing an exception
}
