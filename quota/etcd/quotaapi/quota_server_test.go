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

	"github.com/coreos/etcd/clientv3"
	"github.com/gogo/protobuf/proto"
	"github.com/google/trillian/quota/etcd/quotapb"
	"github.com/google/trillian/quota/etcd/storage"
	"github.com/google/trillian/quota/etcd/storagepb"
	"github.com/google/trillian/testonly/integration/etcd"
	"github.com/kylelemons/godebug/pretty"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	globalWriteName = "quotas/global/write/config"
)

var (
	globalWrite = &quotapb.Config{
		Name:      globalWriteName,
		State:     quotapb.Config_ENABLED,
		MaxTokens: 100,
		ReplenishmentStrategy: &quotapb.Config_SequencingBased{
			SequencingBased: &quotapb.SequencingBasedStrategy{},
		},
	}
	globalRead = copyWithName(globalWrite, "quotas/global/read/config")
	tree1Read  = copyWithName(globalWrite, "quotas/trees/1/read/config")
	tree1Write = copyWithName(globalWrite, "quotas/trees/1/write/config")
	tree2Read  = copyWithName(globalWrite, "quotas/trees/2/read/config")
	tree2Write = copyWithName(globalWrite, "quotas/trees/2/write/config")
	userRead   = &quotapb.Config{
		Name:      "quotas/users/llama/read/config",
		State:     quotapb.Config_ENABLED,
		MaxTokens: 100,
		ReplenishmentStrategy: &quotapb.Config_TimeBased{
			TimeBased: &quotapb.TimeBasedStrategy{
				TokensToReplenish:        10000,
				ReplenishIntervalSeconds: 100,
			},
		},
	}
	userWrite = copyWithName(userRead, "quotas/users/llama/write/config")

	etcdClient  *clientv3.Client
	quotaClient quotapb.QuotaClient
)

func copyWithName(c *quotapb.Config, name string) *quotapb.Config {
	cp := *c
	cp.Name = name
	return &cp
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
	globalWrite2 := *globalWrite
	globalWrite2.MaxTokens += 10

	overrideName := *globalWrite
	overrideName.Name = "ignored"

	invalidConfig := *globalWrite
	invalidConfig.State = quotapb.Config_UNKNOWN_CONFIG_STATE

	tests := []upsertTest{
		{
			desc:     "emptyName",
			req:      &quotapb.CreateConfigRequest{Config: globalWrite},
			wantCode: codes.InvalidArgument,
		},
		{
			desc:     "emptyConfig",
			req:      &quotapb.CreateConfigRequest{Name: globalWriteName},
			wantCode: codes.InvalidArgument,
		},
		{
			desc:    "success",
			req:     &quotapb.CreateConfigRequest{Name: globalWriteName, Config: globalWrite},
			wantCfg: globalWrite,
		},
		{
			desc:     "alreadyExists",
			baseCfg:  globalWrite,
			req:      &quotapb.CreateConfigRequest{Name: globalWriteName, Config: &globalWrite2},
			wantCode: codes.AlreadyExists,
			wantCfg:  globalWrite,
		},
		{
			desc: "overrideName",
			req: &quotapb.CreateConfigRequest{
				Name:   globalWriteName,
				Config: &overrideName,
			},
			wantCfg: globalWrite,
		},
		{
			desc: "invalidConfig",
			req: &quotapb.CreateConfigRequest{
				Name:   "quotas/global/read/config",
				Config: &invalidConfig,
			},
			// TODO(codingllama): Surface the appropriate error codes from storage
			wantCode: codes.Unknown,
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
	disabledGlobalWrite := *globalWrite
	disabledGlobalWrite.State = quotapb.Config_DISABLED

	timeBasedGlobalWrite := *globalWrite
	timeBasedGlobalWrite.MaxTokens += 100
	timeBasedGlobalWrite.ReplenishmentStrategy = &quotapb.Config_TimeBased{
		TimeBased: &quotapb.TimeBasedStrategy{
			TokensToReplenish:        100,
			ReplenishIntervalSeconds: 200,
		},
	}

	timeBasedGlobalWrite2 := timeBasedGlobalWrite
	timeBasedGlobalWrite2.GetTimeBased().TokensToReplenish += 50
	timeBasedGlobalWrite2.GetTimeBased().ReplenishIntervalSeconds -= 20

	tests := []upsertTest{
		{
			desc:    "disableQuota",
			baseCfg: globalWrite,
			req: &quotapb.UpdateConfigRequest{
				Name:       globalWriteName,
				Config:     &quotapb.Config{State: disabledGlobalWrite.State},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"state"}},
			},
			wantCfg: &disabledGlobalWrite,
		},
		{
			desc:    "timeBasedGlobalWrite",
			baseCfg: globalWrite,
			req: &quotapb.UpdateConfigRequest{
				Name: globalWriteName,
				Config: &quotapb.Config{
					MaxTokens:             timeBasedGlobalWrite.MaxTokens,
					ReplenishmentStrategy: timeBasedGlobalWrite.ReplenishmentStrategy,
				},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"max_tokens", "time_based"}},
			},
			wantCfg: &timeBasedGlobalWrite,
		},
		{
			desc:    "timeBasedGlobalWrite2",
			baseCfg: &timeBasedGlobalWrite,
			req: &quotapb.UpdateConfigRequest{
				Name: timeBasedGlobalWrite.Name,
				Config: &quotapb.Config{
					ReplenishmentStrategy: timeBasedGlobalWrite2.ReplenishmentStrategy,
				},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"time_based"}},
			},
			wantCfg: &timeBasedGlobalWrite2,
		},
		{
			desc: "unknownConfig",
			req: &quotapb.UpdateConfigRequest{
				Name:       globalWriteName,
				Config:     &quotapb.Config{MaxTokens: 200},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"max_tokens"}},
			},
			wantCode: codes.NotFound,
		},
		{
			desc:    "badConfig",
			baseCfg: globalWrite,
			req: &quotapb.UpdateConfigRequest{
				Name:       globalWriteName,
				Config:     &quotapb.Config{}, // State == UNKNOWN
				UpdateMask: &field_mask.FieldMask{Paths: []string{"state"}},
			},
			// TODO(codingllama): Surface the appropriate error codes from storage
			wantCode: codes.Unknown,
			wantCfg:  globalWrite,
		},
		{
			desc:    "badPath",
			baseCfg: globalWrite,
			req: &quotapb.UpdateConfigRequest{
				Name:       globalWriteName,
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
				Name:       globalWriteName,
				UpdateMask: &field_mask.FieldMask{Paths: []string{"max_tokens"}},
			},
			wantCode: codes.InvalidArgument,
			wantCfg:  globalWrite,
		},
		{
			desc:    "emptyMask",
			baseCfg: globalWrite,
			req: &quotapb.UpdateConfigRequest{
				Name:   globalWriteName,
				Config: &quotapb.Config{MaxTokens: 100},
			},
			wantCode: codes.InvalidArgument,
			wantCfg:  globalWrite,
		},
		{
			desc:    "emptyConfigWithReset",
			baseCfg: globalWrite,
			req: &quotapb.UpdateConfigRequest{
				Name:       globalWriteName,
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
				Name:       globalWriteName,
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
		Name:   globalWriteName,
		Config: globalWrite,
	}); err != nil {
		t.Fatalf("CreateConfig() returned err = %v", err)
	}
	qs := storage.QuotaStorage{Client: etcdClient}
	if err := qs.Get(ctx, []string{globalWriteName}, globalWrite.MaxTokens-1); err != nil {
		t.Fatalf("Get() returned err = %v", err)
	}

	cfg := *globalWrite
	cfg.MaxTokens += 10

	tests := []struct {
		desc       string
		req        *quotapb.UpdateConfigRequest
		wantTokens int64
	}{
		{
			desc: "resetFalse",
			req: &quotapb.UpdateConfigRequest{
				Name:       globalWriteName,
				Config:     &cfg,
				UpdateMask: &field_mask.FieldMask{Paths: []string{"max_tokens"}},
			},
			wantTokens: 1,
		},
		{
			desc:       "resetTrue",
			req:        &quotapb.UpdateConfigRequest{Name: globalWriteName, ResetQuota: true},
			wantTokens: cfg.MaxTokens,
		},
	}
	for _, test := range tests {
		if _, err := quotaClient.UpdateConfig(ctx, test.req); err != nil {
			t.Errorf("%v: UpdateConfig() returned err = %v", test.desc, err)
			continue
		}

		tokens, err := qs.Peek(ctx, []string{test.req.Name})
		if err != nil {
			t.Fatalf("%v: Peek() returned err = %v", test.desc, err)
		}
		if got := tokens[test.req.Name]; got != test.wantTokens {
			t.Errorf("%v: %q has %v tokens, want = %v", test.desc, test.req.Name, got, test.wantTokens)
		}
	}
}

