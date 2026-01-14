// Package health provides health monitoring for the vm-agent.
package health

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

// Reporter reports health status to the control plane
type Reporter struct {
	mu             sync.RWMutex
	monitor        *Monitor
	reportURL      string
	token          string
	reportInterval time.Duration
	httpClient     *http.Client
	logger         *zap.Logger
	stopCh         chan struct{}
	wg             sync.WaitGroup
	lastReport     time.Time
	lastError      error
}

// NewReporter creates a new health reporter
func NewReporter(monitor *Monitor, reportURL, token string, reportInterval time.Duration, logger *zap.Logger) *Reporter {
	return &Reporter{
		monitor:        monitor,
		reportURL:      reportURL,
		token:          token,
		reportInterval: reportInterval,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
		stopCh: make(chan struct{}),
	}
}

// Start starts the health reporting loop
func (r *Reporter) Start(ctx context.Context) {
	if r.reportURL == "" {
		r.logger.Info("health reporting disabled (no report URL configured)")
		return
	}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()

		// Initial report
		r.report(ctx)

		ticker := time.NewTicker(r.reportInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-r.stopCh:
				return
			case <-ticker.C:
				r.report(ctx)
			}
		}
	}()
}

// Stop stops the health reporter
func (r *Reporter) Stop() {
	close(r.stopCh)
	r.wg.Wait()
}

// report sends a health report to the control plane
func (r *Reporter) report(ctx context.Context) {
	status := r.monitor.GetStatus()

	payload, err := json.Marshal(status)
	if err != nil {
		r.logger.Error("failed to marshal health status", zap.Error(err))
		r.setLastError(err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.reportURL, bytes.NewReader(payload))
	if err != nil {
		r.logger.Error("failed to create health report request", zap.Error(err))
		r.setLastError(err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.token)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		r.logger.Error("failed to send health report", zap.Error(err))
		r.setLastError(err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		err := fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		r.logger.Error("health report rejected", zap.Int("status_code", resp.StatusCode))
		r.setLastError(err)
		return
	}

	r.logger.Debug("health report sent successfully",
		zap.String("overall_status", string(status.Overall)))
	r.setLastReport()
}

// setLastReport records a successful report
func (r *Reporter) setLastReport() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastReport = time.Now()
	r.lastError = nil
}

// setLastError records a reporting error
func (r *Reporter) setLastError(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastError = err
}

// GetLastReport returns the time of the last successful report
func (r *Reporter) GetLastReport() time.Time {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lastReport
}

// GetLastError returns the last reporting error
func (r *Reporter) GetLastError() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lastError
}

// ForceReport triggers an immediate health report
func (r *Reporter) ForceReport(ctx context.Context) error {
	r.report(ctx)
	return r.GetLastError()
}

// HealthEndpointHandler returns an HTTP handler for health endpoints
type HealthEndpointHandler struct {
	monitor *Monitor
}

// NewHealthEndpointHandler creates a new health endpoint handler
func NewHealthEndpointHandler(monitor *Monitor) *HealthEndpointHandler {
	return &HealthEndpointHandler{monitor: monitor}
}

// LivenessHandler handles /healthz endpoint
func (h *HealthEndpointHandler) LivenessHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	status := h.monitor.GetStatus()

	if status.Overall == StatusUnhealthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	json.NewEncoder(w).Encode(map[string]any{
		"status": status.Overall,
		"uptime": status.Uptime.String(),
	})
}

// ReadinessHandler handles /readyz endpoint
func (h *HealthEndpointHandler) ReadinessHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.monitor.IsReady() {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "not ready"})
	}
}

// StatusHandler handles /status endpoint with detailed status
func (h *HealthEndpointHandler) StatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	status := h.monitor.GetStatus()
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(status)
}
