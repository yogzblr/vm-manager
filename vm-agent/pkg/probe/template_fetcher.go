// Package probe provides workflow execution functionality.
package probe

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// TemplateFetcher fetches templates from various sources
type TemplateFetcher struct {
	httpClient       *http.Client
	controlPlaneURL  string
	controlPlaneAuth string
}

// TemplateFetcherConfig contains configuration for the template fetcher
type TemplateFetcherConfig struct {
	// HTTPTimeout is the timeout for HTTP requests
	HTTPTimeout time.Duration
	// ControlPlaneURL is the base URL of the control plane API
	ControlPlaneURL string
	// ControlPlaneAuth is the authentication token for control plane
	ControlPlaneAuth string
}

// NewTemplateFetcher creates a new template fetcher
func NewTemplateFetcher(cfg *TemplateFetcherConfig) *TemplateFetcher {
	timeout := cfg.HTTPTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &TemplateFetcher{
		httpClient: &http.Client{
			Timeout: timeout,
		},
		controlPlaneURL:  cfg.ControlPlaneURL,
		controlPlaneAuth: cfg.ControlPlaneAuth,
	}
}

// FetchResult contains the result of fetching a template
type FetchResult struct {
	Content     string
	Source      string
	ContentType string
	ETag        string
}

// Fetch fetches a template from the given source
// Supported source formats:
//   - http:// or https:// - fetch from HTTP URL
//   - control-plane://templates/{id} - fetch from control plane
//   - control-plane://templates/{id}/content - fetch raw content from control plane
func (f *TemplateFetcher) Fetch(ctx context.Context, source string) (*FetchResult, error) {
	switch {
	case strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://"):
		return f.fetchHTTP(ctx, source)
	case strings.HasPrefix(source, "control-plane://"):
		return f.fetchControlPlane(ctx, source)
	default:
		return nil, fmt.Errorf("unsupported template source: %s", source)
	}
}

// fetchHTTP fetches a template from an HTTP URL
func (f *TemplateFetcher) fetchHTTP(ctx context.Context, url string) (*FetchResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch template: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch template: HTTP %d", resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return &FetchResult{
		Content:     string(content),
		Source:      url,
		ContentType: resp.Header.Get("Content-Type"),
		ETag:        resp.Header.Get("ETag"),
	}, nil
}

// fetchControlPlane fetches a template from the control plane
func (f *TemplateFetcher) fetchControlPlane(ctx context.Context, source string) (*FetchResult, error) {
	if f.controlPlaneURL == "" {
		return nil, fmt.Errorf("control plane URL not configured")
	}

	// Parse the control-plane:// URL
	// Format: control-plane://templates/{id} or control-plane://templates/{id}/content
	path := strings.TrimPrefix(source, "control-plane://")

	// Build the full URL
	url := fmt.Sprintf("%s/api/v1/%s", strings.TrimSuffix(f.controlPlaneURL, "/"), path)

	// Ensure we're fetching the content endpoint
	if !strings.HasSuffix(url, "/content") {
		url = url + "/content"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication header
	if f.controlPlaneAuth != "" {
		req.Header.Set("Authorization", "Bearer "+f.controlPlaneAuth)
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch template from control plane: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("template not found: %s", source)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch template from control plane: HTTP %d", resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return &FetchResult{
		Content:     string(content),
		Source:      source,
		ContentType: resp.Header.Get("Content-Type"),
		ETag:        resp.Header.Get("ETag"),
	}, nil
}

// SetControlPlaneConfig updates the control plane configuration
func (f *TemplateFetcher) SetControlPlaneConfig(url, auth string) {
	f.controlPlaneURL = url
	f.controlPlaneAuth = auth
}
