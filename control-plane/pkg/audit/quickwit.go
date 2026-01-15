// Package audit provides audit logging with Quickwit integration.
package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
)

// QuickwitClient provides HTTP client for Quickwit
type QuickwitClient struct {
	baseURL    string
	httpClient *http.Client
	logger     *zap.Logger
	indexID    string
}

// QuickwitConfig represents Quickwit client configuration
type QuickwitConfig struct {
	BaseURL     string        `json:"base_url" yaml:"base_url"`
	IndexID     string        `json:"index_id" yaml:"index_id"`
	Timeout     time.Duration `json:"timeout" yaml:"timeout"`
	MaxRetries  int           `json:"max_retries" yaml:"max_retries"`
	EnableBatch bool          `json:"enable_batch" yaml:"enable_batch"`
	BatchSize   int           `json:"batch_size" yaml:"batch_size"`
	FlushInterval time.Duration `json:"flush_interval" yaml:"flush_interval"`
}

// DefaultQuickwitConfig returns default Quickwit configuration
func DefaultQuickwitConfig() *QuickwitConfig {
	return &QuickwitConfig{
		BaseURL:       "http://localhost:7280",
		IndexID:       "audit-logs",
		Timeout:       30 * time.Second,
		MaxRetries:    3,
		EnableBatch:   true,
		BatchSize:     100,
		FlushInterval: 5 * time.Second,
	}
}

// NewQuickwitClient creates a new Quickwit client
func NewQuickwitClient(config *QuickwitConfig, logger *zap.Logger) *QuickwitClient {
	return &QuickwitClient{
		baseURL: strings.TrimSuffix(config.BaseURL, "/"),
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		logger:  logger,
		indexID: config.IndexID,
	}
}

// CreateIndex creates the audit log index
func (c *QuickwitClient) CreateIndex(ctx context.Context, config *QuickwitIndexConfig) error {
	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal index config: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/v1/indexes", c.baseURL),
		bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		c.logger.Info("index already exists", zap.String("index_id", config.IndexID))
		return nil
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create index: status=%d body=%s", resp.StatusCode, string(body))
	}

	c.logger.Info("index created", zap.String("index_id", config.IndexID))
	return nil
}

// DeleteIndex deletes an index
func (c *QuickwitClient) DeleteIndex(ctx context.Context, indexID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("%s/api/v1/indexes/%s", c.baseURL, indexID),
		nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete index: status=%d body=%s", resp.StatusCode, string(body))
	}

	return nil
}

// IndexExists checks if an index exists
func (c *QuickwitClient) IndexExists(ctx context.Context, indexID string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/v1/indexes/%s", c.baseURL, indexID),
		nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to check index: %w", err)
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// Ingest ingests documents into the index
func (c *QuickwitClient) Ingest(ctx context.Context, events []AuditEvent) error {
	if len(events) == 0 {
		return nil
	}

	// Convert events to NDJSON format
	var buffer bytes.Buffer
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			c.logger.Error("failed to marshal event", zap.Error(err), zap.String("event_id", event.ID))
			continue
		}
		buffer.Write(data)
		buffer.WriteByte('\n')
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/v1/%s/ingest", c.baseURL, c.indexID),
		&buffer)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-ndjson")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to ingest events: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to ingest events: status=%d body=%s", resp.StatusCode, string(body))
	}

	return nil
}

// IngestSingle ingests a single event
func (c *QuickwitClient) IngestSingle(ctx context.Context, event *AuditEvent) error {
	return c.Ingest(ctx, []AuditEvent{*event})
}

