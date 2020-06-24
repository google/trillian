// Copyright 2017 Google LLC. All Rights Reserved.
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

package quotaapi

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian/quota"
	"github.com/google/trillian/quota/etcd/quotapb"
	"github.com/google/trillian/quota/etcd/storage"
	"github.com/google/trillian/quota/etcd/storagepb"
	"github.com/google/trillian/server/interceptor"
	"github.com/google/trillian/testonly/integration/etcd"
	"go.etcd.io/etcd/clientv3"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

var (
	globalWrite = &quotapb.Config{
		Name:          "quotas/global/write/config",
		State:         quotapb.Config_ENABLED,
		MaxTokens:     100,
		CurrentTokens: 100, // Assume quota is full
		ReplenishmentStrategy: &quotapb.Config_SequencingBased{
			SequencingBased: &quotapb.SequencingBasedStrategy{},
		},
	}
	globalRead = &quotapb.Config{
		Name:          "quotas/global/read/config",
		State:         quotapb.Config_ENABLED,
		MaxTokens:     100,
		CurrentTokens: 100, // Assume quota is full
		ReplenishmentStrategy: &quotapb.Config_TimeBased{
			TimeBased: &quotapb.TimeBasedStrategy{
				TokensToReplenish:        10000,
				ReplenishIntervalSeconds: 100,
			},
		},
	}
	tree1Read  = copyWithName(globalRead, "quotas/trees/1/read/config")
	tree1Write = copyWithName(globalWrite, "quotas/trees/1/write/config")
	tree2Read  = copyWithName(globalRead, "quotas/trees/2/read/config")
	tree2Write = copyWithName(globalWrite, "quotas/trees/2/write/config")
	userRead   = copyWithName(globalRead, "quotas/users/llama/read/config")
	userWrite  = copyWithName(globalRead, "quotas/users/llama/write/config")

	etcdClient  *clientv3.Client
	quotaClient quotapb.QuotaClient
)

func copyWithName(c *quotapb.Config, name string) *quotapb.Config {
	cp := proto.Clone(c).(*quotapb.Config)
	cp.Name = name
	return cp
}

func TestMain(m *testing.M) {
	_, ec, stopEtcd, err := etcd.StartEtcd()
	if err != nil {
		panic(fmt.Sprintf("StartEtcd() returned err = %v", err))
	}
	// Don't defer stopEtcd(): the os.Exit() call below doesn't allow for defers.
	etcdClient = ec

	qc, stopServer, err := startServer(ec)
	if err != nil {
		stopEtcd()
		panic(fmt.Sprintf("startServer() returned err = %v", err))
	}
	// Don't defer stopServer().
	quotaClient = qc

	exitCode := m.Run()
	stopServer()
	stopEtcd()
	os.Exit(exitCode)
}

func TestServer_CreateConfig(t *testing.T) {
	globalWrite2 := proto.Clone(globalWrite).(*quotapb.Config)
	globalWrite2.MaxTokens += 10
	globalWrite2.CurrentTokens = globalWrite2.MaxTokens

	overrideName := proto.Clone(globalWrite).(*quotapb.Config)
	overrideName.Name = "ignored"

	invalidConfig := proto.Clone(globalWrite).(*quotapb.Config)
	invalidConfig.State = quotapb.Config_UNKNOWN_CONFIG_STATE

	tests := []upsertTest{
		{
			desc:     "emptyName",
			req:      &quotapb.CreateConfigRequest{Config: globalWrite},
			wantCode: codes.InvalidArgument,
		},
		{
			desc:     "emptyConfig",
			req:      &quotapb.CreateConfigRequest{Name: globalWrite.Name},
			wantCode: codes.InvalidArgument,
		},
		{
			desc:    "success",
			req:     &quotapb.CreateConfigRequest{Name: globalWrite.Name, Config: globalWrite},
			wantCfg: globalWrite,
		},
		{
			desc:     "alreadyExists",
			baseCfg:  globalWrite,
			req:      &quotapb.CreateConfigRequest{Name: globalWrite.Name, Config: globalWrite2},
			wantCode: codes.AlreadyExists,
			wantCfg:  globalWrite,
		},
		{
			desc: "overrideName",
			req: &quotapb.CreateConfigRequest{
				Name:   globalWrite.Name,
				Config: overrideName,
			},
			wantCfg: globalWrite,
		},
		{
			desc: "invalidConfig",
			req: &quotapb.CreateConfigRequest{
				Name:   "quotas/global/read/config",
				Config: invalidConfig,
			},
			wantCode: codes.InvalidArgument,
		},
	}

	ctx := context.Background()
	rpc := func(ctx context.Context, req interface{}) (*quotapb.Config, error) {
		return quotaClient.CreateConfig(ctx, req.(*quotapb.CreateConfigRequest))
	}
	for _, test := range tests {
		if err := runUpsertTest(ctx, test, rpc, "CreateConfig"); err != nil {
			t.Error(err)
		}
	}
}

