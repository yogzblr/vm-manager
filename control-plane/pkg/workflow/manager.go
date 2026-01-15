// Package workflow provides workflow management for the control plane.
package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/yourorg/control-plane/pkg/db/models"
	"github.com/yourorg/control-plane/pkg/tenant"
)

// Manager manages workflows
type Manager struct {
	db           *gorm.DB
	quotaChecker *tenant.QuotaChecker
	logger       *zap.Logger
}

// NewManager creates a new workflow manager
func NewManager(db *gorm.DB, logger *zap.Logger) *Manager {
	return &Manager{
		db:           db,
		quotaChecker: tenant.NewQuotaChecker(db),
		logger:       logger,
	}
}

// CreateWorkflowRequest represents a request to create a workflow
type CreateWorkflowRequest struct {
	TenantID    string                 `json:"tenant_id" binding:"required"`
	Name        string                 `json:"name" binding:"required"`
	Description string                 `json:"description"`
	Definition  map[string]interface{} `json:"definition" binding:"required"`
	CreatedBy   string                 `json:"created_by"`
}

// Create creates a new workflow
func (m *Manager) Create(ctx context.Context, req *CreateWorkflowRequest) (*models.Workflow, error) {
	// Check quota
	if err := m.quotaChecker.CheckWorkflowQuota(req.TenantID); err != nil {
		return nil, err
	}

	// Validate workflow definition
	validator := NewValidator()
	if err := validator.Validate(req.Definition); err != nil {
		return nil, fmt.Errorf("workflow validation failed: %w", err)
	}

	workflow := &models.Workflow{
		ID:          uuid.New().String(),
		TenantID:    req.TenantID,
		Name:        req.Name,
		Description: req.Description,
		Definition:  req.Definition,
		Version:     1,
		Status:      models.WorkflowStatusDraft,
		CreatedBy:   req.CreatedBy,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := m.db.Create(workflow).Error; err != nil {
		return nil, fmt.Errorf("failed to create workflow: %w", err)
	}

	m.logger.Info("workflow created",
		zap.String("workflow_id", workflow.ID),
		zap.String("tenant_id", req.TenantID),
		zap.String("name", req.Name))

	return workflow, nil
}

// Get retrieves a workflow by ID
func (m *Manager) Get(ctx context.Context, tenantID, workflowID string) (*models.Workflow, error) {
	var workflow models.Workflow
	if err := m.db.Where("id = ? AND tenant_id = ?", workflowID, tenantID).First(&workflow).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("workflow not found")
		}
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}
	return &workflow, nil
}

// UpdateWorkflowRequest represents a request to update a workflow
type UpdateWorkflowRequest struct {
	Name        *string                `json:"name"`
	Description *string                `json:"description"`
	Definition  map[string]interface{} `json:"definition"`
	Status      *models.WorkflowStatus `json:"status"`
}

// Update updates a workflow
func (m *Manager) Update(ctx context.Context, tenantID, workflowID string, req *UpdateWorkflowRequest) (*models.Workflow, error) {
	workflow, err := m.Get(ctx, tenantID, workflowID)
	if err != nil {
		return nil, err
	}

	updates := make(map[string]interface{})

	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Definition != nil {
		validator := NewValidator()
		if err := validator.Validate(req.Definition); err != nil {
			return nil, fmt.Errorf("workflow validation failed: %w", err)
		}
		updates["definition"] = req.Definition
		updates["version"] = workflow.Version + 1
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}

	if len(updates) == 0 {
		return workflow, nil
	}

	updates["updated_at"] = time.Now()

	if err := m.db.Model(workflow).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update workflow: %w", err)
	}

	return m.Get(ctx, tenantID, workflowID)
}

// Delete soft-deletes a workflow
func (m *Manager) Delete(ctx context.Context, tenantID, workflowID string) error {
	result := m.db.Model(&models.Workflow{}).
		Where("id = ? AND tenant_id = ?", workflowID, tenantID).
		Update("status", models.WorkflowStatusDeleted)

	if result.Error != nil {
		return fmt.Errorf("failed to delete workflow: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("workflow not found")
	}

	m.logger.Info("workflow deleted",
		zap.String("workflow_id", workflowID),
		zap.String("tenant_id", tenantID))

	return nil
}

// ListWorkflowsRequest represents a request to list workflows
type ListWorkflowsRequest struct {
	TenantID string
	Status   models.WorkflowStatus
	Limit    int
	Offset   int
}

// List lists workflows
func (m *Manager) List(ctx context.Context, req *ListWorkflowsRequest) ([]models.Workflow, int64, error) {
	query := m.db.Model(&models.Workflow{}).Where("tenant_id = ?", req.TenantID)

	if req.Status != "" {
		query = query.Where("status = ?", req.Status)
	} else {
		query = query.Where("status != ?", models.WorkflowStatusDeleted)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count workflows: %w", err)
	}

	if req.Limit > 0 {
		query = query.Limit(req.Limit)
	}
	if req.Offset > 0 {
		query = query.Offset(req.Offset)
	}

	var workflows []models.Workflow
	if err := query.Order("created_at DESC").Find(&workflows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list workflows: %w", err)
	}

	return workflows, total, nil
}

// Activate activates a workflow
func (m *Manager) Activate(ctx context.Context, tenantID, workflowID string) error {
	result := m.db.Model(&models.Workflow{}).
		Where("id = ? AND tenant_id = ? AND status = ?", workflowID, tenantID, models.WorkflowStatusDraft).
		Update("status", models.WorkflowStatusActive)

	if result.Error != nil {
		return fmt.Errorf("failed to activate workflow: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("workflow not found or not in draft status")
	}

	return nil
}

// Deprecate deprecates a workflow
func (m *Manager) Deprecate(ctx context.Context, tenantID, workflowID string) error {
	result := m.db.Model(&models.Workflow{}).
		Where("id = ? AND tenant_id = ? AND status = ?", workflowID, tenantID, models.WorkflowStatusActive).
		Update("status", models.WorkflowStatusDeprecated)

	if result.Error != nil {
		return fmt.Errorf("failed to deprecate workflow: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("workflow not found or not active")
	}

	return nil
}