func TestServer_UpdateConfig_Race(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping race test due to short mode")
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
			go func(num int, want quotapb.Config) {
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

					want.MaxTokens = tokens
					if !proto.Equal(got, &want) {
						diff := pretty.Compare(got, &want)
						t.Errorf("%v: post-UpdateConfig() diff (-got +want):\n%v", want.Name, diff)
					}
				}
			}(num, *cfg)
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
		return fmt.Errorf("%v: post-%v() diff:\n%v", test.desc, rpcName, pretty.Compare(cfg, test.wantCfg))
	case test.wantCfg == nil:
		return nil
	}

	switch stored, err := quotaClient.GetConfig(ctx, &quotapb.GetConfigRequest{Name: test.req.GetName()}); {
	case err != nil:
		return fmt.Errorf("%v: GetConfig() returned err = %v", test.desc, err)
	case !proto.Equal(stored, test.wantCfg):
		return fmt.Errorf("%v: post-GetConfig() diff:\n%v", test.desc, pretty.Compare(stored, test.wantCfg))
	}

	return nil
}

func TestServer_DeleteConfigErrors(t *testing.T) {
	tests := []struct {
		desc     string
		req      *quotapb.DeleteConfigRequest
		wantCode codes.Code
	}{
		{
			desc:     "unimplemented",
			req:      &quotapb.DeleteConfigRequest{Name: globalWriteName},
			wantCode: codes.Unimplemented,
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
			desc: "badName",
			req:  &quotapb.GetConfigRequest{Name: "not/a/config/name"},
			// TODO(codingllama): Validate names on Get/List and surface the appropriate errors
			wantCode: codes.NotFound,
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
	basicGlobalWrite := &quotapb.Config{Name: globalWriteName}
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
				basicGlobalWrite, basicGlobalRead,
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

		got := resp.Configs
		want := resp.Configs
		sort.Slice(got, func(i, j int) bool { return strings.Compare(got[i].Name, got[j].Name) == -1 })
		sort.Slice(want, func(i, j int) bool { return strings.Compare(want[i].Name, want[j].Name) == -1 })
		if diff := pretty.Compare(got, want); diff != "" {
			t.Errorf("%v: post-ListConfigs() diff (-got +want):\n:%v", test.desc, diff)
		}
	}
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

	s = grpc.NewServer()
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
