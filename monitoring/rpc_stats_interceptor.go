package monitoring

import (
	"expvar"
	"fmt"
	"time"

	"github.com/google/trillian/util"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

const (
	nanosToMillisDivisor int64 = 1000000

	requestCountMapName            string = "requests-by-handler"
	requestSucceededCountMapName   string = "success-by-handler"
	requestErrorCountMapName       string = "errors-by-handler"
	requestSucceededLatencyMapName string = "succeeded-request-total-latency-by-handler-ms"
	requestFailedLatencyMapName    string = "failed-request-total-latency-by-handler-ms"
)

// RPCStatsInterceptor provides a gRPC interceptor that records statistics about the RPCs passing through it.
type RPCStatsInterceptor struct {
	baseName                          string
	timeSource                        util.TimeSource
	handlerRequestCountMap            *expvar.Map
	handlerRequestSucceededCountMap   *expvar.Map
	handlerRequestErrorCountMap       *expvar.Map
	handlerRequestSucceededLatencyMap *expvar.Map
	handlerRequestFailedLatencyMap    *expvar.Map
}

// NewRPCStatsInterceptor creates a new RPCStatsInterceptor for the given application/component, with
// a specified time source.
func NewRPCStatsInterceptor(timeSource util.TimeSource, application, component string) *RPCStatsInterceptor {
	return &RPCStatsInterceptor{baseName: fmt.Sprintf("%s/%s", application, component), timeSource: timeSource,
		handlerRequestCountMap:            new(expvar.Map).Init(),
		handlerRequestSucceededCountMap:   new(expvar.Map).Init(),
		handlerRequestErrorCountMap:       new(expvar.Map).Init(),
		handlerRequestSucceededLatencyMap: new(expvar.Map).Init(),
		handlerRequestFailedLatencyMap:    new(expvar.Map).Init()}
}

func (r RPCStatsInterceptor) nameForMap(name string) string {
	return fmt.Sprintf("%s/%s", r.baseName, name)
}

// Publish must be called for stats to be visible. The expvar framework will prevent
// multiple calls to Publish from succeeding.
func (r RPCStatsInterceptor) Publish() {
	expvar.Publish(r.nameForMap(requestCountMapName), r.handlerRequestCountMap)
	expvar.Publish(r.nameForMap(requestSucceededCountMapName), r.handlerRequestSucceededCountMap)
	expvar.Publish(r.nameForMap(requestErrorCountMapName), r.handlerRequestErrorCountMap)
	expvar.Publish(r.nameForMap(requestSucceededLatencyMapName), r.handlerRequestSucceededLatencyMap)
	expvar.Publish(r.nameForMap(requestFailedLatencyMapName), r.handlerRequestFailedLatencyMap)
}

func (r RPCStatsInterceptor) recordFailureLatency(method string, startTime time.Time) {
	latency := r.timeSource.Now().Sub(startTime)
	r.handlerRequestErrorCountMap.Add(method, 1)
	r.handlerRequestFailedLatencyMap.Add(method, latency.Nanoseconds()/nanosToMillisDivisor)
}

// Interceptor returns a UnaryServerInterceptor that can be registered with an RPC server and
// will record request counts / errors and latencies for that servers handlers
func (r RPCStatsInterceptor) Interceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		method := info.FullMethod

		// Increase the request count for the method and start the clock
		r.handlerRequestCountMap.Add(method, 1)
		startTime := r.timeSource.Now()

		defer func() {
			if rec := recover(); rec != nil {
				// If we reach here then the handler exited via panic, count it as a server failure
				r.recordFailureLatency(method, startTime)
				panic(rec)
			}
		}()

		// Invoke the actual operation
		res, err := handler(ctx, req)

		// Record success / failure and latency
		if err != nil {
			r.recordFailureLatency(method, startTime)
		} else {
			latency := r.timeSource.Now().Sub(startTime)

			r.handlerRequestSucceededCountMap.Add(method, 1)
			r.handlerRequestSucceededLatencyMap.Add(method, latency.Nanoseconds()/nanosToMillisDivisor)
		}

		// Pass the result of the handler invocation back
		return res, err
	}
}
