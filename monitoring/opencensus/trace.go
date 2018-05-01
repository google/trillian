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
	"go.opencensus.io/exporter/stackdriver"
	"go.opencensus.io/plugin/ocgrpc"
	"go.opencensus.io/stats/view"
	"google.golang.org/grpc"
)

func EnableRPCServerTracing(projectID string) ([]grpc.ServerOption, error) {
	sde, err := stackdriver.NewExporter(stackdriver.Options{ProjectID: projectID})
	if err != nil {
		return nil, err
	}
	view.RegisterExporter(sde)

	// Register the views to collect server request count.
	if err := view.Subscribe(ocgrpc.DefaultServerViews...); err != nil {
		return nil, err
	}

	return []grpc.ServerOption{grpc.StatsHandler(&ocgrpc.ServerHandler{})}, nil
}
