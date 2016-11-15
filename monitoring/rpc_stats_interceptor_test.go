package monitoring

import (
	"errors"
	"expvar"
	"fmt"
	"testing"
	"time"

	"github.com/google/trillian/util"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// Arbitrary time for use in tests
var fakeTime = time.Date(2016, 10, 3, 12, 38, 27, 36, time.UTC)

type recordingUnaryHandler struct {
	called bool
	ctx    context.Context
	req    interface{}
	resp   interface{}
	err    error
}

func (r recordingUnaryHandler) handler() grpc.UnaryHandler {
	return func(ctx context.Context, req interface{}) (interface{}, error) {
		r.ctx = ctx
		r.req = req

		return r.resp, r.err
	}
}

type singleRequestTestCase struct {
	name       string
	method     string
	handler    recordingUnaryHandler
	timeSource util.IncrementingFakeTimeSource
	panics     bool
}

// This is an OK request with 500ms latency
var okRequest500 = singleRequestTestCase{name: "ok request", method: "getmethod", handler: recordingUnaryHandler{req: "OK", err: nil}, timeSource: util.IncrementingFakeTimeSource{BaseTime: fakeTime, Increments: []time.Duration{0, time.Millisecond * 500}}}

// This is an errored request with 3000ms latency
var errorRequest3000 = singleRequestTestCase{name: "error request", method: "setmethod", handler: recordingUnaryHandler{err: errors.New("bang")}, timeSource: util.IncrementingFakeTimeSource{BaseTime: fakeTime, Increments: []time.Duration{0, time.Millisecond * 3000}}}

// This request panics with 1500ms latency
var panicRequest1500 = singleRequestTestCase{name: "panic request", method: "getmethod", panics: true, handler: recordingUnaryHandler{req: "OK", err: nil}, timeSource: util.IncrementingFakeTimeSource{BaseTime: fakeTime, Increments: []time.Duration{0, time.Millisecond * 1500}}}

var singleRequestTestCases = []singleRequestTestCase{okRequest500, errorRequest3000, panicRequest1500}

func TestSingleRequests(t *testing.T) {
	for _, req := range singleRequestTestCases {
		req.execute(t)
	}
}

func TestMultipleOKRequestsTotalLatency(t *testing.T) {
	// We're going to make 3 requests so set up the time source appropriately
	ts := util.IncrementingFakeTimeSource{BaseTime: fakeTime, Increments: []time.Duration{0, time.Millisecond * 500, 0, time.Millisecond * 2000, 0, time.Millisecond * 1337}}
	handler := recordingUnaryHandler{resp: "OK", err: nil}
	stats := NewRPCStatsInterceptor(&ts, "test", "test")
	i := stats.Interceptor()

	for r := 0; r < 3; r++ {
		resp, err := i(context.Background(), "wibble", &grpc.UnaryServerInfo{FullMethod: "testmethod"}, handler.handler())
		if resp != "OK" || err != nil {
			t.Fatalf("request handler returned an error unexpectedly")
		}
	}

	if want, got := "3837", stats.handlerRequestSucceededLatencyMap.Get("testmethod").String(); want != got {
		t.Fatalf("wanted total latency: %s but got: %s", want, got)

	}
	if !testMapSizeIs(stats.handlerRequestFailedLatencyMap, 0) {
		t.Fatalf("incorrectly recorded success latency on errors")
	}
}

func TestMultipleErrorRequestsTotalLatency(t *testing.T) {
	// We're going to make 3 requests so set up the time source appropriately
	ts := util.IncrementingFakeTimeSource{BaseTime: fakeTime, Increments: []time.Duration{0, time.Millisecond * 427, 0, time.Millisecond * 1066, 0, time.Millisecond * 1123}}
	handler := recordingUnaryHandler{resp: "", err: errors.New("bang")}
	stats := NewRPCStatsInterceptor(&ts, "test", "test")
	i := stats.Interceptor()

	for r := 0; r < 3; r++ {
		_, err := i(context.Background(), "wibble", &grpc.UnaryServerInfo{FullMethod: "testmethod"}, handler.handler())
		if err == nil {
			t.Fatalf("request handler did not return an error unexpectedly")
		}
	}

	if want, got := "2616", stats.handlerRequestFailedLatencyMap.Get("testmethod").String(); want != got {
		t.Fatalf("wanted total latency: %s but got: %s", want, got)
	}

	if !testMapSizeIs(stats.handlerRequestSucceededLatencyMap, 0) {
		t.Fatalf("incorrectly recorded success latency on errors")
	}
}

func (s singleRequestTestCase) execute(t *testing.T) {
	stats := NewRPCStatsInterceptor(&s.timeSource, "test", "test")
	i := stats.Interceptor()
	resp, err := i(context.Background(), "wibble", &grpc.UnaryServerInfo{FullMethod: s.method}, s.handler.handler())

	// These checks are the that the stats interceptor called the handler and correctly forwarded the
	// result and error returned by the wrapped request handler
	if got, want := s.handler.resp, resp; got != want {
		t.Fatalf("%s: Got result: %v but wanted: %v", s.name, got, want)
	}

	if (err != nil && s.handler.err == nil) || (err == nil && s.handler.err != nil) {
		t.Fatalf("%s: Error status was incorrect: %v got %v", s.name, s.handler.err, err)
	}

	// Now check the resulting state of the stats maps

	// Because we only made a single request there should only be one recorded (with either success or
	// failure depending on the error status and the other maps should count zero for the method
	if stats.handlerRequestCountMap.Get(s.method).String() != "1" {
		t.Fatalf("%s: Expected one request for method but got: %v", s.name, stats.handlerRequestCountMap.Get(s.method))
	}

	expectedTotalLatency := s.timeSource.Increments[1].Nanoseconds() / nanosToMillisDivisor

	var expectOneMap *expvar.Map
	var expectZeroMap *expvar.Map
	var expectLatencyMap *expvar.Map
	var expectNoLatencyMap *expvar.Map
	var logComment string

	if err == nil {
		// Request should have been a success
		expectOneMap = stats.handlerRequestSucceededCountMap
		expectZeroMap = stats.handlerRequestErrorCountMap
		expectLatencyMap = stats.handlerRequestSucceededLatencyMap
		expectNoLatencyMap = stats.handlerRequestFailedLatencyMap
		logComment = "ok"
	} else {
		// Request should be recorded as failed
		expectOneMap = stats.handlerRequestErrorCountMap
		expectZeroMap = stats.handlerRequestSucceededCountMap
		expectLatencyMap = stats.handlerRequestFailedLatencyMap
		expectNoLatencyMap = stats.handlerRequestSucceededLatencyMap
		logComment = "error"
	}

	// There should only be one key in the expected map and request count map
	if !testMapSizeIs(expectOneMap, 1) || !testMapSizeIs(expectZeroMap, 0) || !testMapSizeIs(stats.handlerRequestCountMap, 1) {
		t.Fatalf("%s: Map key counts are incorrect", s.name)
	}

	if !testMapSizeIs(expectLatencyMap, 1) || !testMapSizeIs(expectNoLatencyMap, 0) {
		t.Fatalf("%s: Latency map key counts are incorrect", s.name)
	}

	if expectOneMap.Get(s.method).String() != "1" {
		t.Fatalf("%s: Expected one %s request for method but got: %v", s.name, logComment, expectOneMap.Get(s.method))
	}

	if expectZeroMap.Get(s.method) != nil {
		t.Fatalf("%s: Expected zero %s request for method but got: %v", s.name, logComment, expectZeroMap.Get(s.method))
	}

	if expectLatencyMap.Get(s.method).String() != fmt.Sprintf("%d", expectedTotalLatency) {
		t.Fatalf("%s: Expected %s latency: %v but got: %v", s.name, logComment, expectedTotalLatency, expectLatencyMap.Get(s.method))
	}
	if expectNoLatencyMap.Get(s.method) != nil {
		t.Fatalf("%s: Expected %s latency: nil but got: %v", s.name, logComment, expectNoLatencyMap.Get(s.method))
	}
}

func testMapSizeIs(mapVar *expvar.Map, expectedSize int) bool {
	keys := 0
	mapVar.Do(func(expvar.KeyValue) {
		keys++
	})

	return keys == expectedSize
}
