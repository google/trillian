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

package storage

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/quota/etcd/storagepb"
	"github.com/google/trillian/testonly/integration/etcd"
	"github.com/google/trillian/util/clock"
	"github.com/kylelemons/godebug/pretty"
)

const (
	quotaMaxTokens = int64(quota.MaxTokens)
)

var (
	cfgs = &storagepb.Configs{
		Configs: []*storagepb.Config{
			{
				Name:      "quotas/global/read/config",
				State:     storagepb.Config_DISABLED,
				MaxTokens: 1,
				ReplenishmentStrategy: &storagepb.Config_TimeBased{
					TimeBased: &storagepb.TimeBasedStrategy{
						ReplenishIntervalSeconds: 100,
						TokensToReplenish:        10000,
					},
				},
			},
			{
				Name:      "quotas/global/write/config",
				State:     storagepb.Config_ENABLED,
				MaxTokens: 100,
				ReplenishmentStrategy: &storagepb.Config_SequencingBased{
					SequencingBased: &storagepb.SequencingBasedStrategy{},
				},
			},
			{
				Name:      "quotas/users/llama/read/config",
				State:     storagepb.Config_ENABLED,
				MaxTokens: 1000,
				ReplenishmentStrategy: &storagepb.Config_TimeBased{
					TimeBased: &storagepb.TimeBasedStrategy{
						ReplenishIntervalSeconds: 50,
						TokensToReplenish:        500,
					},
				},
			},
		},
	}
	globalRead  = cfgs.Configs[0]
	globalWrite = cfgs.Configs[1]
	userRead    = cfgs.Configs[2]

	fixedTimeSource = clock.NewFake(time.Now())

	// client is an etcd client.
	// Initialized by TestMain().
	client *clientv3.Client
)

func TestMain(m *testing.M) {
	_, c, cleanup, err := etcd.StartEtcd()
	if err != nil {
		panic(fmt.Sprintf("StartEtcd() returned err = %v", err))
	}
	client = c
	exitCode := m.Run()
	cleanup()
	os.Exit(exitCode)
}

func TestIsNameValid(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "quotas/global/read/config", want: true},
		{name: "quotas/global/write/config", want: true},
		{name: "quotas/trees/12356/read/config", want: true},
		{name: "quotas/users/llama/write/config", want: true},

		{name: "bad/quota/name"},
		{name: "badprefix/quotas/global/read/config"},
		{name: "quotas/global/read/config/badsuffix"},
		{name: "quotas/bad/read/config"},
		{name: "quotas/global/bad/config"},
		{name: "quotas/trees/bad/read/config"},
		{name: "quotas/trees/11111111111111111111/read/config"}, // ID > MaxInt64
	}
	for _, test := range tests {
		if got := IsNameValid(test.name); got != test.want {
			t.Errorf("IsNameValid(%q) = %v, want = %v", test.name, got, test.want)
		}
	}
}

