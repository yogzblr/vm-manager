// Package campaign provides campaign management for the control plane.
package campaign

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/yourorg/control-plane/pkg/db/models"
)

// Manager manages campaigns
type Manager struct {
	db     *gorm.DB
	logger *zap.Logger
}

// NewManager creates a new campaign manager
func NewManager(db *gorm.DB, logger *zap.Logger) *Manager {
	return &Manager{
		db:     db,
		logger: logger,
	}
}

// CreateCampaignRequest represents a request to create a campaign
type CreateCampaignRequest struct {
	TenantID       string                 `json:"tenant_id" binding:"required"`
	WorkflowID     string                 `json:"workflow_id" binding:"required"`
	Name           string                 `json:"name" binding:"required"`
	Description    string                 `json:"description"`
	TargetSelector map[string]interface{} `json:"target_selector" binding:"required"`
	PhaseConfig    []PhaseConfig          `json:"phase_config" binding:"required"`
	CreatedBy      string                 `json:"created_by"`
}

// PhaseConfig represents phase configuration
type PhaseConfig struct {
	Name             string  `json:"name"`
	Percentage       float64 `json:"percentage"`
	SuccessThreshold float64 `json:"success_threshold"`
	WaitMinutes      int     `json:"wait_minutes"`
}

// Create creates a new campaign
func (m *Manager) Create(ctx context.Context, req *CreateCampaignRequest) (*models.Campaign, error) {
	// Verify workflow exists and is active
	var workflow models.Workflow
	if err := m.db.Where("id = ? AND tenant_id = ? AND status = ?", req.WorkflowID, req.TenantID, models.WorkflowStatusActive).First(&workflow).Error; err != nil {
		return nil, fmt.Errorf("workflow not found or not active")
	}

	// Convert phase config to map
	phaseConfigMap := make(map[string]interface{})
	phases := make([]map[string]interface{}, len(req.PhaseConfig))
	for i, phase := range req.PhaseConfig {
		phases[i] = map[string]interface{}{
			"name":              phase.Name,
			"percentage":        phase.Percentage,
			"success_threshold": phase.SuccessThreshold,
			"wait_minutes":      phase.WaitMinutes,
		}
	}
	phaseConfigMap["phases"] = phases

	campaign := &models.Campaign{
		ID:             uuid.New().String(),
		TenantID:       req.TenantID,
		WorkflowID:     req.WorkflowID,
		Name:           req.Name,
		Description:    req.Description,
		Status:         models.CampaignStatusDraft,
		TargetSelector: req.TargetSelector,
		PhaseConfig:    phaseConfigMap,
		CreatedBy:      req.CreatedBy,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := m.db.Create(campaign).Error; err != nil {
		return nil, fmt.Errorf("failed to create campaign: %w", err)
	}

	// Create phase records
	for i, phase := range req.PhaseConfig {
		campaignPhase := &models.CampaignPhase{
			ID:         uuid.New().String(),
			CampaignID: campaign.ID,
			PhaseName:  phase.Name,
			PhaseOrder: i,
			Status:     models.PhaseStatusPending,
		}
		if err := m.db.Create(campaignPhase).Error; err != nil {
			return nil, fmt.Errorf("failed to create campaign phase: %w", err)
		}
	}

	m.logger.Info("campaign created",
		zap.String("campaign_id", campaign.ID),
		zap.String("tenant_id", req.TenantID),
		zap.String("workflow_id", req.WorkflowID))

	return campaign, nil
}

// Get retrieves a campaign by ID
func (m *Manager) Get(ctx context.Context, tenantID, campaignID string) (*models.Campaign, error) {
	var campaign models.Campaign
	if err := m.db.Preload("Phases").Where("id = ? AND tenant_id = ?", campaignID, tenantID).First(&campaign).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("campaign not found")
		}
		return nil, err
	}
	return &campaign, nil
}