// Search searches the audit log index
func (c *QuickwitClient) Search(ctx context.Context, query *SearchQuery) (*SearchResult, error) {
	// Build query string
	queryStr := c.buildQueryString(query)

	searchReq := map[string]interface{}{
		"query":        queryStr,
		"max_hits":     query.MaxHits,
		"start_offset": query.StartOffset,
	}

	if len(query.SortBy) > 0 {
		sortFields := make([]string, len(query.SortBy))
		for i, sf := range query.SortBy {
			sortFields[i] = fmt.Sprintf("%s:%s", sf.Field, sf.Order)
		}
		searchReq["sort_by"] = sortFields
	}

	// Add time range
	if query.StartTime != nil {
		searchReq["start_timestamp"] = query.StartTime.Unix()
	}
	if query.EndTime != nil {
		searchReq["end_timestamp"] = query.EndTime.Unix()
	}

	data, err := json.Marshal(searchReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal search request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/v1/%s/search", c.baseURL, c.indexID),
		bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	var searchResp struct {
		Hits        []json.RawMessage `json:"hits"`
		NumHits     int64             `json:"num_hits"`
		ElapsedSecs float64           `json:"elapsed_secs"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	result := &SearchResult{
		NumHits:     searchResp.NumHits,
		ElapsedSecs: searchResp.ElapsedSecs,
		Hits:        make([]AuditEvent, 0, len(searchResp.Hits)),
	}

	for _, hit := range searchResp.Hits {
		var event AuditEvent
		if err := json.Unmarshal(hit, &event); err != nil {
			c.logger.Error("failed to unmarshal hit", zap.Error(err))
			continue
		}
		result.Hits = append(result.Hits, event)
	}

	return result, nil
}

// buildQueryString builds the Quickwit query string from SearchQuery
func (c *QuickwitClient) buildQueryString(query *SearchQuery) string {
	var parts []string

	// Add tenant filter (required for multi-tenant isolation)
	if query.TenantID != "" {
		parts = append(parts, fmt.Sprintf("tenant_id:%s", query.TenantID))
	}

	// Add event type filter
	if len(query.EventTypes) > 0 {
		types := make([]string, len(query.EventTypes))
		for i, t := range query.EventTypes {
			types[i] = string(t)
		}
		parts = append(parts, fmt.Sprintf("event_type:(%s)", strings.Join(types, " OR ")))
	}

	// Add action filter
	if len(query.Actions) > 0 {
		actions := make([]string, len(query.Actions))
		for i, a := range query.Actions {
			actions[i] = string(a)
		}
		parts = append(parts, fmt.Sprintf("action:(%s)", strings.Join(actions, " OR ")))
	}

	// Add outcome filter
	if len(query.Outcomes) > 0 {
		outcomes := make([]string, len(query.Outcomes))
		for i, o := range query.Outcomes {
			outcomes[i] = string(o)
		}
		parts = append(parts, fmt.Sprintf("outcome:(%s)", strings.Join(outcomes, " OR ")))
	}

	// Add actor filter
	if query.ActorID != "" {
		parts = append(parts, fmt.Sprintf("actor_id:%s", query.ActorID))
	}

	// Add resource filter
	if query.ResourceID != "" {
		parts = append(parts, fmt.Sprintf("resource_id:%s", query.ResourceID))
	}

	// Add free-text query
	if query.Query != "" {
		parts = append(parts, query.Query)
	}

	if len(parts) == 0 {
		return "*"
	}

	return strings.Join(parts, " AND ")
}

// Aggregate performs aggregation queries
func (c *QuickwitClient) Aggregate(ctx context.Context, tenantID string, field string, startTime, endTime *time.Time) (map[string]int64, error) {
	queryStr := fmt.Sprintf("tenant_id:%s", tenantID)

	aggReq := map[string]interface{}{
		"query":    queryStr,
		"max_hits": 0,
		"aggs": map[string]interface{}{
			"counts": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": field,
				},
			},
		},
	}

	if startTime != nil {
		aggReq["start_timestamp"] = startTime.Unix()
	}
	if endTime != nil {
		aggReq["end_timestamp"] = endTime.Unix()
	}

	data, err := json.Marshal(aggReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal aggregation request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/v1/%s/search", c.baseURL, c.indexID),
		bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("aggregation failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	var aggResp struct {
		Aggregations struct {
			Counts struct {
				Buckets []struct {
					Key      string `json:"key"`
					DocCount int64  `json:"doc_count"`
				} `json:"buckets"`
			} `json:"counts"`
		} `json:"aggregations"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&aggResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	result := make(map[string]int64)
	for _, bucket := range aggResp.Aggregations.Counts.Buckets {
		result[bucket.Key] = bucket.DocCount
	}

	return result, nil
}

// HealthCheck performs a health check on Quickwit
func (c *QuickwitClient) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/health/readyz", c.baseURL), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("quickwit unhealthy: status=%d", resp.StatusCode)
	}

	return nil
}

// GetMetrics returns Quickwit metrics for the index
func (c *QuickwitClient) GetMetrics(ctx context.Context) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/v1/indexes/%s/describe", c.baseURL, c.indexID), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get metrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get metrics: status=%d body=%s", resp.StatusCode, string(body))
	}

	var metrics map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return metrics, nil
}

// BuildSearchURL builds a search URL for external use
func (c *QuickwitClient) BuildSearchURL(query *SearchQuery) string {
	params := url.Values{}
	params.Set("query", c.buildQueryString(query))
	if query.MaxHits > 0 {
		params.Set("max_hits", fmt.Sprintf("%d", query.MaxHits))
	}
	return fmt.Sprintf("%s/api/v1/%s/search?%s", c.baseURL, c.indexID, params.Encode())
}