func TestQuotaStorage_UpdateConfigs(t *testing.T) {
	defer setupTimeSource(fixedTimeSource)()

	empty := &storagepb.Configs{}

	cfgs2 := deepCopy(cfgs)
	cfgs2.Configs = cfgs2.Configs[1:]  // Remove global/read
	cfgs2.Configs[0].MaxTokens = 50    // decrease global/write
	cfgs2.Configs[1].MaxTokens = 10000 // increase user/read

	treeWriteName := "quotas/trees/12345/write/config"
	cfgs3 := deepCopy(cfgs)
	cfgs3.Configs = append(cfgs3.Configs, &storagepb.Config{
		Name:      treeWriteName,
		State:     storagepb.Config_ENABLED,
		MaxTokens: 200,
		ReplenishmentStrategy: &storagepb.Config_SequencingBased{
			SequencingBased: &storagepb.SequencingBasedStrategy{},
		},
	})

	// Note: tests are incremental, not isolated. The preceding test will have impact on the
	// next, specially if reset is set to false.
	tests := []struct {
		desc       string
		reset      bool
		wantCfgs   *storagepb.Configs
		wantTokens map[string]int64
	}{
		{
			desc:     "empty",
			reset:    true,
			wantCfgs: empty,
		},
		{
			desc:     "cfgs",
			wantCfgs: cfgs,
			wantTokens: map[string]int64{
				globalRead.Name:  quotaMaxTokens, // disabled
				globalWrite.Name: 100,
				userRead.Name:    1000,
			},
		},
		{
			desc:     "cfgs2",
			wantCfgs: cfgs2,
			wantTokens: map[string]int64{
				globalWrite.Name: 50,   // correctly decreased
				userRead.Name:    1000, // unaltered
			},
		},
		{
			desc:     "cfgs3",
			wantCfgs: cfgs3,
			wantTokens: map[string]int64{
				globalWrite.Name: 50,   // unaltered due to reset = false
				userRead.Name:    1000, // unaltered
				treeWriteName:    200,  // new
			},
		},
		{
			desc:     "cfgs3-pt2",
			reset:    true,
			wantCfgs: cfgs3,
			wantTokens: map[string]int64{
				globalWrite.Name: 100, // correctly reset
				userRead.Name:    1000,
				treeWriteName:    200,
			},
		},
		{
			desc:     "cfgs-pt2",
			wantCfgs: cfgs,
			wantTokens: map[string]int64{
				globalWrite.Name: 100,
				userRead.Name:    1000,
				treeWriteName:    quotaMaxTokens, // deleted / infinite
			},
		},
	}

	ctx := context.Background()
	qs := &QuotaStorage{Client: client}
	for _, test := range tests {
		cfgs, err := qs.UpdateConfigs(ctx, test.reset, updater(test.wantCfgs))
		if err != nil {
			t.Errorf("%v: UpdateConfigs() returned err = %v", test.desc, err)
			continue
		}
		if got, want := cfgs, test.wantCfgs; !proto.Equal(got, want) {
			diff := pretty.Compare(got, want)
			t.Errorf("%v: post-UpdateConfigs() diff (-got +want)\n%v", test.desc, diff)
		}

		stored, err := qs.Configs(ctx)
		if err != nil {
			t.Errorf("%v:Configs() returned err = %v", test.desc, err)
			continue
		}
		if got, want := stored, cfgs; !proto.Equal(got, want) {
			diff := pretty.Compare(got, want)
			t.Errorf("%v: post-Configs() diff (-got +want)\n%v", test.desc, diff)
		}

		if err := peekAndDiff(ctx, qs, test.wantTokens); err != nil {
			t.Errorf("%v: %v", test.desc, err)
		}
	}
}

