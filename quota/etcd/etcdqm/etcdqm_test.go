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

package etcdqm

import (
	"context"
	"fmt"
	"math"
	"os"
	"testing"

	"github.com/coreos/etcd/clientv3"
	"github.com/golang/protobuf/proto"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/quota/etcd/storage"
	"github.com/google/trillian/quota/etcd/storagepb"
	"github.com/google/trillian/testonly/integration/etcd"
	"github.com/kylelemons/godebug/pretty"
)

const (
	treeID = 12345
	userID = "llama"
)

var (
	cfgs = &storagepb.Configs{
		Configs: []*storagepb.Config{
			{
				Name:      "quotas/global/write/config",
				State:     storagepb.Config_ENABLED,
				MaxTokens: 100,
				ReplenishmentStrategy: &storagepb.Config_SequencingBased{
					SequencingBased: &storagepb.SequencingBasedStrategy{},
				},
			},
			{
				Name:      fmt.Sprintf("quotas/trees/%v/write/config", treeID),
				State:     storagepb.Config_ENABLED,
				MaxTokens: 200,
				ReplenishmentStrategy: &storagepb.Config_SequencingBased{
					SequencingBased: &storagepb.SequencingBasedStrategy{},
				},
			},
			{
				Name:      fmt.Sprintf("quotas/users/%v/read/config", userID),
				State:     storagepb.Config_ENABLED,
				MaxTokens: 1000,
				ReplenishmentStrategy: &storagepb.Config_TimeBased{
					TimeBased: &storagepb.TimeBasedStrategy{
						ReplenishIntervalSeconds: 100,
						TokensToReplenish:        1000,
					},
				},
			},
		},
	}
	globalWriteConfig = cfgs.Configs[0]
	treeWriteConfig   = cfgs.Configs[1]
	userReadConfig    = cfgs.Configs[2]

	globalWriteSpec = quota.Spec{Group: quota.Global, Kind: quota.Write}
	treeWriteSpec   = quota.Spec{Group: quota.Tree, Kind: quota.Write, TreeID: treeID}
	userReadSpec    = quota.Spec{Group: quota.User, Kind: quota.Read, User: userID}

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

func TestManager_GetTokens(t *testing.T) {
	tests := []struct {
		desc      string
		numTokens int
		specs     []quota.Spec
	}{
		{
			desc:      "singleSpec",
			numTokens: 10,
			specs:     []quota.Spec{globalWriteSpec},
		},
		{
			desc:      "multiSpecs",
			numTokens: 10,
			specs:     []quota.Spec{userReadSpec, treeWriteSpec, globalWriteSpec},
		},
	}

	qs := &storage.QuotaStorage{Client: client}
	qm := New(client)

	ctx := context.Background()
	for _, test := range tests {
		if err := reset(ctx, qs, cfgs); err != nil {
			t.Fatalf("%v: reset: %v", test.desc, err)
		}
		differ := newQuotaDiffer(qs, test.specs)
		if err := differ.snapshot(ctx); err != nil {
			t.Fatalf("%v: snapshot: %v", test.desc, err)
		}
		if err := qm.GetTokens(ctx, test.numTokens, test.specs); err != nil {
			t.Errorf("%v: GetTokens() returned err = %v", test.desc, err)
			continue
		}
		if err := differ.assertDiff(ctx, "GetTokens", -test.numTokens); err != nil {
			t.Errorf("%v: assertDiff: %v", test.desc, err)
		}
	}
}

func TestManager_GetTokensErrors(t *testing.T) {
	tests := []struct {
		desc      string
		numTokens int
		specs     []quota.Spec
	}{
		{
			desc:      "singleSpec",
			numTokens: int(globalWriteConfig.MaxTokens) + 1,
			specs:     []quota.Spec{globalWriteSpec},
		},
		{
			desc:      "multiSpecs",
			numTokens: int(min(userReadConfig.MaxTokens, treeWriteConfig.MaxTokens, globalWriteConfig.MaxTokens)) + 1,
			specs:     []quota.Spec{userReadSpec, treeWriteSpec, globalWriteSpec},
		},
	}

	qs := &storage.QuotaStorage{Client: client}
	qm := New(client)

	ctx := context.Background()
	for _, test := range tests {
		if err := reset(ctx, qs, cfgs); err != nil {
			t.Fatalf("%v: reset: %v", test.desc, err)
		}
		if err := qm.GetTokens(ctx, test.numTokens, test.specs); err == nil {
			t.Errorf("%v: GetTokens() returned err = nil, want non-nil", test.desc)
		}
	}
}

func TestManager_PeekTokens(t *testing.T) {
	ctx := context.Background()
	qs := &storage.QuotaStorage{Client: client}
	if err := reset(ctx, qs, cfgs); err != nil {
		t.Fatalf("reset() returned err = %v", err)
	}

	tests := []struct {
		desc  string
		specs []quota.Spec
		want  map[quota.Spec]int
	}{
		{
			desc:  "singleSpec",
			specs: []quota.Spec{globalWriteSpec},
			want: map[quota.Spec]int{
				globalWriteSpec: int(globalWriteConfig.MaxTokens),
			},
		},
		{
			desc:  "multiSpecs",
			specs: []quota.Spec{userReadSpec, treeWriteSpec, globalWriteSpec},
			want: map[quota.Spec]int{
				globalWriteSpec: int(globalWriteConfig.MaxTokens),
				treeWriteSpec:   int(treeWriteConfig.MaxTokens),
				userReadSpec:    int(userReadConfig.MaxTokens),
			},
		},
	}

	qm := New(client)
	for _, test := range tests {
		tokens, err := qm.PeekTokens(ctx, test.specs)
		if err != nil {
			t.Errorf("%v: PeekTokens() returned err = %v", test.desc, err)
			continue
		}
		if diff := pretty.Compare(tokens, test.want); diff != "" {
			t.Errorf("%v: post-PeekTokens() diff (-got +want):\n%v", test.desc, diff)
		}
	}
}

func TestManager_PutTokens(t *testing.T) {
	ctx := context.Background()
	qs := &storage.QuotaStorage{Client: client}
	if err := drain(ctx, qs, cfgs); err != nil {
		t.Fatalf("drain() returned err = %v", err)
	}

	tests := []struct {
		desc      string
		numTokens int
		specs     []quota.Spec
	}{
		{
			desc:      "singleSpec",
			numTokens: 10,
			specs:     []quota.Spec{globalWriteSpec},
		},
		{
			desc:      "multiSpecs",
			numTokens: 11,
			specs:     []quota.Spec{globalWriteSpec, treeWriteSpec},
		},
	}

	qm := New(client)
	for _, test := range tests {
		differ := newQuotaDiffer(qs, test.specs)
		if err := differ.snapshot(ctx); err != nil {
			t.Fatalf("%v: snapshot: %v", test.desc, err)
		}
		if err := qm.PutTokens(ctx, test.numTokens, test.specs); err != nil {
			t.Errorf("%v: PutTokens() returned err = %v", test.desc, err)
			continue
		}
		if err := differ.assertDiff(ctx, "PutTokens", test.numTokens); err != nil {
			t.Errorf("%v: assertDiff: %v", test.desc, err)
		}
	}
}

func TestManager_ResetQuota(t *testing.T) {
	tests := []struct {
		desc  string
		specs []quota.Spec
		want  map[quota.Spec]int
	}{
		{
			desc:  "singleSpec",
			specs: []quota.Spec{globalWriteSpec},
			want: map[quota.Spec]int{
				globalWriteSpec: int(globalWriteConfig.MaxTokens),
			},
		},
		{
			desc:  "multiSpecs",
			specs: []quota.Spec{globalWriteSpec, treeWriteSpec, userReadSpec},
			want: map[quota.Spec]int{
				globalWriteSpec: int(globalWriteConfig.MaxTokens),
				treeWriteSpec:   int(treeWriteConfig.MaxTokens),
				userReadSpec:    int(userReadConfig.MaxTokens),
			},
		},
	}

	qs := &storage.QuotaStorage{Client: client}
	qm := New(client)

	ctx := context.Background()
	for _, test := range tests {
		if err := drain(ctx, qs, cfgs); err != nil {
			t.Fatalf("%v: drain() returned err = %v", test.desc, err)
		}
		if err := qm.ResetQuota(ctx, test.specs); err != nil {
			t.Errorf("%v: ResetQuota() returned err = %v", test.desc, err)
			continue
		}
		tokens, err := qm.PeekTokens(ctx, test.specs)
		if err != nil {
			t.Fatalf("%v: PeekTokens() returned err = %v", test.desc, err)
		}
		if diff := pretty.Compare(tokens, test.want); diff != "" {
			t.Errorf("%v: post-PeekTokens() diff (-got +want):\n%v", test.desc, diff)
		}
	}
}

func TestConfigName(t *testing.T) {
	tests := []struct {
		spec quota.Spec
		want string
	}{
		{
			spec: quota.Spec{Group: quota.Global, Kind: quota.Read},
			want: "quotas/global/read/config",
		},
		{
			spec: quota.Spec{Group: quota.Global, Kind: quota.Write},
			want: "quotas/global/write/config",
		},
		{
			spec: quota.Spec{Group: quota.Tree, Kind: quota.Read, TreeID: 10},
			want: "quotas/trees/10/read/config",
		},
		{
			spec: quota.Spec{Group: quota.Tree, Kind: quota.Write, TreeID: 11},
			want: "quotas/trees/11/write/config",
		},
		{
			spec: quota.Spec{Group: quota.User, Kind: quota.Read, User: "alpaca"},
			want: "quotas/users/alpaca/read/config",
		},
		{
			spec: quota.Spec{Group: quota.User, Kind: quota.Write, User: "llama"},
			want: "quotas/users/llama/write/config",
		},
	}
	for _, test := range tests {
		if got := configName(test.spec); got != test.want {
			t.Errorf("configName(%+v) = %v, want = %v", test.spec, got, test.want)
		}
	}
}

func min(nums ...int64) int64 {
	ret := int64(math.MaxInt64)
	for _, n := range nums {
		if n < ret {
			ret = n
		}
	}
	return ret
}

// drain applies cfgs (as per reset()) and consumes all tokens from it.
func drain(ctx context.Context, qs *storage.QuotaStorage, cfgs *storagepb.Configs) error {
	if err := reset(ctx, qs, cfgs); err != nil {
		return err
	}
	for _, cfg := range cfgs.Configs {
		if err := qs.Get(ctx, []string{cfg.Name}, cfg.MaxTokens); err != nil {
			return fmt.Errorf("%v: %w", cfg.Name, err)
		}
	}
	return nil
}

func reset(ctx context.Context, qs *storage.QuotaStorage, cfgs *storagepb.Configs) error {
	if _, err := qs.UpdateConfigs(ctx, true /* reset */, func(c *storagepb.Configs) {
		(*c).Reset()
		proto.Merge(c, cfgs)
	}); err != nil {
		return fmt.Errorf("UpdateConfigs() returned err = %w", err)
	}
	return nil
}

type quotaDiffer struct {
	qs     *storage.QuotaStorage
	names  []string
	tokens map[string]int64
}

func newQuotaDiffer(qs *storage.QuotaStorage, specs []quota.Spec) *quotaDiffer {
	return &quotaDiffer{qs: qs, names: configNames(specs)}
}

func (d *quotaDiffer) snapshot(ctx context.Context) error {
	var err error
	d.tokens, err = d.qs.Peek(ctx, d.names)
	return err
}

func (d *quotaDiffer) assertDiff(ctx context.Context, desc string, want int) error {
	currentTokens, err := d.qs.Peek(ctx, d.names)
	if err != nil {
		return fmt.Errorf("in %s: Peek() returned err = %w", desc, err)
	}
	want64 := int64(want)
	for k, v := range currentTokens {
		if got := v - d.tokens[k]; got != want64 {
			return fmt.Errorf("%v has a diff of %v, want = %v", k, got, want64)
		}
	}
	return nil
}
