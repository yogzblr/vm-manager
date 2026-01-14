// Package config handles configuration loading and management for the vm-agent.
package config

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Source represents a configuration source
type Source int

const (
	// SourceDefault represents default values
	SourceDefault Source = iota
	// SourceFile represents configuration from file
	SourceFile
	// SourceEnv represents configuration from environment
	SourceEnv
	// SourceRemote represents configuration from control plane
	SourceRemote
	// SourceCLI represents configuration from command line
	SourceCLI
)

// String returns the string representation of a Source
func (s Source) String() string {
	switch s {
	case SourceDefault:
		return "default"
	case SourceFile:
		return "file"
	case SourceEnv:
		return "environment"
	case SourceRemote:
		return "remote"
	case SourceCLI:
		return "cli"
	default:
		return "unknown"
	}
}

// Priority returns the priority of a source (higher is more authoritative)
func (s Source) Priority() int {
	switch s {
	case SourceDefault:
		return 0
	case SourceFile:
		return 10
	case SourceEnv:
		return 20
	case SourceRemote:
		return 30
	case SourceCLI:
		return 40
	default:
		return -1
	}
}

// RemoteConfig represents configuration fetched from the control plane
type RemoteConfig struct {
	Version   string          `json:"version"`
	UpdatedAt time.Time       `json:"updated_at"`
	Config    json.RawMessage `json:"config"`
}

// RemoteConfigFetcher fetches configuration from the control plane
type RemoteConfigFetcher struct {
	controlPlaneURL string
	token           string
	agentID         string
	httpClient      *http.Client
}

// NewRemoteConfigFetcher creates a new remote config fetcher
func NewRemoteConfigFetcher(controlPlaneURL, token, agentID string) *RemoteConfigFetcher {
	return &RemoteConfigFetcher{
		controlPlaneURL: controlPlaneURL,
		token:           token,
		agentID:         agentID,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Fetch retrieves configuration from the control plane
func (f *RemoteConfigFetcher) Fetch() (*RemoteConfig, error) {
	if f.controlPlaneURL == "" {
		return nil, fmt.Errorf("control plane URL not configured")
	}

	url := fmt.Sprintf("%s/api/v1/agents/%s/config", f.controlPlaneURL, f.agentID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+f.token)
	req.Header.Set("Accept", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var cfg RemoteConfig
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &cfg, nil
}

// PriorityResolver resolves configuration values based on source priority
type PriorityResolver struct {
	sources map[string]Source
}

// NewPriorityResolver creates a new priority resolver
func NewPriorityResolver() *PriorityResolver {
	return &PriorityResolver{
		sources: make(map[string]Source),
	}
}

// SetSource records the source of a configuration value
func (r *PriorityResolver) SetSource(key string, source Source) {
	r.sources[key] = source
}

// GetSource returns the source of a configuration value
func (r *PriorityResolver) GetSource(key string) Source {
	if source, ok := r.sources[key]; ok {
		return source
	}
	return SourceDefault
}

// ShouldOverride returns true if newSource should override the current source for key
func (r *PriorityResolver) ShouldOverride(key string, newSource Source) bool {
	currentSource := r.GetSource(key)
	return newSource.Priority() >= currentSource.Priority()
}

// MergeConfigs merges multiple configurations based on priority
func MergeConfigs(base *Config, overlay *Config, overlaySource Source, resolver *PriorityResolver) *Config {
	result := *base

	// Merge agent config
	if overlay.Agent.ID != "" && resolver.ShouldOverride("agent.id", overlaySource) {
		result.Agent.ID = overlay.Agent.ID
		resolver.SetSource("agent.id", overlaySource)
	}
	if overlay.Agent.TenantID != "" && resolver.ShouldOverride("agent.tenant_id", overlaySource) {
		result.Agent.TenantID = overlay.Agent.TenantID
		resolver.SetSource("agent.tenant_id", overlaySource)
	}
	if overlay.Agent.ControlPlaneURL != "" && resolver.ShouldOverride("agent.control_plane_url", overlaySource) {
		result.Agent.ControlPlaneURL = overlay.Agent.ControlPlaneURL
		resolver.SetSource("agent.control_plane_url", overlaySource)
	}
	if overlay.Agent.Token != "" && resolver.ShouldOverride("agent.token", overlaySource) {
		result.Agent.Token = overlay.Agent.Token
		resolver.SetSource("agent.token", overlaySource)
	}

	// Merge piko config
	if overlay.Piko.ServerURL != "" && resolver.ShouldOverride("piko.server_url", overlaySource) {
		result.Piko.ServerURL = overlay.Piko.ServerURL
		resolver.SetSource("piko.server_url", overlaySource)
	}
	if overlay.Piko.Endpoint != "" && resolver.ShouldOverride("piko.endpoint", overlaySource) {
		result.Piko.Endpoint = overlay.Piko.Endpoint
		resolver.SetSource("piko.endpoint", overlaySource)
	}

	// Merge webhook config
	if overlay.Webhook.Port != 0 && resolver.ShouldOverride("webhook.port", overlaySource) {
		result.Webhook.Port = overlay.Webhook.Port
		resolver.SetSource("webhook.port", overlaySource)
	}

	// Merge probe config
	if overlay.Probe.MaxConcurrent != 0 && resolver.ShouldOverride("probe.max_concurrent", overlaySource) {
		result.Probe.MaxConcurrent = overlay.Probe.MaxConcurrent
		resolver.SetSource("probe.max_concurrent", overlaySource)
	}

	return &result
}