func TestQuotaStorage_UpdateConfigsErrors(t *testing.T) {
	globalWriteCfgs := &storagepb.Configs{Configs: []*storagepb.Config{globalWrite}}

	emptyName := deepCopy(globalWriteCfgs)
	emptyName.Configs[0].Name = ""

	invalidName1 := deepCopy(globalWriteCfgs)
	invalidName1.Configs[0].Name = "invalid"

	invalidName2 := deepCopy(globalWriteCfgs)
	invalidName2.Configs[0].Name = "quotas/tree/1234/write" // should be "trees", plural

	unknownState := deepCopy(globalWriteCfgs)
	unknownState.Configs[0].State = storagepb.Config_UNKNOWN_CONFIG_STATE

	zeroMaxTokens := deepCopy(globalWriteCfgs)
	zeroMaxTokens.Configs[0].MaxTokens = 0

	invalidMaxTokens := deepCopy(globalWriteCfgs)
	invalidMaxTokens.Configs[0].MaxTokens = -1

	noReplenishmentStrategy := deepCopy(globalWriteCfgs)
	noReplenishmentStrategy.Configs[0].ReplenishmentStrategy = nil

	zeroTimeBasedTokens := deepCopy(globalWriteCfgs)
	zeroTimeBasedTokens.Configs[0].ReplenishmentStrategy = &storagepb.Config_TimeBased{
		TimeBased: &storagepb.TimeBasedStrategy{
			TokensToReplenish:        0,
			ReplenishIntervalSeconds: 10,
		},
	}

	invalidTimeBasedTokens := deepCopy(globalWriteCfgs)
	invalidTimeBasedTokens.Configs[0].ReplenishmentStrategy = &storagepb.Config_TimeBased{
		TimeBased: &storagepb.TimeBasedStrategy{
			TokensToReplenish:        -1,
			ReplenishIntervalSeconds: 10,
		},
	}

	zeroReplenishInterval := deepCopy(globalWriteCfgs)
	zeroReplenishInterval.Configs[0].ReplenishmentStrategy = &storagepb.Config_TimeBased{
		TimeBased: &storagepb.TimeBasedStrategy{
			TokensToReplenish:        1,
			ReplenishIntervalSeconds: 0,
		},
	}

	invalidReplenishInterval := deepCopy(globalWriteCfgs)
	invalidReplenishInterval.Configs[0].ReplenishmentStrategy = &storagepb.Config_TimeBased{
		TimeBased: &storagepb.TimeBasedStrategy{
			TokensToReplenish:        1,
			ReplenishIntervalSeconds: -1,
		},
	}

	duplicateNames := &storagepb.Configs{Configs: []*storagepb.Config{globalRead, globalWrite, globalWrite}}

	sequencingBasedStrategy := &storagepb.Config_SequencingBased{SequencingBased: &storagepb.SequencingBasedStrategy{}}
	sequencingBasedUserQuota := &storagepb.Configs{
		Configs: []*storagepb.Config{
			{
				Name:                  userRead.Name,
				State:                 userRead.State,
				MaxTokens:             userRead.MaxTokens,
				ReplenishmentStrategy: sequencingBasedStrategy,
			},
		},
	}

	sequencingBasedReadQuota1 := deepCopy(globalWriteCfgs)
	sequencingBasedReadQuota1.Configs[0].Name = globalRead.Name
	sequencingBasedReadQuota1.Configs[0].ReplenishmentStrategy = sequencingBasedStrategy

	sequencingBasedReadQuota2 := deepCopy(globalWriteCfgs)
	sequencingBasedReadQuota2.Configs[0].Name = "quotas/trees/1234/read/config"
	sequencingBasedReadQuota2.Configs[0].ReplenishmentStrategy = sequencingBasedStrategy

	tests := []struct {
		desc    string
		update  func(*storagepb.Configs)
		wantErr string
	}{
		{desc: "nil", wantErr: "function required"},
		{
			desc:    "emptyName",
			update:  updater(emptyName),
			wantErr: "name is required",
		},
		{
			desc:    "invalidName1",
			update:  updater(invalidName1),
			wantErr: "name malformed",
		},
		{
			desc:    "invalidName2",
			update:  updater(invalidName2),
			wantErr: "name malformed",
		},
		{
			desc:    "unknownState",
			update:  updater(unknownState),
			wantErr: "state invalid",
		},
		{
			desc:    "zeroMaxTokens",
			update:  updater(zeroMaxTokens),
			wantErr: "max tokens must be > 0",
		},
		{
			desc:    "invalidMaxTokens",
			update:  updater(invalidMaxTokens),
			wantErr: "max tokens must be > 0",
		},
		{
			desc:    "noReplenishmentStrategy",
			update:  updater(noReplenishmentStrategy),
			wantErr: "unsupported replenishment strategy",
		},
		{
			desc:    "zeroTimeBasedTokens",
			update:  updater(zeroTimeBasedTokens),
			wantErr: "time based tokens must be > 0",
		},
		{
			desc:    "invalidTimeBasedTokens",
			update:  updater(invalidTimeBasedTokens),
			wantErr: "time based tokens must be > 0",
		},
		{
			desc:    "zeroReplenishInterval",
			update:  updater(zeroReplenishInterval),
			wantErr: "replenish interval must be > 0",
		},
		{
			desc:    "invalidReplenishInterval",
			update:  updater(invalidReplenishInterval),
			wantErr: "replenish interval must be > 0",
		},
		{
			desc:    "duplicateNames",
			update:  updater(duplicateNames),
			wantErr: "duplicate config name",
		},
		{
			desc:    "sequencingBasedUserQuota",
			update:  updater(sequencingBasedUserQuota),
			wantErr: "cannot use sequencing-based replenishment",
		},
		{
			desc:    "sequencingBasedReadQuota1",
			update:  updater(sequencingBasedReadQuota1),
			wantErr: "cannot use sequencing-based replenishment",
		},
		{
			desc:    "sequencingBasedReadQuota2",
			update:  updater(sequencingBasedReadQuota2),
			wantErr: "cannot use sequencing-based replenishment",
		},
	}

	ctx := context.Background()
	qs := &QuotaStorage{Client: client}

	want := &storagepb.Configs{} // default cfgs is empty
	if _, err := qs.UpdateConfigs(ctx, true /* reset */, updater(want)); err != nil {
		t.Fatalf("UpdateConfigs() returned err = %v", err)
	}

	for _, test := range tests {
		if _, err := qs.UpdateConfigs(ctx, false /* reset */, test.update); !strings.Contains(err.Error(), test.wantErr) {
			// Fatal because the config has been changed, which will break all following tests.
			t.Fatalf("%v: UpdateConfigs() returned err = %v, want substring %q", test.desc, err, test.wantErr)
		}

		stored, err := qs.Configs(ctx)
		if err != nil {
			t.Errorf("%v:Configs() returned err = %v", test.desc, err)
			continue
		}
		if got := stored; !proto.Equal(got, want) {
			diff := pretty.Compare(got, want)
			t.Fatalf("%v: post-Configs() diff (-got +want)\n%v", test.desc, diff)
		}
	}
}

