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

// The mdmtest binary runs merge delay tests against a Trillian Log.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"time"

	"github.com/golang/glog"
	"github.com/golang/protobuf/ptypes"
	"github.com/google/trillian"
	"github.com/google/trillian/client"
	"github.com/google/trillian/crypto/keyspb"
	"github.com/google/trillian/crypto/sigpb"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/monitoring/prometheus"
	"github.com/google/trillian/testonly/mdm"
	"github.com/google/trillian/util"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"

	// Register key ProtoHandlers
	_ "github.com/google/trillian/crypto/keys/der/proto"
	_ "github.com/google/trillian/crypto/keys/pem/proto"

	// Load hashers
	_ "github.com/google/trillian/merkle/rfc6962"
)

var (
	logID           = flag.Int64("log_id", 0, "Log ID to test against; ephemeral tree used if zero")
	rpcServer       = flag.String("rpc_server", "", "Trillian log server address:port")
	adminServer     = flag.String("admin_server", "", "Trillian admin server address:port (defaults to --rpc_server value)")
	metricsEndpoint = flag.String("metrics_endpoint", "", "Endpoint for serving metrics; if left empty, metrics will not be exposed")
	leafSize        = flag.Uint("leaf_size", 500, "Size of leaf values")
	newLeafChance   = flag.Uint("new_leaf_chance", 50, "Percentage chance of using a new leaf for each submission")
	checkers        = flag.Int("checkers", 3, "Number of parallel checker goroutines to run")
	emitInterval    = flag.Duration("emit_interval", 10*time.Second, "How often to output the summary info")
	deadline        = flag.Duration("deadline", 60*time.Second, "Deadline for single add+get-proof operation")
	minMergeDelay   = flag.Duration("min_merge_delay", 3*time.Second, "Minimum merge delay; don't check for inclusion until this interval has passed")
)

func main() {
	flag.Parse()
	ctx := context.Background()
	if err := innerMain(ctx); err != nil {
		glog.Exit(err)
	}
}

func innerMain(ctx context.Context) error {
	var mf monitoring.MetricFactory
	if *metricsEndpoint != "" {
		mf = prometheus.MetricFactory{}
		http.Handle("/metrics", promhttp.Handler())
		server := http.Server{Addr: *metricsEndpoint, Handler: nil}
		glog.Infof("Serving metrics at %v", *metricsEndpoint)
		go func() {
			err := server.ListenAndServe()
			glog.Warningf("Metrics server exited: %v", err)
		}()
	}

	dialOpts := []grpc.DialOption{grpc.WithInsecure()}
	c, err := grpc.Dial(*rpcServer, dialOpts...)
	if err != nil {
		glog.Exitf("Failed to create log client conn: %v", err)
	}
	cl := trillian.NewTrillianLogClient(c)

	ac := c
	if len(*adminServer) > 0 {
		ac, err = grpc.Dial(*adminServer, dialOpts...)
		if err != nil {
			glog.Exitf("Failed to create admin client conn: %v", err)
		}
	}
	adminCl := trillian.NewTrillianAdminClient(ac)

	if *logID <= 0 {
		// No logID provided, so create an ephemeral tree to test against.
		req := trillian.CreateTreeRequest{
			Tree: &trillian.Tree{
				TreeState:          trillian.TreeState_ACTIVE,
				TreeType:           trillian.TreeType_LOG,
				HashStrategy:       trillian.HashStrategy_RFC6962_SHA256,
				HashAlgorithm:      sigpb.DigitallySigned_SHA256,
				SignatureAlgorithm: sigpb.DigitallySigned_ECDSA,
				DisplayName:        fmt.Sprintf("mdmtest-%d", time.Now().UnixNano()/int64(time.Second)),
				Description:        "Transient tree for mdmtest",
				MaxRootDuration:    ptypes.DurationProto(time.Second * 3600),
			},
			KeySpec: &keyspb.Specification{
				Params: &keyspb.Specification_EcdsaParams{
					EcdsaParams: &keyspb.Specification_ECDSA{
						Curve: keyspb.Specification_ECDSA_P256,
					},
				},
			},
		}
		tree, err := client.CreateAndInitTree(ctx, &req, adminCl, nil, cl)
		if err != nil {
			glog.Exitf("failed to create ephemeral tree: %v", err)
		}
		*logID = tree.TreeId
		glog.Infof("testing against ephemeral tree %d", *logID)
		defer func() {
			req := &trillian.DeleteTreeRequest{TreeId: *logID}
			glog.Infof("Soft-delete transient Trillian Log with TreeID=%d", *logID)
			if _, err := adminCl.DeleteTree(ctx, req); err != nil {
				glog.Errorf("failed to DeleteTree(%d): %v", *logID, err)
			}
		}()
	}

	opts := mdm.MergeDelayOptions{
		ParallelAdds:  *checkers,
		LeafSize:      int(*leafSize),
		NewLeafChance: int(*newLeafChance),
		EmitInterval:  *emitInterval,
		Deadline:      *deadline,
		MinMergeDelay: *minMergeDelay,
		MetricFactory: mf,
	}
	monitor, err := mdm.NewMonitor(ctx, *logID, cl, adminCl, opts)
	if err != nil {
		return fmt.Errorf("failed to build merge delay monitor: %v", err)
	}

	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go util.AwaitSignal(ctx, cancel)
	if err := monitor.Monitor(cctx); err != nil {
		return fmt.Errorf("merge delay monitoring failed: %v", err)
	}
	return nil
}
