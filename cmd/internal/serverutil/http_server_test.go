package serverutil_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/google/trillian/cmd/internal/serverutil"
	"github.com/google/trillian/extension"
	"google.golang.org/grpc"

	_ "net/http/pprof"
)

func pickFreePort(t *testing.T) int {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected addr type: %T", ln.Addr())
	}
	return addr.Port
}

func httpGetStatus(t *testing.T, url string) int {
	t.Helper()

	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)
	return resp.StatusCode
}

func waitForStatus(t *testing.T, url string, want int) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url) //nolint:gosec
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == want {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %d from %s", want, url)
}

func TestHTTPServerDoesNotExposeDefaultServeMux(t *testing.T) {
	httpPort := pickFreePort(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	m := &serverutil.Main{
		RPCEndpoint:  "127.0.0.1:0",
		HTTPEndpoint: fmt.Sprintf("127.0.0.1:%d", httpPort),
		DBClose:      func() error { return nil },
		Registry:     extension.Registry{},
		RegisterServerFn: func(_ *grpc.Server, _ extension.Registry) error {
			return nil
		},
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- m.Run(ctx)
	}()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", httpPort)
	waitForStatus(t, baseURL+"/healthz", http.StatusOK)

	if got := httpGetStatus(t, baseURL+"/metrics"); got != http.StatusOK {
		t.Fatalf("expected 200 from /metrics, got %d", got)
	}

	if got := httpGetStatus(t, baseURL+"/debug/pprof/"); got != http.StatusNotFound {
		t.Fatalf("expected 404 from /debug/pprof/, got %d", got)
	}

	cancel()
	select {
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout waiting for server shutdown")
	case <-errCh:
	}
}
