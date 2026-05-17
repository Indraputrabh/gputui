package config

// Agent holds runtime settings for gputui-agent.
type Agent struct {
	CollectIntervalSec int    `json:"collect_interval_sec"`
	SocketPath         string `json:"socket_path"`
	RecordPath         string `json:"record_path,omitempty"`
}

// Client holds runtime settings for gputui client.
type Client struct {
	AgentAddr  string `json:"agent_addr"`
	RefreshSec int    `json:"refresh_sec"`
	NoColor    bool   `json:"no_color"`
}