// Start starts a campaign
func (m *Manager) Start(ctx context.Context, tenantID, campaignID string) error {
	campaign, err := m.Get(ctx, tenantID, campaignID)
	if err != nil {
		return err
	}

	if campaign.Status != models.CampaignStatusDraft && campaign.Status != models.CampaignStatusPaused {
		return fmt.Errorf("campaign cannot be started from status: %s", campaign.Status)
	}

	now := time.Now()
	updates := map[string]interface{}{
		"status":     models.CampaignStatusRunning,
		"updated_at": now,
	}

	if campaign.StartedAt == nil {
		updates["started_at"] = now
	}

	if err := m.db.Model(campaign).Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to start campaign: %w", err)
	}

	m.logger.Info("campaign started",
		zap.String("campaign_id", campaignID))

	return nil
}

// Pause pauses a running campaign
func (m *Manager) Pause(ctx context.Context, tenantID, campaignID string) error {
	result := m.db.Model(&models.Campaign{}).
		Where("id = ? AND tenant_id = ? AND status = ?", campaignID, tenantID, models.CampaignStatusRunning).
		Update("status", models.CampaignStatusPaused)

	if result.Error != nil {
		return fmt.Errorf("failed to pause campaign: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("campaign not found or not running")
	}

	return nil
}

// Cancel cancels a campaign
func (m *Manager) Cancel(ctx context.Context, tenantID, campaignID string) error {
	now := time.Now()
	result := m.db.Model(&models.Campaign{}).
		Where("id = ? AND tenant_id = ? AND status IN ?", campaignID, tenantID, []models.CampaignStatus{
			models.CampaignStatusDraft,
			models.CampaignStatusRunning,
			models.CampaignStatusPaused,
		}).
		Updates(map[string]interface{}{
			"status":       models.CampaignStatusCancelled,
			"completed_at": now,
		})

	if result.Error != nil {
		return fmt.Errorf("failed to cancel campaign: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("campaign not found or cannot be cancelled")
	}

	m.logger.Info("campaign cancelled",
		zap.String("campaign_id", campaignID))

	return nil
}

// List lists campaigns
func (m *Manager) List(ctx context.Context, tenantID string, status models.CampaignStatus, limit, offset int) ([]models.Campaign, int64, error) {
	query := m.db.Model(&models.Campaign{}).Where("tenant_id = ?", tenantID)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	var campaigns []models.Campaign
	if err := query.Order("created_at DESC").Find(&campaigns).Error; err != nil {
		return nil, 0, err
	}

	return campaigns, total, nil
}

// GetProgress returns campaign progress
func (m *Manager) GetProgress(ctx context.Context, tenantID, campaignID string) (*models.CampaignProgress, error) {
	campaign, err := m.Get(ctx, tenantID, campaignID)
	if err != nil {
		return nil, err
	}

	progress := &models.CampaignProgress{}

	// Get current phase
	var currentPhase models.CampaignPhase
	if err := m.db.Where("campaign_id = ? AND status IN ?", campaignID, []models.PhaseStatus{
		models.PhaseStatusPending,
		models.PhaseStatusRunning,
	}).Order("phase_order ASC").First(&currentPhase).Error; err == nil {
		progress.CurrentPhase = currentPhase.PhaseName
	}

	// Count executions
	var total, success, failed int64
	m.db.Model(&models.WorkflowExecution{}).Where("campaign_id = ?", campaignID).Count(&total)
	m.db.Model(&models.WorkflowExecution{}).Where("campaign_id = ? AND status = ?", campaignID, models.ExecutionStatusSuccess).Count(&success)
	m.db.Model(&models.WorkflowExecution{}).Where("campaign_id = ? AND status = ?", campaignID, models.ExecutionStatusFailed).Count(&failed)

	progress.TotalAgents = int(total)
	progress.SuccessfulAgents = int(success)
	progress.FailedAgents = int(failed)
	progress.CompletedAgents = int(success + failed)

	if progress.CompletedAgents > 0 {
		progress.SuccessRate = float64(progress.SuccessfulAgents) / float64(progress.CompletedAgents) * 100
	}

	// Update campaign progress
	m.db.Model(campaign).Update("progress", progress)

	return progress, nil
}