func TestServer_UpdateConfig(t *testing.T) {
	disabledGlobalWrite := proto.Clone(globalWrite).(*quotapb.Config)
	// Disabled quotas have "infinite" tokens
	disabledGlobalWrite.CurrentTokens = int64(quota.MaxTokens)
	disabledGlobalWrite.State = quotapb.Config_DISABLED

	timeBasedGlobalWrite := proto.Clone(globalWrite).(*quotapb.Config)
	timeBasedGlobalWrite.MaxTokens += 100
	timeBasedGlobalWrite.CurrentTokens = globalWrite.MaxTokens
	timeBasedGlobalWrite.ReplenishmentStrategy = &quotapb.Config_TimeBased{
		TimeBased: &quotapb.TimeBasedStrategy{
			TokensToReplenish:        100,
			ReplenishIntervalSeconds: 200,
		},
	}

	timeBasedGlobalWrite2 := proto.Clone(timeBasedGlobalWrite).(*quotapb.Config)
	timeBasedGlobalWrite2.CurrentTokens = timeBasedGlobalWrite.MaxTokens
	timeBasedGlobalWrite2.GetTimeBased().TokensToReplenish += 50
	timeBasedGlobalWrite2.GetTimeBased().ReplenishIntervalSeconds -= 20

	tests := []upsertTest{
		{
			desc:    "disableQuota",
			baseCfg: globalWrite,
			req: &quotapb.UpdateConfigRequest{
				Name:       globalWrite.Name,
				Config:     &quotapb.Config{State: disabledGlobalWrite.State},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"state"}},
			},
			wantCfg: disabledGlobalWrite,
		},
		{
			desc:    "timeBasedGlobalWrite",
			baseCfg: globalWrite,
			req: &quotapb.UpdateConfigRequest{
				Name: globalWrite.Name,
				Config: &quotapb.Config{
					MaxTokens:             timeBasedGlobalWrite.MaxTokens,
					ReplenishmentStrategy: timeBasedGlobalWrite.ReplenishmentStrategy,
				},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"max_tokens", "time_based"}},
			},
			wantCfg: timeBasedGlobalWrite,
		},
		{
			desc:    "timeBasedGlobalWrite2",
			baseCfg: timeBasedGlobalWrite,
			req: &quotapb.UpdateConfigRequest{
				Name: timeBasedGlobalWrite.Name,
				Config: &quotapb.Config{
					ReplenishmentStrategy: timeBasedGlobalWrite2.ReplenishmentStrategy,
				},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"time_based"}},
			},
			wantCfg: timeBasedGlobalWrite2,
		},
		{
			desc: "unknownConfig",
			req: &quotapb.UpdateConfigRequest{
				Name:       globalWrite.Name,
				Config:     &quotapb.Config{MaxTokens: 200},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"max_tokens"}},
			},
			wantCode: codes.NotFound,
		},
		{
			desc:    "badConfig",
			baseCfg: globalWrite,
			req: &quotapb.UpdateConfigRequest{
				Name:       globalWrite.Name,
				Config:     &quotapb.Config{}, // State == UNKNOWN
				UpdateMask: &field_mask.FieldMask{Paths: []string{"state"}},
			},
			wantCode: codes.InvalidArgument,
			wantCfg:  globalWrite,
		},
		{
			desc:    "badPath",
			baseCfg: globalWrite,
			req: &quotapb.UpdateConfigRequest{
				Name:       globalWrite.Name,
				Config:     &quotapb.Config{},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"NOT_A_FIELD"}},
			},
			wantCode: codes.InvalidArgument,
			wantCfg:  globalWrite,
		},
		{
			desc: "emptyName",
			req: &quotapb.UpdateConfigRequest{
				Config:     &quotapb.Config{MaxTokens: 100},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"max_tokens"}},
			},
			wantCode: codes.InvalidArgument,
		},
		{
			desc:    "emptyConfig",
			baseCfg: globalWrite,
			req: &quotapb.UpdateConfigRequest{
				Name:       globalWrite.Name,
				UpdateMask: &field_mask.FieldMask{Paths: []string{"max_tokens"}},
			},
			wantCode: codes.InvalidArgument,
			wantCfg:  globalWrite,
		},
		{
			desc:    "emptyMask",
			baseCfg: globalWrite,
			req: &quotapb.UpdateConfigRequest{
				Name:   globalWrite.Name,
				Config: &quotapb.Config{MaxTokens: 100},
			},
			wantCode: codes.InvalidArgument,
			wantCfg:  globalWrite,
		},
		{
			desc:    "emptyConfigWithReset",
			baseCfg: globalWrite,
			req: &quotapb.UpdateConfigRequest{
				Name:       globalWrite.Name,
				UpdateMask: &field_mask.FieldMask{Paths: []string{"max_tokens"}},
				ResetQuota: true,
			},
			wantCode: codes.InvalidArgument,
			wantCfg:  globalWrite,
		},
		{
			desc:    "emptyMaskWithReset",
			baseCfg: globalWrite,
			req: &quotapb.UpdateConfigRequest{
				Name:       globalWrite.Name,
				Config:     &quotapb.Config{MaxTokens: 100},
				ResetQuota: true,
			},
			wantCode: codes.InvalidArgument,
			wantCfg:  globalWrite,
		},
	}

	ctx := context.Background()
	rpc := func(ctx context.Context, req interface{}) (*quotapb.Config, error) {
		return quotaClient.UpdateConfig(ctx, req.(*quotapb.UpdateConfigRequest))
	}
	for _, test := range tests {
		if err := runUpsertTest(ctx, test, rpc, "UpdateConfig"); err != nil {
			t.Error(err)
		}
	}
}

