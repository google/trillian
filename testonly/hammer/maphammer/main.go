// Copyright 2017 Google Inc. All Rights Reserved.
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

// maphammer is a stress/load test for a Trillian Map.
package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/monitoring/prometheus"
	"github.com/google/trillian/testonly/hammer"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"

	_ "github.com/google/trillian/merkle/coniks"    // register CONIKS_SHA512_256
	_ "github.com/google/trillian/merkle/maphasher" // register TEST_MAP_HASHER
)

var (
	mapIDs          = flag.String("map_ids", "", "Comma-separated list of map IDs to test; ephemeral tree used if empty")
	rpcServer       = flag.String("rpc_server", "", "Server address:port")
	adminServer     = flag.String("admin_server", "", "Address of the gRPC Trillian Admin Server (host:port)")
	metricsEndpoint = flag.String("metrics_endpoint", "", "Endpoint for serving metrics; if left empty, metrics will not be exposed")
	outLog          = flag.String("log_to", "", "File to record operations in")
	seed            = flag.Int64("seed", -1, "Seed for random number generation")
	operations      = flag.Uint64("operations", ^uint64(0), "Number of operations to perform")
	minLeaves       = flag.Int("min_leaves", 0, "Minimum count of leaves to affect per-operation")
	maxLeaves       = flag.Int("max_leaves", 10, "Maximum count of leaves to affect per-operation")
	leafSize        = flag.Uint("leaf_size", 100, "Size of leaf values")
	extraSize       = flag.Uint("extra_size", 100, "Size of leaf extra data")
	checkers        = flag.Int("checkers", 0, "Number of checker goroutines to run")
	retryErrors     = flag.Bool("retry_errors", false, "Whether to retry failed operations")
	opDeadline      = flag.Duration("op_deadline", 60*time.Second, "How long to wait for operation success")
	emitInterval    = flag.Duration("emit_interval", 0, "How often to output the Hammer state")
	keepFailedTree  = flag.Bool("keep_failed_tree", false, "Whether to preserve ephemeral trees on failed run")
)
var (
	getLeavesBias    = flag.Int("get_leaves", 20, "Bias for get-leaves operations")
	getLeavesRevBias = flag.Int("get_leaves_rev", 2, "Bias for get-leaves-revision operations")
	setLeavesBias    = flag.Int("set_leaves", 20, "Bias for set-leaves operations")
	getSMRBias       = flag.Int("get_smr", 10, "Bias for get-smr operations")
	getSMRRevBias    = flag.Int("get_smr_rev", 2, "Bias for get-smr-revision operations")
	invalidChance    = flag.Int("invalid_chance", 10, "Chance of generating an invalid operation, as the N in 1-in-N (0 for never)")
)

func hammerTime() {
	// Purely for fun, so no error checking
	fmt.Print("\n\nIf they'd let me have my way I could have flayed him into shape.\n")
	for i := 0; i < 8; i++ {
		time.Sleep(100 * time.Millisecond)
		fmt.Print(".")
	}
	fmt.Print("\n\n")
	mc := "H4sIAAAAAAAA/5yXMa70KgyF+1kFBR3WaTNCQmloKAK9F8PanzAkgQxJ7vute6UJCR8H24D5qBdD+QfQPT7aO3CCeKL+C7DovWX+E1CNbngCzuTMgXfIEUgg+3cjol/m7ZR/Bm9dv+cbYDKD/wkEHKcjhajN/E/A4Je1WOqAyJsCUhyAF+IUCGeMMcyaWdseqLZt27JJSeMEFtfjCgRAh0VjTKRb26gM9z06lK4/wFMGfJEXRs8Pn+StAI07X8CeGgXYvYJf5WuDW2Cy8oU7iSAcxM/YCUiQ8cFhDkT0CS4C3rj9XQHuxAI8/W45WRTHa+/1ihnQM0df4oNk4g60MusdaLHbl2HXKD8jMzN+rTRT/bkEp1orAdbWoT9K5T2RAIba2rDQzOzRx0vByzD7M5tOuW2B+Sjk3odbPugxnb1rAjAPLWy68cjVSX9g7N0mKiK/ZzRG3AWIE0jTLRg7QzfFBfcdhh7WHJwRYgFOfH+Yrhishbc+fZnXXIHrOOhVaRGZCnaXOtuISuK5kEtYPvQCbKFgjlBPQOQlWwEGegHWaET1AnSLhOVDywvw62PkFKM3j1OG0VmA0PbJ1TbGGL38Ra/uviIJW1UIdrdpo1QFZYjOGD1uHU6L3oFhemyXxI4C8fXZC7JfbeidZbWrQcGqabpSQFXUsZ5jvIjsgDLjClRgNwXKbDtJqC3e65lC0uxqYiuwnqyVUIPhR9/7XeSPGXZ1MX9kG/p5X+X57dLcph3jNResbjMu+yEC5yFwzf8xlhFHdyC1V3potWEXKEAkbY9dUyE37/vpEbD7ts8gFF4VKGcKKC3HtoomIW6d5oHY9J/hR06HwAoEpUDS9fC7P3fu9t/VSz52wVHIOjmX6ThTRFfSuVQBdp/ufeEgtUX7TB7W1PFa5QDY3Y/eRx+/9+VctZqRJQ+xsj4c2Nc2lnXdTkCy1p6BZWCUxLZLcj3vqG0AYg6SJ62GfgHWBHTMI+9QWDRa5pXuN73fFqdZm85/aqi+ALIuccj2JhKXdmt0qtMdNpe+nAORdczBXje94bisf2S0zLak30MFC7LZpYE5u1aQW5jXoi5bei7aIciQWIdMs6UHWLMwJ5cb7vUWUArcnF2Wkzi4TMfeRtatSzmhQ6k2i+8m15RZ0S59bc7OGc0XC9m5OtWJuDtgzTESaO0/mMBE9qzrw8WnXgysYHc77mP319HH2+juvPMCgVtlfwLe2H8BAAD///XdWNEGEAAA"
	mcData, _ := base64.StdEncoding.DecodeString(mc)
	b := bytes.NewReader(mcData)
	r, _ := gzip.NewReader(b)
	if _, err := io.Copy(os.Stdout, r); err != nil {
		return
	}
	if err := r.Close(); err != nil {
		return
	}
	fmt.Print("\n\nLet me hammer him today?\n\n")
}