func TestQuotaStorage_DeletedConfig(t *testing.T) {
	defer setupTimeSource(fixedTimeSource)()

	ctx := context.Background()
	qs := &QuotaStorage{Client: client}

	cfgs := deepCopy(cfgs)
	cfgs.Configs = cfgs.Configs[1:2] // Only global/write
	globalWrite := cfgs.Configs[0]
	if _, err := qs.UpdateConfigs(ctx, true /* reset */, updater(cfgs)); err != nil {
		t.Fatalf("UpdateConfigs() returned err = %v", err)
	}

	// Normal quota behavior
	names := []string{globalWrite.Name}
	_ = qs.Get(ctx, names, 100)
	if err := peekAndDiff(ctx, qs, map[string]int64{globalWrite.Name: globalWrite.MaxTokens - 100}); err != nil {
		t.Fatalf("peekAndDiff returned err = %v", err)
	}

	// Deleted: considered infinite
	cfgs = &storagepb.Configs{}
	if _, err := qs.UpdateConfigs(ctx, false /* reset */, updater(cfgs)); err != nil {
		t.Fatalf("UpdateConfigs() returned err = %v", err)
	}
	if err := peekAndDiff(ctx, qs, map[string]int64{globalWrite.Name: quotaMaxTokens}); err != nil {
		t.Fatalf("peekAndDiff returned err = %v", err)
	}

	// Restored: must behave as new (ie, doesn't "revive" the old token count)
	cfgs = &storagepb.Configs{Configs: []*storagepb.Config{globalWrite}}
	if _, err := qs.UpdateConfigs(ctx, false /* reset */, updater(cfgs)); err != nil {
		t.Fatalf("UpdateConfigs() returned err = %v", err)
	}
	if err := peekAndDiff(ctx, qs, map[string]int64{globalWrite.Name: globalWrite.MaxTokens}); err != nil {
		t.Fatalf("peekAndDiff returned err = %v", err)
	}
}

func TestQuotaStorage_DisabledConfig(t *testing.T) {
	defer setupTimeSource(fixedTimeSource)()

	ctx := context.Background()
	qs := &QuotaStorage{Client: client}

	cfgs := deepCopy(cfgs)
	cfgs.Configs = cfgs.Configs[0:1] // Only global/read
	globalRead := cfgs.Configs[0]
	globalRead.State = storagepb.Config_ENABLED
	globalRead.MaxTokens = 1000
	if _, err := qs.UpdateConfigs(ctx, true /* reset */, updater(cfgs)); err != nil {
		t.Fatalf("UpdateConfigs() returned err = %v", err)
	}

	// Normal quota behavior
	names := []string{globalRead.Name}
	_ = qs.Get(ctx, names, 100)
	if err := peekAndDiff(ctx, qs, map[string]int64{globalRead.Name: globalRead.MaxTokens - 100}); err != nil {
		t.Fatalf("peekAndDiff returned err = %v", err)
	}

	// Disabled: cfg still exists, but is considered infinite
	globalRead.State = storagepb.Config_DISABLED
	if _, err := qs.UpdateConfigs(ctx, false /* reset */, updater(cfgs)); err != nil {
		t.Fatalf("UpdateConfigs() returned err = %v", err)
	}
	if err := peekAndDiff(ctx, qs, map[string]int64{globalRead.Name: quotaMaxTokens}); err != nil {
		t.Fatalf("peekAndDiff returned err = %v", err)
	}

	// Enabled: tokens restored to ceiling, even though reset = false
	globalRead.State = storagepb.Config_ENABLED
	if _, err := qs.UpdateConfigs(ctx, false /* reset */, updater(cfgs)); err != nil {
		t.Fatalf("UpdateConfigs() returned err = %v", err)
	}
	if err := peekAndDiff(ctx, qs, map[string]int64{globalRead.Name: globalRead.MaxTokens}); err != nil {
		t.Fatalf("peekAndDiff returned err = %v", err)
	}
}

