// Package webhook provides HTTP webhook server functionality.
package webhook

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// WorkflowExecutor executes workflows
type WorkflowExecutor interface {
	Execute(workflow []byte) (string, error)
	GetStatus(workflowID string) (*WorkflowStatus, error)
	Cancel(workflowID string) error
}

// WorkflowStatus represents workflow execution status
type WorkflowStatus struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	Result    string    `json:"result,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// HealthChecker provides health status
type HealthChecker interface {
	IsHealthy() bool
	IsReady() bool
	GetStatus() any
}

// ConfigProvider provides configuration
type ConfigProvider interface {
	GetConfig() any
	UpdateConfig(config []byte) error
}

// UpgradeHandler handles agent upgrades
type UpgradeHandler interface {
	StartUpgrade(version string, downloadURL string, checksum string) error
	GetUpgradeStatus() *UpgradeStatus
}

// UpgradeStatus represents upgrade status
type UpgradeStatus struct {
	InProgress bool      `json:"in_progress"`
	Version    string    `json:"version,omitempty"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	Status     string    `json:"status,omitempty"`
	Error      string    `json:"error,omitempty"`
}

// Handlers contains all webhook handlers
type Handlers struct {
	mu              sync.RWMutex
	logger          *zap.Logger
	workflowExec    WorkflowExecutor
	healthChecker   HealthChecker
	configProvider  ConfigProvider
	upgradeHandler  UpgradeHandler
	hooks           map[string]HookHandler
}

// HookHandler handles a specific webhook
type HookHandler func(r *http.Request) (any, error)

// NewHandlers creates new webhook handlers
func NewHandlers(
	logger *zap.Logger,
	workflowExec WorkflowExecutor,
	healthChecker HealthChecker,
	configProvider ConfigProvider,
	upgradeHandler UpgradeHandler,
) *Handlers {
	return &Handlers{
		logger:         logger,
		workflowExec:   workflowExec,
		healthChecker:  healthChecker,
		configProvider: configProvider,
		upgradeHandler: upgradeHandler,
		hooks:          make(map[string]HookHandler),
	}
}

// RegisterHook registers a hook handler
func (h *Handlers) RegisterHook(name string, handler HookHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hooks[name] = handler
}

// HealthzHandler handles liveness probe
func (h *Handlers) HealthzHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.healthChecker != nil && !h.healthChecker.IsHealthy() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "unhealthy"})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

// ReadyzHandler handles readiness probe
func (h *Handlers) ReadyzHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.healthChecker != nil && !h.healthChecker.IsReady() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "not ready"})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

// StatusHandler handles status requests
func (h *Handlers) StatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.healthChecker == nil {
		http.Error(w, "Health checker not configured", http.StatusServiceUnavailable)
		return
	}

	status := h.healthChecker.GetStatus()
	json.NewEncoder(w).Encode(status)
}

// WebhookHandler handles webhook requests
func (h *Handlers) WebhookHandler(w http.ResponseWriter, r *http.Request) {
	// Extract hook name from path
	path := strings.TrimPrefix(r.URL.Path, "/hooks/")
	hookName := strings.Split(path, "/")[0]

	h.mu.RLock()
	handler, ok := h.hooks[hookName]
	h.mu.RUnlock()

	if !ok {
		http.Error(w, "Hook not found", http.StatusNotFound)
		return
	}

	result, err := handler(r)
	if err != nil {
		h.logger.Error("hook execution failed",
			zap.String("hook", hookName),
			zap.Error(err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// ExecuteWorkflowHandler handles workflow execution requests
func (h *Handlers) ExecuteWorkflowHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.workflowExec == nil {
		http.Error(w, "Workflow executor not configured", http.StatusServiceUnavailable)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	workflowID, err := h.workflowExec.Execute(body)
	if err != nil {
		h.logger.Error("workflow execution failed", zap.Error(err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"workflow_id": workflowID,
		"status":      "accepted",
	})
}

// WorkflowStatusHandler handles workflow status requests
func (h *Handlers) WorkflowStatusHandler(w http.ResponseWriter, r *http.Request) {
	if h.workflowExec == nil {
		http.Error(w, "Workflow executor not configured", http.StatusServiceUnavailable)
		return
	}

	workflowID := r.URL.Query().Get("id")
	if workflowID == "" {
		http.Error(w, "Missing workflow ID", http.StatusBadRequest)
		return
	}

	status, err := h.workflowExec.GetStatus(workflowID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// CancelWorkflowHandler handles workflow cancellation requests
func (h *Handlers) CancelWorkflowHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.workflowExec == nil {
		http.Error(w, "Workflow executor not configured", http.StatusServiceUnavailable)
		return
	}

	workflowID := r.URL.Query().Get("id")
	if workflowID == "" {
		http.Error(w, "Missing workflow ID", http.StatusBadRequest)
		return
	}

	if err := h.workflowExec.Cancel(workflowID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"workflow_id": workflowID,
		"status":      "cancelled",
	})
}

// ConfigHandler handles configuration requests
func (h *Handlers) ConfigHandler(w http.ResponseWriter, r *http.Request) {
	if h.configProvider == nil {
		http.Error(w, "Config provider not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(h.configProvider.GetConfig())

	case http.MethodPut, http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		if err := h.configProvider.UpdateConfig(body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "updated"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// UpgradeHandler handles upgrade requests
func (h *Handlers) UpgradeHandler(w http.ResponseWriter, r *http.Request) {
	if h.upgradeHandler == nil {
		http.Error(w, "Upgrade handler not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		status := h.upgradeHandler.GetUpgradeStatus()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)

	case http.MethodPost:
		var req struct {
			Version     string `json:"version"`
			DownloadURL string `json:"download_url"`
			Checksum    string `json:"checksum"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if err := h.upgradeHandler.StartUpgrade(req.Version, req.DownloadURL, req.Checksum); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "upgrade_started",
			"version": req.Version,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
