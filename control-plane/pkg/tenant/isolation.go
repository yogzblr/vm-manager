// Package tenant provides tenant management for the control plane.
package tenant

import (
	"fmt"

	"gorm.io/gorm"
)

// TenantScope is a GORM scope that filters by tenant
func TenantScope(tenantID string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("tenant_id = ?", tenantID)
	}
}

// IsolationEnforcer enforces tenant isolation
type IsolationEnforcer struct {
	db *gorm.DB
}

// NewIsolationEnforcer creates a new isolation enforcer
func NewIsolationEnforcer(db *gorm.DB) *IsolationEnforcer {
	return &IsolationEnforcer{db: db}
}

// ScopedDB returns a database scoped to a tenant
func (e *IsolationEnforcer) ScopedDB(tenantID string) *gorm.DB {
	return e.db.Scopes(TenantScope(tenantID))
}

// ValidateAccess validates that an entity belongs to the tenant
func (e *IsolationEnforcer) ValidateAccess(tenantID, entityTenantID string) error {
	if tenantID != entityTenantID {
		return fmt.Errorf("access denied: entity belongs to different tenant")
	}
	return nil
}

// ValidateAgentAccess validates agent access
func (e *IsolationEnforcer) ValidateAgentAccess(tenantID, agentID string) error {
	var count int64
	if err := e.db.Table("agents").Where("id = ? AND tenant_id = ?", agentID, tenantID).Count(&count).Error; err != nil {
		return fmt.Errorf("failed to validate agent access: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("agent not found or access denied")
	}
	return nil
}

// ValidateWorkflowAccess validates workflow access
func (e *IsolationEnforcer) ValidateWorkflowAccess(tenantID, workflowID string) error {
	var count int64
	if err := e.db.Table("workflows").Where("id = ? AND tenant_id = ?", workflowID, tenantID).Count(&count).Error; err != nil {
		return fmt.Errorf("failed to validate workflow access: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("workflow not found or access denied")
	}
	return nil
}

// ValidateCampaignAccess validates campaign access
func (e *IsolationEnforcer) ValidateCampaignAccess(tenantID, campaignID string) error {
	var count int64
	if err := e.db.Table("campaigns").Where("id = ? AND tenant_id = ?", campaignID, tenantID).Count(&count).Error; err != nil {
		return fmt.Errorf("failed to validate campaign access: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("campaign not found or access denied")
	}
	return nil
}

// QuotaChecker checks tenant quotas
type QuotaChecker struct {
	db *gorm.DB
}

// NewQuotaChecker creates a new quota checker
func NewQuotaChecker(db *gorm.DB) *QuotaChecker {
	return &QuotaChecker{db: db}
}

// CheckAgentQuota checks if tenant can add more agents
func (c *QuotaChecker) CheckAgentQuota(tenantID string) error {
	var tenant struct {
		QuotaAgents int
	}
	if err := c.db.Table("tenants").Select("quota_agents").Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return fmt.Errorf("failed to get tenant quota: %w", err)
	}

	var count int64
	if err := c.db.Table("agents").Where("tenant_id = ?", tenantID).Count(&count).Error; err != nil {
		return fmt.Errorf("failed to count agents: %w", err)
	}

	if int(count) >= tenant.QuotaAgents {
		return fmt.Errorf("agent quota exceeded: %d/%d", count, tenant.QuotaAgents)
	}

	return nil
}

// CheckWorkflowQuota checks if tenant can add more workflows
func (c *QuotaChecker) CheckWorkflowQuota(tenantID string) error {
	var tenant struct {
		QuotaWorkflows int
	}
	if err := c.db.Table("tenants").Select("quota_workflows").Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return fmt.Errorf("failed to get tenant quota: %w", err)
	}

	var count int64
	if err := c.db.Table("workflows").Where("tenant_id = ? AND status != 'deleted'", tenantID).Count(&count).Error; err != nil {
		return fmt.Errorf("failed to count workflows: %w", err)
	}

	if int(count) >= tenant.QuotaWorkflows {
		return fmt.Errorf("workflow quota exceeded: %d/%d", count, tenant.QuotaWorkflows)
	}

	return nil
}

// QuotaStatus represents quota usage status
type QuotaStatus struct {
	AgentsCurrent    int64 `json:"agents_current"`
	AgentsQuota      int   `json:"agents_quota"`
	WorkflowsCurrent int64 `json:"workflows_current"`
	WorkflowsQuota   int   `json:"workflows_quota"`
}

// GetQuotaStatus gets the current quota status for a tenant
func (c *QuotaChecker) GetQuotaStatus(tenantID string) (*QuotaStatus, error) {
	var tenant struct {
		QuotaAgents    int
		QuotaWorkflows int
	}
	if err := c.db.Table("tenants").Select("quota_agents, quota_workflows").Where("id = ?", tenantID).First(&tenant).Error; err != nil {
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}

	status := &QuotaStatus{
		AgentsQuota:    tenant.QuotaAgents,
		WorkflowsQuota: tenant.QuotaWorkflows,
	}

	if err := c.db.Table("agents").Where("tenant_id = ?", tenantID).Count(&status.AgentsCurrent).Error; err != nil {
		return nil, err
	}

	if err := c.db.Table("workflows").Where("tenant_id = ? AND status != 'deleted'", tenantID).Count(&status.WorkflowsCurrent).Error; err != nil {
		return nil, err
	}

	return status, nil
}
