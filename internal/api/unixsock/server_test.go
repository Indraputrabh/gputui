package unixsock

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/indraputrabh/gputui/internal/model"
)

type staticProvider struct {
	snapshot model.Snapshot
	ok       bool
}

func (p staticProvider) LatestSnapshot() (model.Snapshot, bool) {
	return p.snapshot, p.ok
}

func TestServerEndpoints(t *testing.T) {
	t.Parallel()

	socketPath := testSocketPath(t)
	server, err := NewServer(socketPath, staticProvider{})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	if err := server.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if shutdownErr := server.Shutdown(ctx); shutdownErr != nil {
			t.Fatalf("shutdown server: %v", shutdownErr)
		}
	})

	client := unixClient(t, socketPath)
	mustGetStatus(t, client, "/healthz", http.StatusOK)
	mustGetStatus(t, client, "/v1/snapshot", http.StatusServiceUnavailable)
}

func TestServerSnapshotResponse(t *testing.T) {
	t.Parallel()

	socketPath := testSocketPath(t)
	expectedTS := time.Now().UTC().Round(time.Second)
	server, err := NewServer(socketPath, staticProvider{
		ok: true,
		snapshot: model.Snapshot{
			TS:    expectedTS,
			GPUs:  []model.GPUStat{},
			Procs: []model.ProcStat{},
			Node:  model.NodeStat{MemTotal: 1024},
		},
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	if err := server.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if shutdownErr := server.Shutdown(ctx); shutdownErr != nil {
			t.Fatalf("shutdown server: %v", shutdownErr)
		}
	})

	client := unixClient(t, socketPath)
	resp, err := client.Get("http://unix/v1/snapshot")
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: got=%d want=%d", resp.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var got model.Snapshot
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if !got.TS.Equal(expectedTS) {
		t.Fatalf("snapshot ts mismatch: got=%s want=%s", got.TS, expectedTS)
	}
	if got.Node.MemTotal != 1024 {
		t.Fatalf("snapshot node.mem_total mismatch: got=%d want=%d", got.Node.MemTotal, 1024)
	}
}

func unixClient(t *testing.T, socketPath string) *http.Client {
	t.Helper()

	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			dialer := &net.Dialer{}
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
	t.Cleanup(transport.CloseIdleConnections)

	return &http.Client{
		Transport: transport,
		Timeout:   2 * time.Second,
	}
}

func mustGetStatus(t *testing.T, client *http.Client, path string, want int) {
	t.Helper()

	resp, err := client.Get("http://unix" + path)
	if err != nil {
		t.Fatalf("get %s: %v", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != want {
		t.Fatalf("status %s: got=%d want=%d", path, resp.StatusCode, want)
	}
}

func testSocketPath(t *testing.T) string {
	t.Helper()
	return "/tmp/gputui-agent-" + strconv.FormatInt(time.Now().UnixNano(), 10) + ".sock"
}