func TestQuotaStorage_Get(t *testing.T) {
	fakeTime := clock.NewFake(time.Now())
	setupTimeSource(fakeTime)

	tests := []struct {
		desc                      string
		names                     []string
		tokens                    int64
		nowIncrement              time.Duration
		initialTokens, wantTokens map[string]int64
	}{
		{
			desc:   "success",
			names:  []string{globalRead.Name, globalWrite.Name, userRead.Name},
			tokens: 5,
			wantTokens: map[string]int64{
				globalRead.Name:  quotaMaxTokens, // disabled
				globalWrite.Name: globalWrite.MaxTokens - 5,
				userRead.Name:    userRead.MaxTokens - 5,
			},
		},
		{
			desc:   "globalOnly",
			names:  []string{globalWrite.Name},
			tokens: 7,
			wantTokens: map[string]int64{
				globalWrite.Name: globalWrite.MaxTokens - 7,
				userRead.Name:    userRead.MaxTokens,
			},
		},
		{
			desc:   "userOnly",
			names:  []string{userRead.Name},
			tokens: 7,
			wantTokens: map[string]int64{
				globalWrite.Name: globalWrite.MaxTokens,
				userRead.Name:    userRead.MaxTokens - 7,
			},
		},
		{
			desc:   "zeroTokens",
			names:  []string{globalWrite.Name, userRead.Name},
			tokens: 0,
			wantTokens: map[string]int64{
				globalWrite.Name: globalWrite.MaxTokens,
				userRead.Name:    userRead.MaxTokens,
			},
		},
		{
			desc:         "successWithReplenishment",
			names:        []string{globalWrite.Name, userRead.Name},
			tokens:       5,
			nowIncrement: time.Duration(userRead.GetTimeBased().ReplenishIntervalSeconds) * time.Second,
			wantTokens: map[string]int64{
				globalWrite.Name: globalWrite.MaxTokens - 5,
				userRead.Name:    userRead.MaxTokens - 5, // Replenished then deduced
			},
		},
		{
			desc:         "successDueToReplenishment",
			names:        []string{globalWrite.Name, userRead.Name},
			tokens:       1,
			nowIncrement: time.Duration(userRead.GetTimeBased().ReplenishIntervalSeconds) * time.Second,
			initialTokens: map[string]int64{
				userRead.Name: 0,
			},
			wantTokens: map[string]int64{
				globalWrite.Name: globalWrite.MaxTokens - 1,
				userRead.Name:    userRead.GetTimeBased().TokensToReplenish - 1,
			},
		},
	}

	ctx := context.Background()
	qs := &QuotaStorage{Client: client}
	for _, test := range tests {
		if err := setupTokens(ctx, qs, cfgs, test.initialTokens); err != nil {
			t.Errorf("%v: setupTokens() returned err = %v", test.desc, err)
			continue
		}

		fakeTime.Set(fakeTime.Now().Add(test.nowIncrement))
		if err := qs.Get(ctx, test.names, test.tokens); err != nil {
			t.Errorf("%v: Get() returned err = %v", test.desc, err)
			continue
		}

		if err := peekAndDiff(ctx, qs, test.wantTokens); err != nil {
			t.Errorf("%v: %v", test.desc, err)
		}
	}
}

