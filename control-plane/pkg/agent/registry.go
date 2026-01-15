// Package agent provides agent management for the control plane.
package agent

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/yourorg/control-plane/pkg/db/models"
)

// Registry manages agent records
type Registry struct {
	db     *gorm.DB
	logger *zap.Logger
}

// NewRegistry creates a new agent registry
func NewRegistry(db *gorm.DB, logger *zap.Logger) *Registry {
	return &Registry{
		db:     db,
		logger: logger,
	}
}

// Get retrieves an agent by ID
func (r *Registry) Get(ctx context.Context, tenantID, agentID string) (*models.Agent, error) {
	var agent models.Agent
	if err := r.db.Where("id = ? AND tenant_id = ?", agentID, tenantID).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("agent not found")
		}
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}
	return &agent, nil
}

// ListRequest represents a request to list agents
type ListRequest struct {
	TenantID string
	Status   string
	Tags     map[string]string
	Limit    int
	Offset   int
}

// List lists agents
func (r *Registry) List(ctx context.Context, req *ListRequest) ([]models.Agent, int64, error) {
	query := r.db.Model(&models.Agent{})

	if req.TenantID != "" {
		query = query.Where("tenant_id = ?", req.TenantID)
	}

	if req.Status != "" {
		query = query.Where("status = ?", req.Status)
	}

	// Filter by tags (JSON query)
	for key, value := range req.Tags {
		query = query.Where("JSON_EXTRACT(tags, ?) = ?", "$."+key, value)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count agents: %w", err)
	}

	if req.Limit > 0 {
		query = query.Limit(req.Limit)
	}
	if req.Offset > 0 {
		query = query.Offset(req.Offset)
	}

	var agents []models.Agent
	if err := query.Order("registered_at DESC").Find(&agents).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list agents: %w", err)
	}

	return agents, total, nil
}

// UpdateStatus updates an agent's status
func (r *Registry) UpdateStatus(ctx context.Context, tenantID, agentID string, status models.AgentStatus) error {
	result := r.db.Model(&models.Agent{}).
		Where("id = ? AND tenant_id = ?", agentID, tenantID).
		Updates(map[string]interface{}{
			"status":       status,
			"last_seen_at": time.Now(),
			"updated_at":   time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update agent status: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("agent not found")
	}

	return nil
}

// UpdateHeartbeat updates the agent's last seen timestamp
func (r *Registry) UpdateHeartbeat(ctx context.Context, tenantID, agentID string) error {
	result := r.db.Model(&models.Agent{}).
		Where("id = ? AND tenant_id = ?", agentID, tenantID).
		Updates(map[string]interface{}{
			"last_seen_at": time.Now(),
			"status":       models.AgentStatusOnline,
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update heartbeat: %w", result.Error)
	}

	return nil
}

// RecordHealthReport records a health report from an agent
func (r *Registry) RecordHealthReport(ctx context.Context, tenantID, agentID string, status models.AgentStatus, components map[string]interface{}) error {
	report := &models.AgentHealthReport{
		ID:         fmt.Sprintf("%s-%d", agentID, time.Now().UnixNano()),
		AgentID:    agentID,
		TenantID:   tenantID,
		Status:     status,
		Components: components,
		ReportedAt: time.Now(),
	}

	if err := r.db.Create(report).Error; err != nil {
		return fmt.Errorf("failed to record health report: %w", err)
	}

	// Update agent status
	return r.UpdateStatus(ctx, tenantID, agentID, status)
}

// GetOfflineAgents returns agents that haven't reported in recently
func (r *Registry) GetOfflineAgents(ctx context.Context, tenantID string, threshold time.Duration) ([]models.Agent, error) {
	cutoff := time.Now().Add(-threshold)

	var agents []models.Agent
	query := r.db.Model(&models.Agent{}).
		Where("tenant_id = ? AND status = ? AND last_seen_at < ?", tenantID, models.AgentStatusOnline, cutoff)

	if err := query.Find(&agents).Error; err != nil {
		return nil, fmt.Errorf("failed to get offline agents: %w", err)
	}

	return agents, nil
}

// MarkOfflineAgents marks agents as offline if they haven't reported recently
func (r *Registry) MarkOfflineAgents(ctx context.Context, threshold time.Duration) (int64, error) {
	cutoff := time.Now().Add(-threshold)

	result := r.db.Model(&models.Agent{}).
		Where("status = ? AND last_seen_at < ?", models.AgentStatusOnline, cutoff).
		Update("status", models.AgentStatusOffline)

	if result.Error != nil {
		return 0, fmt.Errorf("failed to mark offline agents: %w", result.Error)
	}

	if result.RowsAffected > 0 {
		r.logger.Info("marked agents as offline",
			zap.Int64("count", result.RowsAffected))
	}

	return result.RowsAffected, nil
}

// UpdateAgent updates agent information
func (r *Registry) UpdateAgent(ctx context.Context, tenantID, agentID string, updates map[string]interface{}) error {
	updates["updated_at"] = time.Now()

	result := r.db.Model(&models.Agent{}).
		Where("id = ? AND tenant_id = ?", agentID, tenantID).
		Updates(updates)

	if result.Error != nil {
		return fmt.Errorf("failed to update agent: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("agent not found")
	}

	return nil
}

// GetAgentCount returns the count of agents by status
func (r *Registry) GetAgentCount(ctx context.Context, tenantID string) (map[string]int64, error) {
	counts := make(map[string]int64)

	type result struct {
		Status string
		Count  int64
	}

	var results []result
	query := r.db.Model(&models.Agent{}).
		Select("status, count(*) as count").
		Where("tenant_id = ?", tenantID).
		Group("status")

	if err := query.Find(&results).Error; err != nil {
		return nil, fmt.Errorf("failed to get agent counts: %w", err)
	}

	for _, r := range results {
		counts[r.Status] = r.Count
	}

	return counts, nil
}
