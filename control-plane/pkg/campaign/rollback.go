// Package campaign provides campaign management for the control plane.
package campaign

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/yourorg/control-plane/pkg/db/models"
)

// RollbackManager handles campaign rollbacks
type RollbackManager struct {
	db     *gorm.DB
	logger *zap.Logger
}

// NewRollbackManager creates a new rollback manager
func NewRollbackManager(db *gorm.DB, logger *zap.Logger) *RollbackManager {
	return &RollbackManager{
		db:     db,
		logger: logger,
	}
}

// RollbackConfig contains rollback configuration
type RollbackConfig struct {
	Threshold        float64 // Success rate threshold below which rollback is triggered
	AutoRollback     bool    // Whether to automatically trigger rollback
	RollbackWorkflow string  // Optional workflow to run for rollback
}

// InitiateRollback initiates a campaign rollback
func (m *RollbackManager) InitiateRollback(ctx context.Context, campaignID, reason string) error {
	var campaign models.Campaign
	if err := m.db.First(&campaign, "id = ?", campaignID).Error; err != nil {
		return fmt.Errorf("campaign not found: %w", err)
	}

	// Update campaign status
	now := time.Now()
	if err := m.db.Model(&campaign).Updates(map[string]interface{}{
		"status":     models.CampaignStatusRollingBack,
		"updated_at": now,
		"progress": map[string]interface{}{
			"rollback_reason":     reason,
			"rollback_started_at": now,
		},
	}).Error; err != nil {
		return fmt.Errorf("failed to initiate rollback: %w", err)
	}

	m.logger.Warn("campaign rollback initiated",
		zap.String("campaign_id", campaignID),
		zap.String("reason", reason))

	return nil
}

// ExecuteRollback executes rollback operations
func (m *RollbackManager) ExecuteRollback(ctx context.Context, campaignID string, config *RollbackConfig) error {
	// Get all successful executions that need to be rolled back
	var executions []models.WorkflowExecution
	if err := m.db.Where("campaign_id = ? AND status = ?", campaignID, models.ExecutionStatusSuccess).Find(&executions).Error; err != nil {
		return fmt.Errorf("failed to get executions: %w", err)
	}

	m.logger.Info("executing rollback",
		zap.String("campaign_id", campaignID),
		zap.Int("agents_to_rollback", len(executions)))

	if config.RollbackWorkflow == "" {
		// No rollback workflow specified, just mark as rolled back
		return m.markRollbackComplete(campaignID)
	}

	// Execute rollback workflow on each agent
	// This would trigger the rollback workflow execution
	// For now, we'll just mark as complete

	return m.markRollbackComplete(campaignID)
}

// markRollbackComplete marks the rollback as complete
func (m *RollbackManager) markRollbackComplete(campaignID string) error {
	now := time.Now()
	return m.db.Model(&models.Campaign{}).Where("id = ?", campaignID).Updates(map[string]interface{}{
		"status":       models.CampaignStatusFailed,
		"completed_at": now,
	}).Error
}

// CanRollback checks if a campaign can be rolled back
func (m *RollbackManager) CanRollback(ctx context.Context, campaignID string) (bool, string) {
	var campaign models.Campaign
	if err := m.db.First(&campaign, "id = ?", campaignID).Error; err != nil {
		return false, "campaign not found"
	}

	switch campaign.Status {
	case models.CampaignStatusRunning, models.CampaignStatusPaused:
		return true, ""
	case models.CampaignStatusDraft:
		return false, "campaign has not started"
	case models.CampaignStatusCompleted, models.CampaignStatusFailed, models.CampaignStatusCancelled:
		return false, "campaign is already finished"
	case models.CampaignStatusRollingBack:
		return false, "rollback already in progress"
	default:
		return false, "unknown campaign status"
	}
}

// GetRollbackStatus returns the status of a rollback
func (m *RollbackManager) GetRollbackStatus(ctx context.Context, campaignID string) (*RollbackStatus, error) {
	var campaign models.Campaign
	if err := m.db.First(&campaign, "id = ?", campaignID).Error; err != nil {
		return nil, fmt.Errorf("campaign not found: %w", err)
	}

	status := &RollbackStatus{
		CampaignID: campaignID,
		Status:     string(campaign.Status),
	}

	if progress, ok := campaign.Progress["rollback_reason"].(string); ok {
		status.Reason = progress
	}

	if startedAt, ok := campaign.Progress["rollback_started_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, startedAt); err == nil {
			status.StartedAt = &t
		}
	}

	return status, nil
}

// RollbackStatus represents rollback status
type RollbackStatus struct {
	CampaignID      string     `json:"campaign_id"`
	Status          string     `json:"status"`
	Reason          string     `json:"reason,omitempty"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	AgentsRolledBack int       `json:"agents_rolled_back"`
	AgentsFailed    int        `json:"agents_failed"`
}