func TestQuotaStorage_GetErrors(t *testing.T) {
	tests := []struct {
		desc   string
		names  []string
		tokens int64
	}{
		{
			desc:   "invalidTokens",
			names:  []string{globalWrite.Name, userRead.Name},
			tokens: -1,
		},
		{
			desc:   "insufficientTokens",
			names:  []string{globalWrite.Name, userRead.Name},
			tokens: globalWrite.MaxTokens + 10,
		},
	}

	ctx := context.Background()
	qs := &QuotaStorage{Client: client}
	if _, err := qs.UpdateConfigs(ctx, true /* reset */, updater(cfgs)); err != nil {
		t.Fatalf("UpdateConfigs() returned err = %v", err)
	}

	for _, test := range tests {
		if err := qs.Get(ctx, test.names, test.tokens); err == nil {
			t.Errorf("%v: Get() returned err = nil, want non-nil", test.desc)
		}
	}
}

func TestQuotaStorage_Peek(t *testing.T) {
	fakeTime := clock.NewFake(time.Now())
	defer setupTimeSource(fakeTime)()

	tests := []struct {
		desc                      string
		names                     []string
		nowIncrement              time.Duration
		initialTokens, wantTokens map[string]int64
	}{
		{
			desc:  "success",
			names: []string{globalRead.Name, globalWrite.Name, userRead.Name, "quotas/users/llama/write/config"},
			wantTokens: map[string]int64{
				globalRead.Name:                   quotaMaxTokens, // disabled
				globalWrite.Name:                  globalWrite.MaxTokens,
				userRead.Name:                     userRead.MaxTokens,
				"quotas/users/llama/write/config": quotaMaxTokens, // unknown
			},
		},
		{
			desc:         "timeBasedReplenish",
			names:        []string{globalWrite.Name, userRead.Name},
			nowIncrement: time.Duration(userRead.GetTimeBased().ReplenishIntervalSeconds) * time.Second,
			initialTokens: map[string]int64{
				globalWrite.Name: 10,
				userRead.Name:    10,
			},
			wantTokens: map[string]int64{
				globalWrite.Name: 10,
				userRead.Name:    10 + userRead.GetTimeBased().TokensToReplenish,
			},
		},
	}

	ctx := context.Background()
	qs := &QuotaStorage{Client: client}
	for _, test := range tests {
		if err := setupTokens(ctx, qs, cfgs, test.initialTokens); err != nil {
			t.Errorf("%v: setupTokens() returned err = %v", test.desc, err)
			continue
		}

		fakeTime.Set(fakeTime.Now().Add(test.nowIncrement))
		if err := peekAndDiff(ctx, qs, test.wantTokens); err != nil {
			t.Errorf("%v: %v", test.desc, err)
		}
	}
}

func TestQuotaStorage_Put(t *testing.T) {
	fakeTime := clock.NewFake(time.Now())
	defer setupTimeSource(fakeTime)()

	tests := []struct {
		desc                      string
		names                     []string
		tokens                    int64
		nowIncrement              time.Duration
		initialTokens, wantTokens map[string]int64
	}{
		{
			desc:   "zero",
			names:  []string{globalWrite.Name, userRead.Name},
			tokens: 0,
			wantTokens: map[string]int64{
				globalWrite.Name: globalWrite.MaxTokens,
				userRead.Name:    userRead.MaxTokens,
			},
		},
		{
			desc:   "success",
			names:  []string{globalRead.Name, globalWrite.Name, userRead.Name},
			tokens: 10,
			initialTokens: map[string]int64{
				globalWrite.Name: 10,
				userRead.Name:    10,
			},
			wantTokens: map[string]int64{
				globalRead.Name:  quotaMaxTokens, // disabled
				globalWrite.Name: 20,
				userRead.Name:    10, // Time-based quotas don't change on Put()
			},
		},
		{
			desc:   "fullQuota",
			names:  []string{globalWrite.Name, userRead.Name},
			tokens: 10,
			wantTokens: map[string]int64{
				globalWrite.Name: globalWrite.MaxTokens,
				userRead.Name:    userRead.MaxTokens,
			},
		},
		{
			desc:         "replenishToFull",
			names:        []string{userRead.Name},
			tokens:       0,
			nowIncrement: time.Duration(userRead.GetTimeBased().ReplenishIntervalSeconds) * time.Second,
			initialTokens: map[string]int64{
				userRead.Name: userRead.MaxTokens - 1,
			},
			wantTokens: map[string]int64{
				userRead.Name: userRead.MaxTokens,
			},
		},
		{
			desc:         "partialReplenish",
			names:        []string{userRead.Name},
			tokens:       100,
			nowIncrement: time.Duration(userRead.GetTimeBased().ReplenishIntervalSeconds) * time.Second,
			initialTokens: map[string]int64{
				userRead.Name: 0,
			},
			wantTokens: map[string]int64{
				userRead.Name: userRead.GetTimeBased().TokensToReplenish,
			},
		},
	}

	ctx := context.Background()
	qs := &QuotaStorage{Client: client}
	for _, test := range tests {
		if err := setupTokens(ctx, qs, cfgs, test.initialTokens); err != nil {
			t.Errorf("%v: setupTokens() returned err = %v", test.desc, err)
			continue
		}

		if err := qs.Put(ctx, test.names, test.tokens); err != nil {
			t.Errorf("%v: Put() returned err = %v", test.desc, err)
		}

		fakeTime.Set(fakeTime.Now().Add(test.nowIncrement))
		if err := peekAndDiff(ctx, qs, test.wantTokens); err != nil {
			t.Errorf("%v: %v", test.desc, err)
		}
	}
}

