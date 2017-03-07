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
	"context"
	"flag"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/google/trillian/examples/ct"
	"github.com/google/trillian/testonly/integration"
)

var httpServersFlag = flag.String("ct_http_servers", "localhost:8092", "Comma-separated list of (assumed interchangeable) servers, each as address:port")
var testDir = flag.String("testdata_dir", "testdata", "Name of directory with test data")
var seed = flag.Int64("seed", -1, "Seed for random number generation")
var logConfigFlag = flag.String("log_config", "", "File holding log config in JSON")
var mmdFlag = flag.Duration("mmd", 30*time.Second, "MMD for tested logs")
var skipStats = flag.Bool("skip_stats", false, "Skip checks of expected log statistics")

func TestLiveCTIntegration(t *testing.T) {
	flag.Parse()
	if *logConfigFlag == "" {
		t.Skip("Integration test skipped as no log config provided")
	}
	if *seed == -1 {
		*seed = time.Now().UTC().UnixNano() & 0xFFFFFFFF
	}
	fmt.Printf("Today's test has been brought to you by the letters C and T and the number %#x\n", *seed)
	rand.Seed(*seed)

	cfgs, err := ct.LogConfigFromFile(*logConfigFlag)
	if err != nil {
		t.Fatalf("Failed to read log config: %v", err)
	}

	for _, cfg := range cfgs {
		cfg := cfg // capture config
		t.Run(cfg.Prefix, func(t *testing.T) {
			t.Parallel()
			var stats *wantStats
			if !*skipStats {
				stats = newWantStats(cfg.LogID)
			}
			if err := RunCTIntegrationForLog(cfg, *httpServersFlag, *testDir, *mmdFlag, stats); err != nil {
				t.Errorf("%s: failed: %v", cfg.Prefix, err)
			}
		})
	}
}

const (
	rootsPEMFile    = "../testdata/fake-ca.cert"
	pubKeyPEMFile   = "../testdata/ct-http-server.pubkey.pem"
	privKeyPEMFile  = "../testdata/ct-http-server.privkey.pem"
	privKeyPassword = "dirk"
)

func TestInProcessCTIntegration(t *testing.T) {
	ctx := context.Background()
	cfgs := []*ct.LogConfig{
		{
			Prefix:          "athos",
			RootsPEMFile:    rootsPEMFile,
			PubKeyPEMFile:   pubKeyPEMFile,
			PrivKeyPEMFile:  privKeyPEMFile,
			PrivKeyPassword: privKeyPassword,
		},
		{
			Prefix:          "porthos",
			RootsPEMFile:    rootsPEMFile,
			PubKeyPEMFile:   pubKeyPEMFile,
			PrivKeyPEMFile:  privKeyPEMFile,
			PrivKeyPassword: privKeyPassword,
		},
		{
			Prefix:          "aramis",
			RootsPEMFile:    rootsPEMFile,
			PubKeyPEMFile:   pubKeyPEMFile,
			PrivKeyPEMFile:  privKeyPEMFile,
			PrivKeyPassword: privKeyPassword,
		},
	}

	env, err := integration.NewCTLogEnv(ctx, cfgs, 2, "TestInProcessCTIntegration")
	if err != nil {
		t.Fatalf("Failed to launch test environment: %v", err)
	}
	defer env.Close()

	mmd := 120 * time.Second
	// Run a container for the parallel sub-tests, so that we wait until they
	// all complete before terminating the test environment.
	t.Run("container", func(t *testing.T) {
		for _, cfg := range cfgs {
			cfg := cfg // capture config
			t.Run(cfg.Prefix, func(t *testing.T) {
				t.Parallel()
				stats := newWantStats(cfg.LogID)
				if err := RunCTIntegrationForLog(*cfg, env.CTAddr, "../testdata", mmd, stats); err != nil {
					t.Errorf("%s: failed: %v", cfg.Prefix, err)
				}
			})
		}
	})
}
