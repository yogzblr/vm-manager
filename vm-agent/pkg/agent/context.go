// Package agent provides the main agent manager.
package agent

import (
	"context"
	"sync"

	"github.com/yourorg/vm-agent/pkg/config"
	"go.uber.org/zap"
)

// contextKey is a custom type for context keys
type contextKey string

const (
	contextKeyAgentID  contextKey = "agent_id"
	contextKeyTenantID contextKey = "tenant_id"
	contextKeyLogger   contextKey = "logger"
	contextKeyConfig   contextKey = "config"
)

// SharedContext provides shared state across components
type SharedContext struct {
	mu       sync.RWMutex
	agentID  string
	tenantID string
	version  string
	config   *config.Config
	logger   *zap.Logger
	data     map[string]interface{}
}

// NewSharedContext creates a new shared context
func NewSharedContext(cfg *config.Config, logger *zap.Logger) *SharedContext {
	return &SharedContext{
		agentID:  cfg.Agent.ID,
		tenantID: cfg.Agent.TenantID,
		config:   cfg,
		logger:   logger,
		data:     make(map[string]interface{}),
	}
}

// AgentID returns the agent ID
func (sc *SharedContext) AgentID() string {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.agentID
}

// TenantID returns the tenant ID
func (sc *SharedContext) TenantID() string {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.tenantID
}

// Version returns the version
func (sc *SharedContext) Version() string {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.version
}

// SetVersion sets the version
func (sc *SharedContext) SetVersion(version string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.version = version
}

// Config returns the configuration
func (sc *SharedContext) Config() *config.Config {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.config
}

// UpdateConfig updates the configuration
func (sc *SharedContext) UpdateConfig(cfg *config.Config) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.config = cfg
}

// Logger returns the logger
func (sc *SharedContext) Logger() *zap.Logger {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.logger
}

// Set stores a value in the shared context
func (sc *SharedContext) Set(key string, value interface{}) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.data[key] = value
}

// Get retrieves a value from the shared context
func (sc *SharedContext) Get(key string) (interface{}, bool) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	value, ok := sc.data[key]
	return value, ok
}

// Delete removes a value from the shared context
func (sc *SharedContext) Delete(key string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	delete(sc.data, key)
}

// WithAgentContext returns a context with agent metadata
func WithAgentContext(ctx context.Context, sc *SharedContext) context.Context {
	ctx = context.WithValue(ctx, contextKeyAgentID, sc.AgentID())
	ctx = context.WithValue(ctx, contextKeyTenantID, sc.TenantID())
	ctx = context.WithValue(ctx, contextKeyLogger, sc.Logger())
	ctx = context.WithValue(ctx, contextKeyConfig, sc.Config())
	return ctx
}

// AgentIDFromContext extracts the agent ID from context
func AgentIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(contextKeyAgentID).(string); ok {
		return id
	}
	return ""
}

// TenantIDFromContext extracts the tenant ID from context
func TenantIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(contextKeyTenantID).(string); ok {
		return id
	}
	return ""
}

// LoggerFromContext extracts the logger from context
func LoggerFromContext(ctx context.Context) *zap.Logger {
	if logger, ok := ctx.Value(contextKeyLogger).(*zap.Logger); ok {
		return logger
	}
	return zap.NewNop()
}

// ConfigFromContext extracts the config from context
func ConfigFromContext(ctx context.Context) *config.Config {
	if cfg, ok := ctx.Value(contextKeyConfig).(*config.Config); ok {
		return cfg
	}
	return nil
}
