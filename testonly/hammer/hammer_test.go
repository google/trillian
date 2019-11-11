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

package hammer

import (
	"context"
	"flag"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/google/trillian/monitoring"
	"github.com/google/trillian/storage/testdb"
	"github.com/google/trillian/testonly/integration"

	_ "github.com/google/trillian/merkle/coniks"    // register CONIKS_SHA512_256
	_ "github.com/google/trillian/merkle/maphasher" // register TEST_MAP_HASHER
)

var (
	operations = flag.Uint64("operations", 20, "Number of operations to perform")
	singleTX   = flag.Bool("single_transaction", false, "Experimental: whether to use a single transaction when updating the map")
)

func TestRetryExposesDeadlineError(t *testing.T) {
	testdb.SkipIfNoMySQL(t)
	ctx := context.Background()
	env, err := integration.NewMapEnv(ctx, *singleTX)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()

	bias := MapBias{
		Bias: map[MapEntrypointName]int{
			GetLeavesName:    10,
			GetLeavesRevName: 10,
			SetLeavesName:    10,
			GetSMRName:       10,
			GetSMRRevName:    10,
		},
		InvalidChance: map[MapEntrypointName]int{
			GetLeavesName:    10,
			GetLeavesRevName: 10,
			SetLeavesName:    10,
			GetSMRName:       0,
			GetSMRRevName:    10,
		},
	}

	seed := int64(99)
	cfg := MapConfig{
		MapID:         0, // ephemeral tree
		Client:        env.Map,
		Write:         env.Write,
		Admin:         env.Admin,
		MetricFactory: monitoring.InertMetricFactory{},
		RandSource:    rand.NewSource(seed),
		EPBias:        bias,
		LeafSize:      1000,
		ExtraSize:     100,
		// TODO(mhutchinson): Increase these when #1845 is fixed.
		MinLeavesR:  90,
		MaxLeavesR:  100,
		MinLeavesW:  90,
		MaxLeavesW:  100,
		Operations:  *operations,
		NumCheckers: 1,
	}

	ctx, cancel := context.WithTimeout(ctx, time.Nanosecond)
	defer cancel()
	if err, wantErr := HitMap(ctx, cfg), context.DeadlineExceeded; !strings.Contains(err.Error(), wantErr.Error()) {
		t.Fatalf("Got err %q, expected %q", err, wantErr)
	}
}

func TestInProcessMapHammer(t *testing.T) {
	testdb.SkipIfNoMySQL(t)
	ctx := context.Background()
	env, err := integration.NewMapEnv(ctx, *singleTX)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()

	bias := MapBias{
		Bias: map[MapEntrypointName]int{
			GetLeavesName:    10,
			GetLeavesRevName: 10,
			SetLeavesName:    10,
			GetSMRName:       10,
			GetSMRRevName:    10,
		},
		InvalidChance: map[MapEntrypointName]int{
			GetLeavesName:    10,
			GetLeavesRevName: 10,
			SetLeavesName:    10,
			GetSMRName:       0,
			GetSMRRevName:    10,
		},
	}

	seed := int64(99)
	cfg := MapConfig{
		MapID:         0, // ephemeral tree
		Client:        env.Map,
		Write:         env.Write,
		Admin:         env.Admin,
		MetricFactory: monitoring.InertMetricFactory{},
		RandSource:    rand.NewSource(seed),
		EPBias:        bias,
		LeafSize:      1000,
		ExtraSize:     100,
		// TODO(mhutchinson): Increase these when #1845 is fixed.
		MinLeavesR:  90,
		MaxLeavesR:  100,
		MinLeavesW:  90,
		MaxLeavesW:  100,
		Operations:  *operations,
		NumCheckers: 1,
	}
	if err := HitMap(ctx, cfg); err != nil {
		t.Fatalf("hammer failure: %v", err)
	}
}