func TestQuotaStorage_PutErrors(t *testing.T) {
	tests := []struct {
		desc   string
		names  []string
		tokens int64
	}{
		{desc: "invalidTokens", names: []string{globalWrite.Name}, tokens: -1},
	}

	ctx := context.Background()
	qs := &QuotaStorage{Client: client}
	if _, err := qs.UpdateConfigs(ctx, true /* reset */, updater(cfgs)); err != nil {
		t.Fatalf("UpdateConfigs() returned err = %v", err)
	}

	for _, test := range tests {
		if err := qs.Put(ctx, test.names, test.tokens); err == nil {
			t.Errorf("%v: Put() returned err = nil, want non-nil", test.desc)
		}
	}
}

func TestQuotaStorage_Reset(t *testing.T) {
	defer setupTimeSource(fixedTimeSource)()

	tests := []struct {
		desc                      string
		names                     []string
		initialTokens, wantTokens map[string]int64
	}{
		{
			desc:  "success",
			names: []string{globalRead.Name, globalWrite.Name, userRead.Name},
			initialTokens: map[string]int64{
				globalWrite.Name: 10,
				userRead.Name:    10,
			},
			wantTokens: map[string]int64{
				globalRead.Name:  quotaMaxTokens, // disabled
				globalWrite.Name: globalWrite.MaxTokens,
				userRead.Name:    userRead.MaxTokens,
			},
		},
		{
			desc:  "globalWrite",
			names: []string{globalWrite.Name},
			initialTokens: map[string]int64{
				globalWrite.Name: 10,
			},
			wantTokens: map[string]int64{
				globalWrite.Name: globalWrite.MaxTokens,
			},
		},
		{
			desc:  "userRead",
			names: []string{userRead.Name},
			initialTokens: map[string]int64{
				userRead.Name: 10,
			},
			wantTokens: map[string]int64{
				userRead.Name: userRead.MaxTokens,
			},
		},
		{
			desc:  "fullQuotas",
			names: []string{globalWrite.Name, userRead.Name},
			wantTokens: map[string]int64{
				globalWrite.Name: globalWrite.MaxTokens,
				userRead.Name:    userRead.MaxTokens,
			},
		},
		{
			desc:  "unknownQuota",
			names: []string{"quotas/users/llama/write/config"},
			wantTokens: map[string]int64{
				"quotas/users/llama/write/config": quotaMaxTokens,
			},
		},
	}

	ctx := context.Background()
	qs := &QuotaStorage{Client: client}
	if _, err := qs.UpdateConfigs(ctx, true /* reset */, updater(cfgs)); err != nil {
		t.Fatalf("UpdateConfigs() returned err = %v", err)
	}

	for _, test := range tests {
		if err := setupTokens(ctx, qs, cfgs, test.initialTokens); err != nil {
			t.Errorf("%v: setupTokens() returned err = %v", test.desc, err)
			continue
		}

		if err := qs.Reset(ctx, test.names); err != nil {
			t.Errorf("%v: Reset() returned err = %v", test.desc, err)
		}

		if err := peekAndDiff(ctx, qs, test.wantTokens); err != nil {
			t.Errorf("%v: %v", test.desc, err)
		}
	}
}

