package client

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/types"
	"github.com/transparency-dev/merkle/rfc6962"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeLogClient is a minimal TrillianLogClient that only serves signed log
// roots. It returns codes.NotFound until setRoot is called, after which it
// returns the provided root verbatim.
type fakeLogClient struct {
	trillian.TrillianLogClient
	mu  sync.Mutex
	slr *trillian.SignedLogRoot
}

func (f *fakeLogClient) GetLatestSignedLogRoot(ctx context.Context, _ *trillian.GetLatestSignedLogRootRequest, _ ...grpc.CallOption) (*trillian.GetLatestSignedLogRootResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.slr == nil {
		return nil, status.Error(codes.NotFound, "no root available yet")
	}
	return &trillian.GetLatestSignedLogRootResponse{SignedLogRoot: f.slr}, nil
}

func (f *fakeLogClient) setRoot(slr *trillian.SignedLogRoot) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.slr = slr
}

func signedRoot(t *testing.T, treeSize uint64) *trillian.SignedLogRoot {
	t.Helper()
	root := types.LogRootV1{TimestampNanos: 1, TreeSize: treeSize, RootHash: []byte{0x01}}
	rootBytes, err := root.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary(): %v", err)
	}
	return &trillian.SignedLogRoot{LogRoot: rootBytes}
}

// TestConcurrentWaitForRootUpdate checks that concurrent WaitForRootUpdate
// callers all observe a root update once one is published, rather than only the
// caller that happened to apply the update.
//
// See https://github.com/google/trillian/issues/3294: when two goroutines wait
// for their leaves to be sequenced and both leaves are integrated in a single
// pass, the first goroutine to notice the new root applies it and returns, but
// the others keep waiting because UpdateRoot reports no further change.
func TestConcurrentWaitForRootUpdate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fake := &fakeLogClient{}
	c := New(12345, fake, NewLogVerifier(rfc6962.DefaultHasher), types.LogRootV1{})

	// Both goroutines start waiting for a root update that does not exist yet.
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			root, err := c.WaitForRootUpdate(ctx)
			if err != nil {
				results <- err
				return
			}
			if root == nil {
				results <- fmt.Errorf("WaitForRootUpdate() returned nil root")
				return
			}
			if root.TreeSize != 2 {
				results <- fmt.Errorf("WaitForRootUpdate() returned root with TreeSize %d, want 2", root.TreeSize)
			}
			results <- nil
		}()
	}

	// Make sure both callers are already polling, then publish the new root.
	time.Sleep(200 * time.Millisecond)
	fake.setRoot(signedRoot(t, 2))

	for i := 0; i < 2; i++ {
		select {
		case err := <-results:
			if err != nil {
				t.Errorf("goroutine %d: %v", i, err)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("goroutine %d did not observe the root update", i)
		}
	}
}