func TestServer_UpdateConfig_ResetQuota(t *testing.T) {
	ctx := context.Background()
	if err := reset(ctx); err != nil {
		t.Fatalf("reset() returned err = %v", err)
	}
	if _, err := quotaClient.CreateConfig(ctx, &quotapb.CreateConfigRequest{
		Name:   globalWrite.Name,
		Config: globalWrite,
	}); err != nil {
		t.Fatalf("CreateConfig() returned err = %v", err)
	}
	qs := storage.QuotaStorage{Client: etcdClient}
	if err := qs.Get(ctx, []string{globalWrite.Name}, globalWrite.MaxTokens-1); err != nil {
		t.Fatalf("Get() returned err = %v", err)
	}

	cfg := proto.Clone(globalWrite).(*quotapb.Config)
	cfg.MaxTokens += 10

	tests := []struct {
		desc       string
		req        *quotapb.UpdateConfigRequest
		wantTokens int64
	}{
		{
			desc: "resetFalse",
			req: &quotapb.UpdateConfigRequest{
				Name:       globalWrite.Name,
				Config:     cfg,
				UpdateMask: &field_mask.FieldMask{Paths: []string{"max_tokens"}},
			},
			wantTokens: 1,
		},
		{
			desc:       "resetTrue",
			req:        &quotapb.UpdateConfigRequest{Name: globalWrite.Name, ResetQuota: true},
			wantTokens: cfg.MaxTokens,
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			if _, err := quotaClient.UpdateConfig(ctx, test.req); err != nil {
				t.Errorf("UpdateConfig() returned err = %v", err)
				return
			}

			tokens, err := qs.Peek(ctx, []string{test.req.Name})
			if err != nil {
				t.Fatalf("Peek() returned err = %v", err)
			}
			if got := tokens[test.req.Name]; got != test.wantTokens {
				t.Errorf("%q has %v tokens, want = %v", test.req.Name, got, test.wantTokens)
			}
		})
	}
}

