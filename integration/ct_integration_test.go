// Copyright 2016 Google Inc. All Rights Reserved.
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

package integration

import (
	"flag"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	ctfe "github.com/google/trillian/examples/ct"
)

var httpServersFlag = flag.String("ct_http_servers", "localhost:8092", "Comma-separated list of (assumed interchangeable) servers, each as address:port")
var testDir = flag.String("testdata", "testdata", "Name of directory with test data")
var seed = flag.Int64("seed", -1, "Seed for random number generation")
var logConfigFlag = flag.String("log_config", "", "File holding log config in JSON")
var skipStats = flag.Bool("skip_stats", false, "Skip checks of expected log statistics")

func TestCTIntegration(t *testing.T) {
	flag.Parse()
	if *logConfigFlag == "" {
		t.Skip("Integration test skipped as no log config provided")
	}
	if *seed == -1 {
		*seed = time.Now().UTC().UnixNano() & 0xFFFFFFFF
	}
	fmt.Printf("Today's test has been brought to you by the letters C and T and the number %#x\n", *seed)
	rand.Seed(*seed)

	cfg, err := ctfe.LogConfigFromFile(*logConfigFlag)
	if err != nil {
		t.Fatalf("Failed to read log config: %v", err)
	}

	type result struct {
		prefix string
		err    error
	}
	results := make(chan result, len(cfg))
	var wg sync.WaitGroup
	for _, c := range cfg {
		wg.Add(1)
		go func(c ctfe.LogConfig) {
			defer wg.Done()
			var stats *wantStats
			if !*skipStats {
				stats = newWantStats(c.LogID)
			}
			err := RunCTIntegrationForLog(c, *httpServersFlag, *testDir, stats)
			results <- result{prefix: c.Prefix, err: err}
		}(c)
	}
	wg.Wait()
	close(results)
	for e := range results {
		if e.err != nil {
			t.Errorf("%s: %v", e.prefix, e.err)
		}
	}
}
