// Copyright 2018 Google LLC. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package opencensus

import (
	"context"
	"errors"
	"net/http"

	"contrib.go.opencensus.io/exporter/stackdriver"
	"go.opencensus.io/plugin/ocgrpc"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/trace"
	"google.golang.org/grpc"
)

// This is the same set of views that used to be the default before that
// was deprecated. Possibly some of these are not useful but for the moment
// we don't really know that.
var serverViews = []*view.View{
	ochttp.ServerRequestCountView,
	ochttp.ServerRequestBytesView,
	ochttp.ServerResponseBytesView,
	ochttp.ServerLatencyView,
	ochttp.ServerRequestCountByMethod,
	ochttp.ServerResponseCountByStatusCode,
}

// EnableRPCServerTracing turns on Stackdriver tracing. The returned
// options must be passed to the GRPC server. The supplied
// projectID can be nil for GCP but might need to be set for other
// cloud platforms. Refer to the appropriate documentation. The percentage
// of traced requests can be set between 0 and 100. Note that 0 does not
// disable tracing entirely but causes the default configuration to be used.
func EnableRPCServerTracing(projectID string, percent int) ([]grpc.ServerOption, error) {
	if err := exporter(projectID); err != nil {
		return nil, err
	}
	if err := applyConfig(percent); err != nil {
		return nil, err
	}
	// Register the views to collect server request count.
	if err := view.Register(ocgrpc.DefaultServerViews...); err != nil {
		return nil, err
	}

	return []grpc.ServerOption{grpc.StatsHandler(&ocgrpc.ServerHandler{})}, nil
}

// EnableHTTPServerTracing turns on Stackdriver tracing for HTTP requests
// on the default ServeMux. The returned handler must be passed to the HTTP
// server. The supplied projectID can be nil for GCP but might need to be set
// for other cloud platforms. Refer to the appropriate documentation.
// The percentage of traced requests can be set between 0 and 100. Note that 0
// does not disable tracing entirely but causes the default configuration to be
// used.
func EnableHTTPServerTracing(projectID string, percent int) (http.Handler, error) {
	if err := exporter(projectID); err != nil {
		return nil, err
	}
	if err := applyConfig(percent); err != nil {
		return nil, err
	}
	if err := view.Register(serverViews...); err != nil {
		return nil, err
	}
	return &ochttp.Handler{}, nil
}

func exporter(projectID string) error {
	sde, err := stackdriver.NewExporter(stackdriver.Options{ProjectID: projectID})
	if err != nil {
		return err
	}
	view.RegisterExporter(sde)
	trace.RegisterExporter(sde)

	return nil
}

func applyConfig(percent int) error {
	switch {
	case percent == 0:
		// Use the default config, which traces relatively few requests.
	case percent == 100:
		trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})
	case percent > 100:
		return errors.New("cannot trace more than 100 percent of requests")
	default:
		trace.ApplyConfig(trace.Config{DefaultSampler: trace.ProbabilitySampler(float64(percent) / 100.0)})
	}
	return nil
}

// StartSpan starts a new tracing span.
// The returned context should be used for all child calls within the span, and
// the returned func should be called to close the span.
func StartSpan(ctx context.Context, name string) (context.Context, func()) {
	ctx, span := trace.StartSpan(ctx, name)
	return ctx, span.End
}
