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
	"fmt"
	"hash"
	"net"
	"testing"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/quota/mysql"
	"github.com/google/trillian/server"
	"github.com/google/trillian/server/admin"
	"github.com/google/trillian/server/interceptor"
	"github.com/google/trillian/storage/testonly"
	"github.com/google/trillian/testonly/integration"
	"github.com/google/trillian/trees"
	"github.com/google/trillian/util"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRateLimiting(t *testing.T) {
	maxUnsequenced := 20

	adminClient, logClient, closeFn, err := setupLogServer(maxUnsequenced)
	if err != nil {
		t.Fatalf("setupLogServer() returned err = %v", err)
	}
	defer closeFn()

	ctx := context.Background()
	tree, err := adminClient.CreateTree(ctx, &trillian.CreateTreeRequest{Tree: testonly.LogTree})
	if err != nil {
		t.Fatalf("CreateTree() returned err = %v", err)
	}
	hasherFn, err := trees.Hash(tree)
	if err != nil {
		t.Fatalf("Hash() returned err = %v", err)
	}
	hasher := hasherFn.New()
	lw := &leafWriter{client: logClient, hash: hasher, treeID: tree.TreeId}

	// Requests where leaves < maxUnsequenced should work
	for i := 0; i < maxUnsequenced; i++ {
		if err := lw.QueueLeaf(ctx); err != nil {
			t.Errorf("QueueLeaf() returned err = %v", err)
		}
	}

	// Some point after now requests should start to fail
	stop := false
	timeout := time.After(1 * time.Second)
	for !stop {
		select {
		case <-timeout:
			t.Error("Timed out before rate limiting kicked in")
			stop = true
		default:
			err := lw.QueueLeaf(ctx)
			if err == nil {
				continue // Rate liming hasn't kicked in yet
			}
			if s, ok := status.FromError(err); !ok || s.Code() != codes.ResourceExhausted {
				t.Fatalf("QueueLeaf() returned err = %v", err)
			}
			stop = true
		}
	}
}

func setupLogServer(maxUnsequenced int) (trillian.TrillianAdminClient, trillian.TrillianLogClient, func(), error) {
	var lis net.Listener
	var s *grpc.Server
	var conn *grpc.ClientConn
	closeFn := func() {
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

	registry, err := integration.NewRegistryForTests("RateLimitingTest")
	if err != nil {
		closeFn()
		return nil, nil, nil, err
	}

	qm, ok := registry.QuotaManager.(*mysql.QuotaManager)
	if !ok {
		closeFn()
		return nil, nil, nil, fmt.Errorf("unexpected QuotaManager type: %T", registry.QuotaManager)
	}
	qm.MaxUnsequencedRows = maxUnsequenced

	intercept := &interceptor.TrillianInterceptor{
		Admin:        registry.AdminStorage,
		QuotaManager: registry.QuotaManager,
	}
	netInterceptor := interceptor.Combine(interceptor.ErrorWrapper, intercept.UnaryInterceptor)
	s = grpc.NewServer(grpc.UnaryInterceptor(netInterceptor))
	trillian.RegisterTrillianAdminServer(s, admin.New(registry))
	trillian.RegisterTrillianLogServer(s, server.NewTrillianLogRPCServer(registry, util.SystemTimeSource{}))

	lis, err = net.Listen("tcp", ":0")
	if err != nil {
		closeFn()
		return nil, nil, nil, err
	}
	go s.Serve(lis)

	conn, err = grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		closeFn()
		return nil, nil, nil, err
	}
	return trillian.NewTrillianAdminClient(conn), trillian.NewTrillianLogClient(conn), closeFn, nil
}

type leafWriter struct {
	client trillian.TrillianLogClient
	hash   hash.Hash
	treeID int64
	leafID int
}

func (w *leafWriter) QueueLeaf(ctx context.Context) error {
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
			MerkleLeafHash:   h,
			LeafValue:        value,
			LeafIdentityHash: h,
		}})
	return err
}