func TestServer_UpdateConfig_ConcurrentUpdates(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrency test due to short mode")
		return
	}

	ctx := context.Background()
	if err := reset(ctx); err != nil {
		t.Fatalf("reset() returned err = %v", err)
	}

	// Quotas we'll use for the test, a few of each type
	configs := []*quotapb.Config{globalRead, globalWrite, tree1Read, tree1Write, userRead, userWrite}

	// Create the configs we'll use for the test concurrently.
	// If we get any errors a msg is sent to createErrs; that's a sign to stop
	// the test *after* all spawned goroutines have exited.
	createErrs := make(chan bool, len(configs))
	wg := sync.WaitGroup{}
	for _, cfg := range configs {
		cfg := cfg
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := quotaClient.CreateConfig(ctx, &quotapb.CreateConfigRequest{
				Name:   cfg.Name,
				Config: cfg,
			}); err != nil {
				t.Errorf("%v: CreateConfig() returned err = %v", cfg.Name, err)
				createErrs <- true
			}
		}()
	}
	wg.Wait()
	select {
	case <-createErrs:
		return
	default:
	}

	const routinesPerConfig = 4
	const updatesPerRoutine = 100
	for _, cfg := range configs {
		for num := 0; num < routinesPerConfig; num++ {
			wg.Add(1)
			go func(num int, want *quotapb.Config) {
				defer wg.Done()
				baseTokens := 1 + rand.Intn(routinesPerConfig*100)
				reset := num%2 == 0
				for i := 0; i < updatesPerRoutine; i++ {
					tokens := int64(baseTokens + i)
					got, err := quotaClient.UpdateConfig(ctx, &quotapb.UpdateConfigRequest{
						Name:       want.Name,
						Config:     &quotapb.Config{MaxTokens: tokens},
						UpdateMask: &field_mask.FieldMask{Paths: []string{"max_tokens"}},
						ResetQuota: reset,
					})
					if err != nil {
						t.Errorf("%v: UpdateConfig() returned err = %v", want.Name, err)
						continue
					}

					want.CurrentTokens = got.CurrentTokens // Not important for this test
					want.MaxTokens = tokens
					if !proto.Equal(got, want) {
						diff := cmp.Diff(got, &want, cmp.Comparer(proto.Equal))
						t.Errorf("%v: post-UpdateConfig() diff (-got +want):\n%v", want.Name, diff)
					}
				}
			}(num, proto.Clone(cfg).(*quotapb.Config))
		}
	}
	wg.Wait()
}

// upsertTest represents either a CreateConfig or UpdateConfig test.
type upsertTest struct {
	desc string
	// baseCfg is created before calling the RPC, if non-nil.
	baseCfg  *quotapb.Config
	req      upsertRequest
	wantCode codes.Code
	wantCfg  *quotapb.Config
}

type upsertRequest interface {
	GetName() string
}

type upsertRPC func(ctx context.Context, req interface{}) (*quotapb.Config, error)

