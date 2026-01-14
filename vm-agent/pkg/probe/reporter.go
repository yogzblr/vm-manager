// Package probe provides workflow execution functionality.
package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Reporter reports workflow results to the control plane
type Reporter struct {
	mu         sync.Mutex
	reportURL  string
	token      string
	httpClient *http.Client
	logger     *zap.Logger
	queue      chan *WorkflowResult
	wg         sync.WaitGroup
	stopCh     chan struct{}
}

// ReporterConfig contains reporter configuration
type ReporterConfig struct {
	ReportURL   string
	Token       string
	QueueSize   int
	MaxRetries  int
	RetryDelay  time.Duration
}

// NewReporter creates a new workflow result reporter
func NewReporter(cfg *ReporterConfig, logger *zap.Logger) *Reporter {
	queueSize := cfg.QueueSize
	if queueSize <= 0 {
		queueSize = 100
	}

	return &Reporter{
		reportURL: cfg.ReportURL,
		token:     cfg.Token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
		queue:  make(chan *WorkflowResult, queueSize),
		stopCh: make(chan struct{}),
	}
}

// Start starts the reporter
func (r *Reporter) Start(ctx context.Context) {
	r.wg.Add(1)
	go r.processQueue(ctx)
}

// Stop stops the reporter
func (r *Reporter) Stop() {
	close(r.stopCh)
	r.wg.Wait()
}

// Report queues a workflow result for reporting
func (r *Reporter) Report(result *WorkflowResult) {
	select {
	case r.queue <- result:
	default:
		r.logger.Warn("report queue full, dropping result",
			zap.String("workflow_id", result.WorkflowID))
	}
}

// processQueue processes the report queue
func (r *Reporter) processQueue(ctx context.Context) {
	defer r.wg.Done()

	for {
		select {
		case <-ctx.Done():
			r.flushQueue()
			return
		case <-r.stopCh:
			r.flushQueue()
			return
		case result := <-r.queue:
			r.sendReport(ctx, result)
		}
	}
}

// flushQueue sends all remaining reports in the queue
func (r *Reporter) flushQueue() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for {
		select {
		case result := <-r.queue:
			r.sendReport(ctx, result)
		default:
			return
		}
	}
}

// sendReport sends a single report
func (r *Reporter) sendReport(ctx context.Context, result *WorkflowResult) {
	if r.reportURL == "" {
		return
	}

	payload, err := json.Marshal(result)
	if err != nil {
		r.logger.Error("failed to marshal workflow result",
			zap.String("workflow_id", result.WorkflowID),
			zap.Error(err))
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.reportURL, bytes.NewReader(payload))
	if err != nil {
		r.logger.Error("failed to create report request",
			zap.String("workflow_id", result.WorkflowID),
			zap.Error(err))
		return
	}

	req.Header.Set("Content-Type", "application/json")
	if r.token != "" {
		req.Header.Set("Authorization", "Bearer "+r.token)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		r.logger.Error("failed to send workflow report",
			zap.String("workflow_id", result.WorkflowID),
			zap.Error(err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		r.logger.Error("workflow report rejected",
			zap.String("workflow_id", result.WorkflowID),
			zap.Int("status_code", resp.StatusCode))
		return
	}

	r.logger.Debug("workflow report sent",
		zap.String("workflow_id", result.WorkflowID),
		zap.String("status", string(result.Status)))
}

// ReportSync sends a report synchronously and returns any error
func (r *Reporter) ReportSync(ctx context.Context, result *WorkflowResult) error {
	if r.reportURL == "" {
		return nil
	}

	payload, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.reportURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if r.token != "" {
		req.Header.Set("Authorization", "Bearer "+r.token)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send report: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("report rejected with status %d", resp.StatusCode)
	}

	return nil
}

// ResultAggregator aggregates workflow results
type ResultAggregator struct {
	mu      sync.RWMutex
	results map[string]*WorkflowResult
	maxSize int
}

// NewResultAggregator creates a new result aggregator
func NewResultAggregator(maxSize int) *ResultAggregator {
	if maxSize <= 0 {
		maxSize = 1000
	}
	return &ResultAggregator{
		results: make(map[string]*WorkflowResult),
		maxSize: maxSize,
	}
}

// Add adds a result to the aggregator
func (a *ResultAggregator) Add(result *WorkflowResult) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Remove oldest if at capacity
	if len(a.results) >= a.maxSize {
		var oldest string
		var oldestTime time.Time
		for id, r := range a.results {
			if oldest == "" || r.EndedAt.Before(oldestTime) {
				oldest = id
				oldestTime = r.EndedAt
			}
		}
		delete(a.results, oldest)
	}

	a.results[result.WorkflowID] = result
}

// Get returns a result by workflow ID
func (a *ResultAggregator) Get(workflowID string) (*WorkflowResult, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result, ok := a.results[workflowID]
	return result, ok
}

// List returns all results
func (a *ResultAggregator) List() []*WorkflowResult {
	a.mu.RLock()
	defer a.mu.RUnlock()

	results := make([]*WorkflowResult, 0, len(a.results))
	for _, r := range a.results {
		results = append(results, r)
	}
	return results
}

// Stats returns aggregated statistics
func (a *ResultAggregator) Stats() map[string]int {
	a.mu.RLock()
	defer a.mu.RUnlock()

	stats := map[string]int{
		"total":     len(a.results),
		"success":   0,
		"failed":    0,
		"cancelled": 0,
	}

	for _, r := range a.results {
		switch r.Status {
		case StepStatusSuccess:
			stats["success"]++
		case StepStatusFailed:
			stats["failed"]++
		case StepStatusCancelled:
			stats["cancelled"]++
		}
	}

	return stats
}
