package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/indraputrabh/gputui/internal/model"
)

const defaultTimeout = 3 * time.Second

// Client fetches data from a gputui-agent unix-socket API.
type Client struct {
	httpClient *http.Client
}

// New creates a Client that connects to the agent at socketPath.
func New(socketPath string) *Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		},
	}
	return &Client{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   defaultTimeout,
		},
	}
}

// FetchSnapshot returns the latest snapshot from the agent.
func (c *Client) FetchSnapshot() (model.Snapshot, error) {
	resp, err := c.httpClient.Get("http://unix/v1/snapshot")
	if err != nil {
		return model.Snapshot{}, fmt.Errorf("get snapshot: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return model.Snapshot{}, fmt.Errorf("read snapshot body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return model.Snapshot{}, fmt.Errorf("snapshot endpoint returned %d: %s", resp.StatusCode, body)
	}

	var snap model.Snapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		return model.Snapshot{}, fmt.Errorf("decode snapshot: %w", err)
	}
	return snap, nil
}

// Healthz checks that the agent is responding.
func (c *Client) Healthz() error {
	resp, err := c.httpClient.Get("http://unix/healthz")
	if err != nil {
		return fmt.Errorf("healthz: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz returned %d", resp.StatusCode)
	}
	return nil
}

// Close releases transport resources.
func (c *Client) Close() {
	c.httpClient.CloseIdleConnections()
}