func TestQuotaStorage_ValidateNames(t *testing.T) {
	fns := []struct {
		name string
		run  func(context.Context, *QuotaStorage, []string) error
	}{
		{
			name: "Get",
			run: func(ctx context.Context, qs *QuotaStorage, names []string) error {
				return qs.Get(ctx, names, 0)
			},
		},
		{
			name: "Peek",
			run: func(ctx context.Context, qs *QuotaStorage, names []string) error {
				_, err := qs.Peek(ctx, names)
				return err
			},
		},
		{
			name: "Put",
			run: func(ctx context.Context, qs *QuotaStorage, names []string) error {
				return qs.Put(ctx, names, 0)
			},
		},
		{
			name: "Reset",
			run: func(ctx context.Context, qs *QuotaStorage, names []string) error {
				return qs.Reset(ctx, names)
			},
		},
	}

	tests := []struct {
		names []string
	}{
		{names: []string{"bad/quota/name"}},
		{names: []string{"quotas/bad/read/configs"}},
		{names: []string{"quotas/global/read"}}, // missing "/configs"
		{names: []string{"quotas/trees/1234/write"}},
		{names: []string{"quotas/users/llama/write"}},
		{names: []string{"quotas/tree/1234/read/configs"}},  // should be "trees"
		{names: []string{"quotas/user/llama/read/configs"}}, // should be "users"
		{names: []string{globalWrite.Name, "bad"}},
	}

	ctx := context.Background()
	qs := &QuotaStorage{Client: client}
	for _, test := range tests {
		for _, fn := range fns {
			if err := fn.run(ctx, qs, test.names); err == nil {
				t.Errorf("%v(%v) returned err = nil, want non-nil", fn.name, test.names)
			}
		}
	}
}

func peekAndDiff(ctx context.Context, qs *QuotaStorage, want map[string]int64) error {
	got, err := qs.Peek(ctx, keys(want))
	if err != nil {
		return err
	}
	if diff := pretty.Compare(got, want); diff != "" {
		return fmt.Errorf("post-Peek() diff (-got +want):\n%v", diff)
	}
	return nil
}

// setupTimeSource prepares timeSource for tests.
// A cleanup function that restores timeSource to its initial value is returned and should be
// defer-called.
func setupTimeSource(ts clock.TimeSource) func() {
	prevTimeSource := timeSource
	timeSource = ts
	return func() { timeSource = prevTimeSource }
}

// setupTokens resets cfgs and gets tokens from each quota in order to make them match
// initialTokens.
func setupTokens(ctx context.Context, qs *QuotaStorage, cfgs *storagepb.Configs, initialTokens map[string]int64) error {
	if _, err := qs.UpdateConfigs(ctx, true /* reset */, updater(cfgs)); err != nil {
		return fmt.Errorf("UpdateConfigs() returned err = %w", err)
	}
	for name, wantTokens := range initialTokens {
		names := []string{name}
		tokens, err := qs.Peek(ctx, names)
		if err != nil {
			return fmt.Errorf("qs.Peek()=%v,%v, want: err=nil", tokens, err)
		}
		mod := tokens[name] - wantTokens
		if err := qs.Get(ctx, names, mod); err != nil {
			return fmt.Errorf("qs.Get()=%v, want: nil", err)
		}
		if err := peekAndDiff(ctx, qs, map[string]int64{name: wantTokens}); err != nil {
			return err
		}
	}
	return nil
}

func deepCopy(c1 *storagepb.Configs) *storagepb.Configs {
	return proto.Clone(c1).(*storagepb.Configs)
}

func keys(m map[string]int64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func updater(cfgs *storagepb.Configs) func(*storagepb.Configs) {
	return func(c *storagepb.Configs) {
		(*c).Reset()
		proto.Merge(c, cfgs)
	}
}
