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

	etcdClient  *clientv3.Client
	quotaClient quotapb.QuotaClient
)

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

// TODO(codingllama): Remove after methods are implemented
func TestServer_Unimplemented(t *testing.T) {
	tests := []struct {
		desc string
		fn   func(context.Context, quotapb.QuotaClient) error
	}{
		{
			desc: "DeleteConfig",
			fn: func(ctx context.Context, client quotapb.QuotaClient) error {
				_, err := client.DeleteConfig(ctx, &quotapb.DeleteConfigRequest{})
				return err
			},
		},
		{
			desc: "ListConfig",
			fn: func(ctx context.Context, client quotapb.QuotaClient) error {
				_, err := client.ListConfigs(ctx, &quotapb.ListConfigsRequest{})
				return err
			},
		},
	}

	ctx := context.Background()
	want := codes.Unimplemented
	for _, test := range tests {
		err := test.fn(ctx, quotaClient)
		switch s, ok := status.FromError(err); {
		case !ok:
			t.Errorf("%v() returned a non-gRPC error: %v", test.desc, err)
		case s.Code() != want:
			t.Errorf("%v() returned err = %v, wantCode = %s", test.desc, err, want)
		}
	}
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

	// Quotas we'll use for the test, one of each type
	globalRead := *globalWrite
	globalRead.Name = "quotas/global/read/config"
	treeRead := *globalWrite
	treeRead.Name = "quotas/trees/1/read/config"
	treeWrite := *globalWrite
	treeWrite.Name = "quotas/trees/1/write/config"
	userRead := *globalWrite
	userRead.Name = "quotas/users/llama/read/config"
	userRead.ReplenishmentStrategy = &quotapb.Config_TimeBased{
		TimeBased: &quotapb.TimeBasedStrategy{
			TokensToReplenish:        userRead.MaxTokens,
			ReplenishIntervalSeconds: 100,
		},
	}
	userWrite := userRead
	userWrite.Name = "quotas/users/llama/write/config"
	configs := []*quotapb.Config{&globalRead, globalWrite, &treeRead, &treeWrite, &userRead, &userWrite}

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
