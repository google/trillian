// Copyright 2018 Google LLC. All Rights Reserved.
//
// ￼Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// ￼You may obtain a copy of the License at
// ￼
// ￼     http://www.apache.org/licenses/LICENSE-2.0
// ￼
// ￼Unless required by applicable law or agreed to in writing, software
// ￼distributed under the License is distributed on an "AS IS" BASIS,
// ￼WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// ￼See the License for the specific language governing permissions and
// ￼limitations under the License.

package opencensus

import (
	"errors"

	"go.opencensus.io/exporter/stackdriver"
	"go.opencensus.io/plugin/ocgrpc"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/trace"
	"google.golang.org/grpc"
)

// EnableRPCServerTracing turns on Stackdriver tracing. The returned
// options must be passed to the GRPC server. The supplied
// projectID can be nil for GCP but might need to be set for other
// cloud platforms. Refer to the appropriate documentation.
func EnableRPCServerTracing(projectID string, percent int) ([]grpc.ServerOption, error) {
	sde, err := stackdriver.NewExporter(stackdriver.Options{ProjectID: projectID})
	if err != nil {
		return nil, err
	}
	view.RegisterExporter(sde)
	trace.RegisterExporter(sde)

	switch {
	case percent == 0:
		// Use the default config, which traces relatively few requests.
	case percent == 100:
		trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})
	case percent > 100:
		return nil, errors.New("cannot trace more than 100 percent of requests")
	default:
		trace.ApplyConfig(trace.Config{DefaultSampler: trace.ProbabilitySampler(float64(percent) / 100.0)})
	}

	// Register the views to collect server request count.
	if err := view.Register(ocgrpc.DefaultServerViews...); err != nil {
		return nil, err
	}

	return []grpc.ServerOption{grpc.StatsHandler(&ocgrpc.ServerHandler{})}, nil
}