// runUpsertTest runs either CreateConfig or UpdateConfig tests, depending on the supplied rpc.
// Storage is reset() before the test is executed.
func runUpsertTest(ctx context.Context, test upsertTest, rpc upsertRPC, rpcName string) error {
	if err := reset(ctx); err != nil {
		return fmt.Errorf("%v: reset() returned err = %v", test.desc, err)
	}
	if test.baseCfg != nil {
		if _, err := quotaClient.CreateConfig(ctx, &quotapb.CreateConfigRequest{
			Name:   test.baseCfg.Name,
			Config: test.baseCfg,
		}); err != nil {
			return fmt.Errorf("%v: CreateConfig() returned err = %v", test.desc, err)
		}
	}

	cfg, err := rpc(ctx, test.req)
	switch s, ok := status.FromError(err); {
	case !ok || s.Code() != test.wantCode:
		return fmt.Errorf("%v: %v() returned err = %v, wantCode = %s", test.desc, rpcName, err, test.wantCode)
	case test.wantCode == codes.OK && !proto.Equal(cfg, test.wantCfg):
		return fmt.Errorf("%v: post-%v() diff:\n%v", test.desc, rpcName, cmp.Diff(cfg, test.wantCfg, cmp.Comparer(proto.Equal)))
	case test.wantCfg == nil:
		return nil
	}

	switch stored, err := quotaClient.GetConfig(ctx, &quotapb.GetConfigRequest{Name: test.req.GetName()}); {
	case err != nil:
		return fmt.Errorf("%v: GetConfig() returned err = %v", test.desc, err)
	case !proto.Equal(stored, test.wantCfg):
		return fmt.Errorf("%v: post-GetConfig() diff:\n%v", test.desc, cmp.Diff(stored, test.wantCfg, cmp.Comparer(proto.Equal)))
	}

	return nil
}