func main() {
	flag.Parse()
	defer glog.Flush()

	if *mapIDs == "" {
		glog.Info("No mapIDs provided so using a transient tree")
		*mapIDs = "0"
	}
	if *seed == -1 {
		*seed = time.Now().UTC().UnixNano() & 0xFFFFFFFF
	}
	fmt.Printf("Today's test has been brought to you by the letters M, A, and P and the number %#x\n", *seed)

	bias := hammer.MapBias{
		Bias: map[hammer.MapEntrypointName]int{
			hammer.GetLeavesName:    *getLeavesBias,
			hammer.GetLeavesRevName: *getLeavesRevBias,
			hammer.SetLeavesName:    *setLeavesBias,
			hammer.GetSMRName:       *getSMRBias,
			hammer.GetSMRRevName:    *getSMRRevBias,
		},
		InvalidChance: map[hammer.MapEntrypointName]int{
			hammer.GetLeavesName:    *invalidChance,
			hammer.GetLeavesRevName: *invalidChance,
			hammer.SetLeavesName:    *invalidChance,
			hammer.GetSMRName:       0,
			hammer.GetSMRRevName:    *invalidChance,
		},
	}

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
	} else {
		mf = monitoring.InertMetricFactory{}
	}

	if glog.V(1) {
		hammerTime()
	}

	dialOpts := []grpc.DialOption{grpc.WithInsecure()}
	if *outLog != "" {
		cl, err := hammer.NewRecordingInterceptor(*outLog)
		if err != nil {
			glog.Exitf("failed to build recording interceptor: %v", err)
		}
		dialOpts = append(dialOpts, grpc.WithUnaryInterceptor(cl))
	}

	mIDs := strings.Split(*mapIDs, ",")
	type result struct {
		mapID int64
		err   error
	}
	results := make(chan result, len(mIDs))
	var wg sync.WaitGroup
	for _, m := range mIDs {
		randSrc := rand.NewSource(*seed)
		mapid, err := strconv.ParseInt(m, 10, 64)
		if err != nil || mapid < 0 {
			glog.Exitf("Invalid map ID %q", m)
		}
		c, err := grpc.Dial(*rpcServer, dialOpts...)
		if err != nil {
			glog.Exitf("Failed to create map client conn: %v", err)
		}
		ac, err := grpc.Dial(*adminServer, dialOpts...)
		if err != nil {
			glog.Exitf("Failed to create admin client conn: %v", err)
		}
		cfg := hammer.MapConfig{
			MapID:             mapid,
			Client:            trillian.NewTrillianMapClient(c),
			Admin:             trillian.NewTrillianAdminClient(ac),
			MetricFactory:     mf,
			RandSource:        randSrc,
			EPBias:            bias,
			LeafSize:          *leafSize,
			ExtraSize:         *extraSize,
			MinLeaves:         *minLeaves,
			MaxLeaves:         *maxLeaves,
			Operations:        *operations,
			EmitInterval:      *emitInterval,
			NumCheckers:       *checkers,
			RetryErrors:       *retryErrors,
			OperationDeadline: *opDeadline,
			KeepFailedTree:    *keepFailedTree,
		}
		fmt.Printf("%v\n\n", cfg)
		wg.Add(1)
		go func(cfg hammer.MapConfig) {
			defer wg.Done()
			err := hammer.HitMap(context.Background(), cfg)
			results <- result{mapID: cfg.MapID, err: err}
		}(cfg)
	}
	wg.Wait()

	glog.Infof("Completed tests on all %d maps:", len(mIDs))
	close(results)
	errCount := 0
	for e := range results {
		if e.err != nil {
			errCount++
			glog.Errorf("  %d: failed with %v", e.mapID, e.err)
		}
	}
	if errCount > 0 {
		glog.Exitf("non-zero error count (%d), exiting", errCount)
	}
	glog.Info("  no errors; done")
}
