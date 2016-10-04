package monitoring

import (
	"expvar"
	"fmt"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"github.com/google/trillian/util"
)

const (
	nanosToMillisDivisor int64 = 1000000

	requestCountMapName string = "requests-by-handler"
	successCountMapName string = "success-by-handler"
	errorCountMapName string = "errors-by-handler"
	successLatencyTotalMapName string = "successful-total-latency-by-handler-ms"
	errorLatencyTotalMapName string = "errored-total-latency-by-handler-ms"
)

type RpcStatsInterceptor struct {
	baseName                           string
	timeSource                         util.TimeSource
	handlerRequestCountMap             *expvar.Map
	handlerRequestSucceededMap         *expvar.Map
	handlerRequestErrorsMap            *expvar.Map
	handlerRequestSuccessfulLatencyMap *expvar.Map
	handlerRequestFailedLatencyMap     *expvar.Map
}

func NewRpcStatsInterceptor(timeSource util.TimeSource, application, component string) *RpcStatsInterceptor {
	return &RpcStatsInterceptor{baseName: fmt.Sprintf("%s/%s", application, component), timeSource: timeSource,
		handlerRequestCountMap: new(expvar.Map).Init(),
		handlerRequestSucceededMap: new(expvar.Map).Init(),
		handlerRequestErrorsMap: new(expvar.Map).Init(),
		handlerRequestSuccessfulLatencyMap: new(expvar.Map).Init(),
		handlerRequestFailedLatencyMap: new(expvar.Map).Init()}
}

func (r RpcStatsInterceptor) nameForMap(name string) string {
	return fmt.Sprintf("%s/%s", r.baseName, name)
}

// Publish must be called for stats to be visible. The expvar framework will prevent
// multiple calls to Publish from succeeding.
func (r RpcStatsInterceptor) Publish() {
	expvar.Publish(r.nameForMap(requestCountMapName), r.handlerRequestCountMap)
	expvar.Publish(r.nameForMap(successCountMapName), r.handlerRequestSucceededMap)
	expvar.Publish(r.nameForMap(errorCountMapName), r.handlerRequestErrorsMap)
	expvar.Publish(r.nameForMap(successLatencyTotalMapName), r.handlerRequestSuccessfulLatencyMap)
	expvar.Publish(r.nameForMap(errorLatencyTotalMapName), r.handlerRequestFailedLatencyMap)
}

func (r RpcStatsInterceptor) recordFailureLatency(method string, startTime time.Time) {
	latency := r.timeSource.Now().Sub(startTime)
	r.handlerRequestErrorsMap.Add(method, 1)
	r.handlerRequestFailedLatencyMap.Add(method, latency.Nanoseconds() / nanosToMillisDivisor)
}

// Interceptor returns a UnaryServerInterceptor that can be registered with an RPC server and
// will record request counts / errors and latencies for that servers handlers
func (r RpcStatsInterceptor) Interceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		method := info.FullMethod

		// Increase the request count for the method and start the clock
		r.handlerRequestCountMap.Add(method, 1)
		startTime := r.timeSource.Now()

		defer func() {
			if rec := recover(); rec != nil {
				// If we reach here then the handler exited via panic, count it as a server failure
				r.recordFailureLatency(method, startTime)
			}
		}()

		// Invoke the actual operation
		res, err := handler(ctx, req)

		// Record success / failure and latency
		if (err != nil) {
			r.recordFailureLatency(method, startTime)
		} else {
			latency := r.timeSource.Now().Sub(startTime)

			r.handlerRequestSucceededMap.Add(method, 1)
			r.handlerRequestSuccessfulLatencyMap.Add(method, latency.Nanoseconds() / nanosToMillisDivisor)
		}

		// Pass the result of the handler invocation back
		return res, err
	}
}
