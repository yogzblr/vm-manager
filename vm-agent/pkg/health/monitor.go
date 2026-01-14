// Package health provides health monitoring for the vm-agent.
package health

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ComponentStatus represents the status of a component
type ComponentStatus string

const (
	StatusHealthy   ComponentStatus = "healthy"
	StatusDegraded  ComponentStatus = "degraded"
	StatusUnhealthy ComponentStatus = "unhealthy"
	StatusUnknown   ComponentStatus = "unknown"
)

// Component represents a monitored component
type Component struct {
	Name        string          `json:"name"`
	Status      ComponentStatus `json:"status"`
	Message     string          `json:"message,omitempty"`
	LastChecked time.Time       `json:"last_checked"`
	Details     map[string]any  `json:"details,omitempty"`
}

// Status represents the overall health status
type Status struct {
	Overall     ComponentStatus       `json:"overall"`
	Components  map[string]*Component `json:"components"`
	AgentID     string                `json:"agent_id"`
	TenantID    string                `json:"tenant_id"`
	Version     string                `json:"version"`
	Uptime      time.Duration         `json:"uptime"`
	LastUpdated time.Time             `json:"last_updated"`
}

// Checker is the interface for health checks
type Checker interface {
	Name() string
	Check(ctx context.Context) *Component
}

// Monitor monitors the health of all components
type Monitor struct {
	mu            sync.RWMutex
	checkers      []Checker
	status        *Status
	checkInterval time.Duration
	logger        *zap.Logger
	startTime     time.Time
	agentID       string
	tenantID      string
	version       string
	stopCh        chan struct{}
	wg            sync.WaitGroup
}

// NewMonitor creates a new health monitor
func NewMonitor(agentID, tenantID, version string, checkInterval time.Duration, logger *zap.Logger) *Monitor {
	return &Monitor{
		checkers:      make([]Checker, 0),
		checkInterval: checkInterval,
		logger:        logger,
		startTime:     time.Now(),
		agentID:       agentID,
		tenantID:      tenantID,
		version:       version,
		stopCh:        make(chan struct{}),
		status: &Status{
			Overall:    StatusUnknown,
			Components: make(map[string]*Component),
			AgentID:    agentID,
			TenantID:   tenantID,
			Version:    version,
		},
	}
}

// RegisterChecker registers a health checker
func (m *Monitor) RegisterChecker(checker Checker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkers = append(m.checkers, checker)
}

// Start starts the health monitoring loop
func (m *Monitor) Start(ctx context.Context) {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.runChecks(ctx)

		ticker := time.NewTicker(m.checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-m.stopCh:
				return
			case <-ticker.C:
				m.runChecks(ctx)
			}
		}
	}()
}

// Stop stops the health monitor
func (m *Monitor) Stop() {
	close(m.stopCh)
	m.wg.Wait()
}

// runChecks runs all health checks
func (m *Monitor) runChecks(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	overallStatus := StatusHealthy

	for _, checker := range m.checkers {
		component := checker.Check(checkCtx)
		m.status.Components[checker.Name()] = component

		switch component.Status {
		case StatusUnhealthy:
			overallStatus = StatusUnhealthy
		case StatusDegraded:
			if overallStatus != StatusUnhealthy {
				overallStatus = StatusDegraded
			}
		}

		m.logger.Debug("health check completed",
			zap.String("component", checker.Name()),
			zap.String("status", string(component.Status)),
			zap.String("message", component.Message))
	}

	m.status.Overall = overallStatus
	m.status.Uptime = time.Since(m.startTime)
	m.status.LastUpdated = time.Now()
}

// GetStatus returns the current health status
func (m *Monitor) GetStatus() *Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy
	status := *m.status
	status.Components = make(map[string]*Component)
	for k, v := range m.status.Components {
		component := *v
		status.Components[k] = &component
	}
	status.Uptime = time.Since(m.startTime)

	return &status
}

// GetComponentStatus returns the status of a specific component
func (m *Monitor) GetComponentStatus(name string) *Component {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if component, ok := m.status.Components[name]; ok {
		c := *component
		return &c
	}
	return nil
}

// IsHealthy returns true if all components are healthy
func (m *Monitor) IsHealthy() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status.Overall == StatusHealthy
}

// IsReady returns true if the agent is ready to receive requests
func (m *Monitor) IsReady() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status.Overall == StatusHealthy || m.status.Overall == StatusDegraded
}
