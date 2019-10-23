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

package quota

import (
	"context"
	"errors"
	"fmt"
	"hash"
	"net"
	"testing"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/extension"
	"github.com/google/trillian/quota/etcd/etcdqm"
	"github.com/google/trillian/quota/etcd/quotaapi"
	"github.com/google/trillian/quota/etcd/quotapb"
	"github.com/google/trillian/quota/mysqlqm"
	"github.com/google/trillian/server"
	"github.com/google/trillian/server/admin"
	"github.com/google/trillian/server/interceptor"
	"github.com/google/trillian/storage/mysql"
	"github.com/google/trillian/storage/testdb"
	"github.com/google/trillian/storage/testonly"
	"github.com/google/trillian/testonly/integration"
	"github.com/google/trillian/testonly/integration/etcd"
	"github.com/google/trillian/trees"
	"github.com/google/trillian/util/clock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
)

func TestEtcdRateLimiting(t *testing.T) {
	testdb.SkipIfNoMySQL(t)
	ctx := context.Background()

	registry, done, err := integration.NewRegistryForTests(ctx)
	if err != nil {
		t.Fatalf("NewRegistryForTests() returned err = %v", err)
	}
	defer done(ctx)

	_, etcdClient, cleanup, err := etcd.StartEtcd()
	if err != nil {
		t.Fatalf("StartEtcd() returned err = %v", err)
	}
	defer cleanup()

	registry.QuotaManager = etcdqm.New(etcdClient)

	s, err := newTestServer(registry)
	if err != nil {
		t.Fatalf("newTestServer() returned err = %v", err)
	}
	defer s.close()

	quotapb.RegisterQuotaServer(s.server, quotaapi.NewServer(etcdClient))
	quotaClient := quotapb.NewQuotaClient(s.conn)
	go s.serve()

	const maxTokens = 100
	if _, err := quotaClient.CreateConfig(ctx, &quotapb.CreateConfigRequest{
		Name: "quotas/global/write/config",
		Config: &quotapb.Config{
			State:     quotapb.Config_ENABLED,
			MaxTokens: maxTokens,
			ReplenishmentStrategy: &quotapb.Config_TimeBased{
				TimeBased: &quotapb.TimeBasedStrategy{
					TokensToReplenish:        maxTokens,
					ReplenishIntervalSeconds: 1000,
				},
			},
		},
	}); err != nil {
		t.Fatalf("CreateConfig() returned err = %v", err)
	}

	if err := runRateLimitingTest(ctx, s, maxTokens); err != nil {
		t.Error(err)
	}
}

func TestMySQLRateLimiting(t *testing.T) {
	testdb.SkipIfNoMySQL(t)
	ctx := context.Background()
	db, done, err := testdb.NewTrillianDB(ctx)
	if err != nil {
		t.Fatalf("GetTestDB() returned err = %v", err)
	}
	defer done(ctx)

	const maxUnsequenced = 20
	qm := &mysqlqm.QuotaManager{DB: db, MaxUnsequencedRows: maxUnsequenced}
	registry := extension.Registry{
		AdminStorage: mysql.NewAdminStorage(db),
		LogStorage:   mysql.NewLogStorage(db, nil),
		MapStorage:   mysql.NewMapStorage(db),
		QuotaManager: qm,
	}

	s, err := newTestServer(registry)
	if err != nil {
		t.Fatalf("newTestServer() returned err = %v", err)
	}
	defer s.close()
	go s.serve()

	if err := runRateLimitingTest(ctx, s, maxUnsequenced); err != nil {
		t.Error(err)
	}
}

func runRateLimitingTest(ctx context.Context, s *testServer, numTokens int) error {
	tree, err := s.admin.CreateTree(ctx, &trillian.CreateTreeRequest{Tree: testonly.LogTree})
	if err != nil {
		return fmt.Errorf("CreateTree() returned err = %v", err)
	}
	// InitLog costs 1 token
	numTokens--
	_, err = s.log.InitLog(ctx, &trillian.InitLogRequest{LogId: tree.TreeId})
	if err != nil {
		return fmt.Errorf("InitLog() returned err = %v", err)
	}
	hasherFn, err := trees.Hash(tree)
	if err != nil {
		return fmt.Errorf("trees.Hash()=%v, want: nil", err)
	}
	hasher := hasherFn.New()
	lw := &leafWriter{client: s.log, hash: hasher, treeID: tree.TreeId}

	// Requests where leaves < numTokens should work
	for i := 0; i < numTokens; i++ {
		if err := lw.queueLeaf(ctx); err != nil {
			return fmt.Errorf("queueLeaf(@%d) returned err = %v", i, err)
		}
	}

	// Some point after now requests should start to fail
	stop := false
	timeout := time.After(1 * time.Second)
	for !stop {
		select {
		case <-timeout:
			return errors.New("timed out before rate limiting kicked in")
		default:
			err := lw.queueLeaf(ctx)
			if err == nil {
				continue // Rate liming hasn't kicked in yet
			}
			if s, ok := status.FromError(err); !ok || s.Code() != codes.ResourceExhausted {
				return fmt.Errorf("queueLeaf() returned err = %v", err)
			}
			stop = true
		}
	}
	return nil
}

type leafWriter struct {
	client trillian.TrillianLogClient
	hash   hash.Hash
	treeID int64
	leafID int
}

func (w *leafWriter) queueLeaf(ctx context.Context) error {
	value := []byte(fmt.Sprintf("leaf-%v", w.leafID))
	w.leafID++

	w.hash.Reset()
	if _, err := w.hash.Write(value); err != nil {
		return err
	}
	h := w.hash.Sum(nil)

	_, err := w.client.QueueLeaf(ctx, &trillian.QueueLeafRequest{
		LogId: w.treeID,
		Leaf: &trillian.LogLeaf{
			MerkleLeafHash: h,
			LeafValue:      value,
		}})
	return err
}

type testServer struct {
	lis    net.Listener
	server *grpc.Server
	conn   *grpc.ClientConn
	admin  trillian.TrillianAdminClient
	log    trillian.TrillianLogClient
}

func (s *testServer) close() {
	if s.conn != nil {
		s.conn.Close()
	}
	if s.server != nil {
		s.server.GracefulStop()
	}
	if s.lis != nil {
		s.lis.Close()
	}
}

func (s *testServer) serve() {
	s.server.Serve(s.lis)
}

// newTestServer returns a new testServer configured for integration tests.
// Callers must defer-call s.close() to make sure resources aren't being leaked and must start the
// server via s.serve().
func newTestServer(registry extension.Registry) (*testServer, error) {
	s := &testServer{}

	ti := interceptor.New(registry.AdminStorage, registry.QuotaManager, false /* quotaDryRun */, registry.MetricFactory)
	s.server = grpc.NewServer(
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(
			interceptor.ErrorWrapper,
			ti.UnaryInterceptor,
		)),
	)
	trillian.RegisterTrillianAdminServer(s.server, admin.New(registry, nil /* allowedTreeTypes */))
	trillian.RegisterTrillianLogServer(s.server, server.NewTrillianLogRPCServer(registry, clock.System))

	var err error
	s.lis, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		s.close()
		return nil, err
	}

	s.conn, err = grpc.Dial(s.lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		s.close()
		return nil, err
	}
	s.admin = trillian.NewTrillianAdminClient(s.conn)
	s.log = trillian.NewTrillianLogClient(s.conn)
	return s, nil
}
