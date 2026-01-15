// Package tenant provides tenant management for the control plane.
package tenant

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/yourorg/control-plane/pkg/db/models"
)

// Manager manages tenant operations
type Manager struct {
	db     *gorm.DB
	logger *zap.Logger
}

// NewManager creates a new tenant manager
func NewManager(db *gorm.DB, logger *zap.Logger) *Manager {
	return &Manager{
		db:     db,
		logger: logger,
	}
}

// CreateTenantRequest represents a request to create a tenant
type CreateTenantRequest struct {
	Name           string                 `json:"name" binding:"required"`
	Description    string                 `json:"description"`
	Settings       map[string]interface{} `json:"settings"`
	QuotaAgents    int                    `json:"quota_agents"`
	QuotaWorkflows int                    `json:"quota_workflows"`
}

// Create creates a new tenant
func (m *Manager) Create(ctx context.Context, req *CreateTenantRequest) (*models.Tenant, error) {
	tenant := &models.Tenant{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Description: req.Description,
		Status:      models.TenantStatusActive,
		Settings:    req.Settings,
		QuotaAgents: req.QuotaAgents,
		QuotaWorkflows: req.QuotaWorkflows,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if tenant.QuotaAgents == 0 {
		tenant.QuotaAgents = 1000
	}
	if tenant.QuotaWorkflows == 0 {
		tenant.QuotaWorkflows = 100
	}

	if err := m.db.Create(tenant).Error; err != nil {
		return nil, fmt.Errorf("failed to create tenant: %w", err)
	}

	m.logger.Info("tenant created",
		zap.String("tenant_id", tenant.ID),
		zap.String("name", tenant.Name))

	return tenant, nil
}

// Get retrieves a tenant by ID
func (m *Manager) Get(ctx context.Context, tenantID string) (*models.Tenant, error) {
	var tenant models.Tenant
	if err := m.db.Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("tenant not found")
		}
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}
	return &tenant, nil
}

// GetByName retrieves a tenant by name
func (m *Manager) GetByName(ctx context.Context, name string) (*models.Tenant, error) {
	var tenant models.Tenant
	if err := m.db.Where("name = ?", name).First(&tenant).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("tenant not found")
		}
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}
	return &tenant, nil
}

// UpdateTenantRequest represents a request to update a tenant
type UpdateTenantRequest struct {
	Name           *string                `json:"name"`
	Description    *string                `json:"description"`
	Settings       map[string]interface{} `json:"settings"`
	QuotaAgents    *int                   `json:"quota_agents"`
	QuotaWorkflows *int                   `json:"quota_workflows"`
}

// Update updates a tenant
func (m *Manager) Update(ctx context.Context, tenantID string, req *UpdateTenantRequest) (*models.Tenant, error) {
	tenant, err := m.Get(ctx, tenantID)
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
	if req.Settings != nil {
		updates["settings"] = req.Settings
	}
	if req.QuotaAgents != nil {
		updates["quota_agents"] = *req.QuotaAgents
	}
	if req.QuotaWorkflows != nil {
		updates["quota_workflows"] = *req.QuotaWorkflows
	}

	if len(updates) == 0 {
		return tenant, nil
	}

	if err := m.db.Model(tenant).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update tenant: %w", err)
	}

	m.logger.Info("tenant updated",
		zap.String("tenant_id", tenantID))

	return m.Get(ctx, tenantID)
}

// Delete soft-deletes a tenant
func (m *Manager) Delete(ctx context.Context, tenantID string) error {
	result := m.db.Model(&models.Tenant{}).Where("id = ?", tenantID).Updates(map[string]interface{}{
		"status":     models.TenantStatusDeleted,
		"deleted_at": time.Now(),
	})

	if result.Error != nil {
		return fmt.Errorf("failed to delete tenant: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("tenant not found")
	}

	m.logger.Info("tenant deleted",
		zap.String("tenant_id", tenantID))

	return nil
}

// Suspend suspends a tenant
func (m *Manager) Suspend(ctx context.Context, tenantID string) error {
	result := m.db.Model(&models.Tenant{}).Where("id = ?", tenantID).Update("status", models.TenantStatusSuspended)

	if result.Error != nil {
		return fmt.Errorf("failed to suspend tenant: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("tenant not found")
	}

	m.logger.Info("tenant suspended",
		zap.String("tenant_id", tenantID))

	return nil
}

// Activate activates a suspended tenant
func (m *Manager) Activate(ctx context.Context, tenantID string) error {
	result := m.db.Model(&models.Tenant{}).Where("id = ? AND status = ?", tenantID, models.TenantStatusSuspended).Update("status", models.TenantStatusActive)

	if result.Error != nil {
		return fmt.Errorf("failed to activate tenant: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("tenant not found or not suspended")
	}

	m.logger.Info("tenant activated",
		zap.String("tenant_id", tenantID))

	return nil
}

// ListTenantsRequest represents a request to list tenants
type ListTenantsRequest struct {
	Status string
	Limit  int
	Offset int
}

// List lists tenants
func (m *Manager) List(ctx context.Context, req *ListTenantsRequest) ([]models.Tenant, int64, error) {
	query := m.db.Model(&models.Tenant{})

	if req.Status != "" {
		query = query.Where("status = ?", req.Status)
	} else {
		query = query.Where("status != ?", models.TenantStatusDeleted)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count tenants: %w", err)
	}

	if req.Limit > 0 {
		query = query.Limit(req.Limit)
	}
	if req.Offset > 0 {
		query = query.Offset(req.Offset)
	}

	var tenants []models.Tenant
	if err := query.Order("created_at DESC").Find(&tenants).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list tenants: %w", err)
	}

	return tenants, total, nil
}

// GetStats returns tenant statistics
func (m *Manager) GetStats(ctx context.Context, tenantID string) (*TenantStats, error) {
	stats := &TenantStats{}

	// Count agents
	if err := m.db.Model(&models.Agent{}).Where("tenant_id = ?", tenantID).Count(&stats.TotalAgents).Error; err != nil {
		return nil, err
	}

	// Count online agents
	if err := m.db.Model(&models.Agent{}).Where("tenant_id = ? AND status = ?", tenantID, models.AgentStatusOnline).Count(&stats.OnlineAgents).Error; err != nil {
		return nil, err
	}

	// Count workflows
	if err := m.db.Model(&models.Workflow{}).Where("tenant_id = ? AND status != ?", tenantID, models.WorkflowStatusDeleted).Count(&stats.TotalWorkflows).Error; err != nil {
		return nil, err
	}

	// Count campaigns
	if err := m.db.Model(&models.Campaign{}).Where("tenant_id = ?", tenantID).Count(&stats.TotalCampaigns).Error; err != nil {
		return nil, err
	}

	return stats, nil
}

// TenantStats contains tenant statistics
type TenantStats struct {
	TotalAgents    int64 `json:"total_agents"`
	OnlineAgents   int64 `json:"online_agents"`
	TotalWorkflows int64 `json:"total_workflows"`
	TotalCampaigns int64 `json:"total_campaigns"`
}
