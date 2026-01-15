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

// PhaseExecutor handles campaign phase execution
type PhaseExecutor struct {
	db     *gorm.DB
	logger *zap.Logger
}

// NewPhaseExecutor creates a new phase executor
func NewPhaseExecutor(db *gorm.DB, logger *zap.Logger) *PhaseExecutor {
	return &PhaseExecutor{
		db:     db,
		logger: logger,
	}
}

// ExecutePhase executes a campaign phase
func (e *PhaseExecutor) ExecutePhase(ctx context.Context, campaignID, phaseID string) error {
	var phase models.CampaignPhase
	if err := e.db.First(&phase, "id = ?", phaseID).Error; err != nil {
		return fmt.Errorf("phase not found: %w", err)
	}

	// Mark phase as running
	now := time.Now()
	e.db.Model(&phase).Updates(map[string]interface{}{
		"status":     models.PhaseStatusRunning,
		"started_at": now,
	})

	e.logger.Info("phase started",
		zap.String("campaign_id", campaignID),
		zap.String("phase_id", phaseID),
		zap.String("phase_name", phase.PhaseName))

	return nil
}

// CompletePhase marks a phase as complete
func (e *PhaseExecutor) CompletePhase(ctx context.Context, phaseID string, success bool) error {
	now := time.Now()
	status := models.PhaseStatusSuccess
	if !success {
		status = models.PhaseStatusFailed
	}

	return e.db.Model(&models.CampaignPhase{}).Where("id = ?", phaseID).Updates(map[string]interface{}{
		"status":       status,
		"completed_at": now,
	}).Error
}

// UpdatePhaseProgress updates phase progress
func (e *PhaseExecutor) UpdatePhaseProgress(ctx context.Context, phaseID string, successCount, failureCount int) error {
	return e.db.Model(&models.CampaignPhase{}).Where("id = ?", phaseID).Updates(map[string]interface{}{
		"success_count": successCount,
		"failure_count": failureCount,
	}).Error
}

// GetPhaseAgents returns the agents targeted by a phase
func (e *PhaseExecutor) GetPhaseAgents(ctx context.Context, campaign *models.Campaign, phaseIndex int) ([]models.Agent, error) {
	// Get phase config
	phaseConfigRaw := campaign.PhaseConfig["phases"]
	phases, ok := phaseConfigRaw.([]interface{})
	if !ok || phaseIndex >= len(phases) {
		return nil, fmt.Errorf("invalid phase index")
	}

	phaseConfig, ok := phases[phaseIndex].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid phase config")
	}

	percentage := phaseConfig["percentage"].(float64)

	// Get all matching agents
	query := e.db.Model(&models.Agent{}).Where("tenant_id = ?", campaign.TenantID)

	// Apply target selector filters
	if tags, ok := campaign.TargetSelector["tags"].(map[string]interface{}); ok {
		for key, value := range tags {
			query = query.Where("JSON_EXTRACT(tags, ?) = ?", "$."+key, value)
		}
	}

	if status, ok := campaign.TargetSelector["status"].(string); ok {
		query = query.Where("status = ?", status)
	}

	var allAgents []models.Agent
	if err := query.Find(&allAgents).Error; err != nil {
		return nil, err
	}

	// Calculate number of agents for this phase
	targetCount := int(float64(len(allAgents)) * percentage / 100)
	if targetCount < 1 && len(allAgents) > 0 {
		targetCount = 1
	}

	// Get agents already processed in previous phases
	var processedAgentIDs []string
	e.db.Model(&models.WorkflowExecution{}).
		Where("campaign_id = ?", campaign.ID).
		Pluck("agent_id", &processedAgentIDs)

	processedMap := make(map[string]bool)
	for _, id := range processedAgentIDs {
		processedMap[id] = true
	}

	// Filter out already processed agents
	var availableAgents []models.Agent
	for _, agent := range allAgents {
		if !processedMap[agent.ID] {
			availableAgents = append(availableAgents, agent)
		}
	}

	// Select agents for this phase
	if targetCount > len(availableAgents) {
		targetCount = len(availableAgents)
	}

	return availableAgents[:targetCount], nil
}

// CheckPhaseCompletion checks if a phase is complete
func (e *PhaseExecutor) CheckPhaseCompletion(ctx context.Context, phaseID string) (bool, error) {
	var phase models.CampaignPhase
	if err := e.db.First(&phase, "id = ?", phaseID).Error; err != nil {
		return false, err
	}

	// Check if all executions are complete
	var pending int64
	e.db.Model(&models.WorkflowExecution{}).
		Where("campaign_id = ? AND status IN ?",
			phase.CampaignID,
			[]models.ExecutionStatus{models.ExecutionStatusPending, models.ExecutionStatusRunning}).
		Count(&pending)

	return pending == 0, nil
}

// CheckPhaseSuccess checks if a phase meets success threshold
func (e *PhaseExecutor) CheckPhaseSuccess(ctx context.Context, phaseID string, threshold float64) (bool, error) {
	var phase models.CampaignPhase
	if err := e.db.First(&phase, "id = ?", phaseID).Error; err != nil {
		return false, err
	}

	successRate := phase.SuccessRate()
	return successRate >= threshold, nil
}

// GetNextPhase returns the next pending phase
func (e *PhaseExecutor) GetNextPhase(ctx context.Context, campaignID string) (*models.CampaignPhase, error) {
	var phase models.CampaignPhase
	if err := e.db.Where("campaign_id = ? AND status = ?", campaignID, models.PhaseStatusPending).
		Order("phase_order ASC").First(&phase).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &phase, nil
}
