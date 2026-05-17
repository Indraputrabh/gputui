package unixsock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/indraputrabh/gputui/internal/model"
)

// SnapshotProvider returns the most recent snapshot published by the agent.
type SnapshotProvider interface {
	LatestSnapshot() (model.Snapshot, bool)
}

// Server exposes health and snapshot endpoints over a unix socket.
type Server struct {
	socketPath string
	provider   SnapshotProvider

	listener net.Listener
	server   *http.Server
}

func NewServer(socketPath string, provider SnapshotProvider) (*Server, error) {
	if provider == nil {
		return nil, errors.New("provider must not be nil")
	}
	if socketPath == "" {
		return nil, errors.New("socket path must not be empty")
	}

	s := &Server{
		socketPath: socketPath,
		provider:   provider,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/v1/snapshot", s.handleSnapshot)
	s.server = &http.Server{Handler: mux}
	return s, nil
}

func (s *Server) Start() error {
	if err := os.MkdirAll(filepath.Dir(s.socketPath), 0o755); err != nil {
		return fmt.Errorf("create socket parent dir: %w", err)
	}
	if err := os.Remove(s.socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale socket: %w", err)
	}

	l, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listen on unix socket %s: %w", s.socketPath, err)
	}
	if err := os.Chmod(s.socketPath, 0o600); err != nil {
		_ = l.Close()
		return fmt.Errorf("set socket permissions: %w", err)
	}

	s.listener = l
	go func() {
		serveErr := s.server.Serve(l)
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			// Keep process running; failures can still be observed in API callers.
			fmt.Fprintf(os.Stderr, "gputui-agent api server stopped: %v\n", serveErr)
		}
	}()
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("http shutdown: %w", err)
	}
	if err := os.Remove(s.socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove socket: %w", err)
	}
	return nil
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleSnapshot(w http.ResponseWriter, _ *http.Request) {
	snapshot, ok := s.provider.LatestSnapshot()
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "snapshot not ready",
		})
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