func TestServer_DeleteConfig(t *testing.T) {
	tests := []struct {
		desc                 string
		createCfgs, wantCfgs []*quotapb.Config
		req                  *quotapb.DeleteConfigRequest
	}{
		{
			desc: "success",
			createCfgs: []*quotapb.Config{
				globalRead, globalWrite,
				tree1Read, tree1Write,
				tree2Read, tree2Write,
				userRead, userWrite,
			},
			wantCfgs: []*quotapb.Config{
				globalRead, globalWrite,
				tree1Write,
				tree2Read, tree2Write,
				userRead, userWrite,
			},
			req: &quotapb.DeleteConfigRequest{Name: tree1Read.Name},
		},
		{
			desc:       "lastConfig",
			createCfgs: []*quotapb.Config{tree1Read},
			req:        &quotapb.DeleteConfigRequest{Name: tree1Read.Name},
		},
		{
			desc:       "global",
			createCfgs: []*quotapb.Config{globalRead, globalWrite},
			wantCfgs:   []*quotapb.Config{globalRead},
			req:        &quotapb.DeleteConfigRequest{Name: globalWrite.Name},
		},
		{
			desc:       "user",
			createCfgs: []*quotapb.Config{userRead, userWrite},
			wantCfgs:   []*quotapb.Config{userWrite},
			req:        &quotapb.DeleteConfigRequest{Name: userRead.Name},
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		if err := reset(ctx); err != nil {
			t.Errorf("%v: reset() returned err = %v", test.desc, err)
			continue
		}

		for _, cfg := range test.createCfgs {
			if _, err := quotaClient.CreateConfig(ctx, &quotapb.CreateConfigRequest{
				Name:   cfg.Name,
				Config: cfg,
			}); err != nil {
				t.Fatalf("%v: CreateConfig(%q) returned err = %v", test.desc, cfg.Name, err)
			}
		}

		if _, err := quotaClient.DeleteConfig(ctx, test.req); err != nil {
			t.Errorf("%v: DeleteConfig(%q) returned err = %v", test.desc, test.req.Name, err)
			continue
		}

		resp, err := quotaClient.ListConfigs(ctx, &quotapb.ListConfigsRequest{
			View: quotapb.ListConfigsRequest_FULL,
		})
		if err != nil {
			t.Errorf("%v: ListConfigs() returned err = %v", test.desc, err)
			continue
		}
		if err := sortAndCompare(resp.Configs, test.wantCfgs); err != nil {
			t.Errorf("%v: post-DeleteConfig() %v", test.desc, err)
		}
	}
}

func TestServer_DeleteConfigErrors(t *testing.T) {
	tests := []struct {
		desc     string
		req      *quotapb.DeleteConfigRequest
		wantCode codes.Code
	}{
		{
			desc:     "emptyName",
			req:      &quotapb.DeleteConfigRequest{},
			wantCode: codes.InvalidArgument,
		},
		{
			desc:     "badName",
			req:      &quotapb.DeleteConfigRequest{Name: "bad/quota/name"},
			wantCode: codes.InvalidArgument,
		},
		{
			desc:     "unknown",
			req:      &quotapb.DeleteConfigRequest{Name: globalWrite.Name},
			wantCode: codes.NotFound,
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		_, err := quotaClient.DeleteConfig(ctx, test.req)
		if s, ok := status.FromError(err); !ok || s.Code() != test.wantCode {
			t.Errorf("%v: DeleteConfig() returned err = %v, wantCode = %s", test.desc, err, test.wantCode)
		}
	}
}

func TestServer_GetConfigErrors(t *testing.T) {
	ctx := context.Background()
	if err := reset(ctx); err != nil {
		t.Fatalf("reset() returned err = %v", err)
	}

	tests := []struct {
		desc     string
		req      *quotapb.GetConfigRequest
		wantCode codes.Code
	}{
		{
			desc:     "emptyName",
			req:      &quotapb.GetConfigRequest{},
			wantCode: codes.InvalidArgument,
		},
		{
			desc:     "badName",
			req:      &quotapb.GetConfigRequest{Name: "not/a/config/name"},
			wantCode: codes.InvalidArgument,
		},
		{
			desc:     "notFound",
			req:      &quotapb.GetConfigRequest{Name: "quotas/global/write/config"},
			wantCode: codes.NotFound,
		},
	}
	for _, test := range tests {
		// GetConfig's success return is tested as a consequence of other test cases.
		_, err := quotaClient.GetConfig(ctx, test.req)
		if s, ok := status.FromError(err); !ok || s.Code() != test.wantCode {
			t.Errorf("%v: GetConfig() returned err = %v, wantCode = %s", test.desc, err, test.wantCode)
		}
	}
}

func TestServer_ListConfigs(t *testing.T) {
	ctx := context.Background()
	if err := reset(ctx); err != nil {
		t.Fatalf("reset() returned err = %v", err)
	}

	// Test listing with an empty config first, so we can work with a fixed list of configs from
	// here on
	resp, err := quotaClient.ListConfigs(ctx, &quotapb.ListConfigsRequest{})
	switch {
	case err != nil:
		t.Errorf("empty: ListConfigs() returned err = %v", err)
	case len(resp.Configs) != 0:
		t.Errorf("empty: ListConfigs() returned >0 results: %v", err)
	}

	configs := []*quotapb.Config{
		globalRead, globalWrite,
		tree1Read, tree1Write,
		tree2Read, tree2Write,
		userRead, userWrite,
	}
	for _, cfg := range configs {
		if _, err := quotaClient.CreateConfig(ctx, &quotapb.CreateConfigRequest{
			Name:   cfg.Name,
			Config: cfg,
		}); err != nil {
			t.Fatalf("%q: CreateConfig() returned err = %v", cfg.Name, err)
		}
	}

	basicGlobalRead := &quotapb.Config{Name: globalRead.Name}
	basicGlobalWrite := &quotapb.Config{Name: globalWrite.Name}
	basicTree1Read := &quotapb.Config{Name: tree1Read.Name}
	basicTree1Write := &quotapb.Config{Name: tree1Write.Name}
	basicTree2Read := &quotapb.Config{Name: tree2Read.Name}
	basicTree2Write := &quotapb.Config{Name: tree2Write.Name}
	basicUserRead := &quotapb.Config{Name: userRead.Name}
	basicUserWrite := &quotapb.Config{Name: userWrite.Name}

	tests := []struct {
		desc     string
		req      *quotapb.ListConfigsRequest
		wantCfgs []*quotapb.Config
	}{
		{
			desc: "allBasicView",
			req:  &quotapb.ListConfigsRequest{},
			wantCfgs: []*quotapb.Config{
				basicGlobalRead, basicGlobalWrite,
				basicTree1Read, basicTree1Write,
				basicTree2Read, basicTree2Write,
				basicUserRead, basicUserWrite,
			},
		},
		{
			desc:     "allFullView",
			req:      &quotapb.ListConfigsRequest{View: quotapb.ListConfigsRequest_FULL},
			wantCfgs: configs,
		},
		{
			desc:     "allTrees",
			req:      &quotapb.ListConfigsRequest{Names: []string{"quotas/trees/-/-/config"}},
			wantCfgs: []*quotapb.Config{basicTree1Read, basicTree1Write, basicTree2Read, basicTree2Write},
		},
		{
			desc:     "allUsers",
			req:      &quotapb.ListConfigsRequest{Names: []string{"quotas/users/-/-/config"}},
			wantCfgs: []*quotapb.Config{basicUserRead, basicUserWrite},
		},
		{
			desc: "unknowns",
			req: &quotapb.ListConfigsRequest{
				Names: []string{
					"quotas/trees/99997/read/config",
					"quotas/trees/99998/write/config",
					"quotas/trees/99999/-/config",
					"quotas/users/unknown/-/config",
				},
			},
		},
	}
	for _, test := range tests {
		resp, err := quotaClient.ListConfigs(ctx, test.req)
		if err != nil {
			t.Errorf("%v: ListConfigs() returned err = %v", test.desc, err)
			continue
		}
		if err := sortAndCompare(resp.Configs, test.wantCfgs); err != nil {
			t.Errorf("%v: post-ListConfigs() %v", test.desc, err)
		}
	}
}

func sortAndCompare(got, want []*quotapb.Config) error {
	sort.Slice(got, func(i, j int) bool { return strings.Compare(got[i].Name, got[j].Name) == -1 })
	sort.Slice(want, func(i, j int) bool { return strings.Compare(want[i].Name, want[j].Name) == -1 })
	if len(got) != len(want) {
		return fmt.Errorf("got %v configs, want %v", len(got), len(want))
	}
	for i, cfg := range want {
		if !proto.Equal(got[i], cfg) {
			return fmt.Errorf("diff (-got +want):\n%v", cmp.Diff(got[i], cfg, cmp.Comparer(proto.Equal)))
		}
	}
	return nil
}

func TestServer_ListConfigsErrors(t *testing.T) {
	tests := []struct {
		desc     string
		req      *quotapb.ListConfigsRequest
		wantCode codes.Code
	}{
		{
			desc:     "badNameFilter",
			req:      &quotapb.ListConfigsRequest{Names: []string{"bad/quota/name"}},
			wantCode: codes.InvalidArgument,
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		_, err := quotaClient.ListConfigs(ctx, test.req)
		if s, ok := status.FromError(err); !ok || s.Code() != test.wantCode {
			t.Errorf("%v: ListConfigs() returned err = %v, wantCode = %s", test.desc, err, test.wantCode)
		}
	}
}

func reset(ctx context.Context) error {
	qs := storage.QuotaStorage{Client: etcdClient}
	_, err := qs.UpdateConfigs(ctx, true /* reset */, func(c *storagepb.Configs) { *c = storagepb.Configs{} })
	return err
}

func startServer(etcdClient *clientv3.Client) (quotapb.QuotaClient, func(), error) {
	var lis net.Listener
	var s *grpc.Server
	var conn *grpc.ClientConn

	cleanup := func() {
		if conn != nil {
			conn.Close()
		}
		if s != nil {
			s.GracefulStop()
		}
		if lis != nil {
			lis.Close()
		}
	}

	var err error
	lis, err = net.Listen("tcp", "localhost:0")
	if err != nil {
		cleanup()
		return nil, nil, err
	}

	s = grpc.NewServer(grpc.UnaryInterceptor(interceptor.ErrorWrapper))
	quotapb.RegisterQuotaServer(s, NewServer(etcdClient))
	go s.Serve(lis)

	conn, err = grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		cleanup()
		return nil, nil, err
	}

	quotaClient := quotapb.NewQuotaClient(conn)
	return quotaClient, cleanup, nil
}
