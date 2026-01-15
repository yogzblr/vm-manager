// Package models contains database models for the control plane.
package models

import (
	"time"
)

// WorkflowStatus represents the status of a workflow
type WorkflowStatus string

const (
	WorkflowStatusDraft      WorkflowStatus = "draft"
	WorkflowStatusActive     WorkflowStatus = "active"
	WorkflowStatusDeprecated WorkflowStatus = "deprecated"
	WorkflowStatusDeleted    WorkflowStatus = "deleted"
)

// Workflow represents a workflow definition
type Workflow struct {
	ID          string         `gorm:"primaryKey;size:64" json:"id"`
	TenantID    string         `gorm:"size:64;not null;index" json:"tenant_id"`
	Name        string         `gorm:"size:255;not null" json:"name"`
	Description string         `gorm:"type:text" json:"description,omitempty"`
	Definition  JSONMap        `gorm:"type:json;not null" json:"definition"`
	Version     int            `gorm:"default:1" json:"version"`
	Status      WorkflowStatus `gorm:"type:enum('draft','active','deprecated','deleted');default:'draft'" json:"status"`
	CreatedBy   string         `gorm:"size:255" json:"created_by,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`

	// Relationships
	Tenant     Tenant              `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Executions []WorkflowExecution `gorm:"foreignKey:WorkflowID" json:"executions,omitempty"`
	Campaigns  []Campaign          `gorm:"foreignKey:WorkflowID" json:"campaigns,omitempty"`
}

// TableName returns the table name for Workflow
func (Workflow) TableName() string {
	return "workflows"
}

// ExecutionStatus represents the status of a workflow execution
type ExecutionStatus string

const (
	ExecutionStatusPending   ExecutionStatus = "pending"
	ExecutionStatusRunning   ExecutionStatus = "running"
	ExecutionStatusSuccess   ExecutionStatus = "success"
	ExecutionStatusFailed    ExecutionStatus = "failed"
	ExecutionStatusCancelled ExecutionStatus = "cancelled"
	ExecutionStatusTimeout   ExecutionStatus = "timeout"
)

// WorkflowExecution represents a workflow execution
type WorkflowExecution struct {
	ID          string          `gorm:"primaryKey;size:64" json:"id"`
	WorkflowID  string          `gorm:"size:64;not null;index" json:"workflow_id"`
	TenantID    string          `gorm:"size:64;not null;index" json:"tenant_id"`
	AgentID     string          `gorm:"size:64;not null;index" json:"agent_id"`
	CampaignID  *string         `gorm:"size:64;index" json:"campaign_id,omitempty"`
	Status      ExecutionStatus `gorm:"type:enum('pending','running','success','failed','cancelled','timeout');default:'pending'" json:"status"`
	Result      JSONMap         `gorm:"type:json" json:"result,omitempty"`
	StartedAt   *time.Time      `json:"started_at,omitempty"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`

	// Relationships
	Workflow Workflow  `gorm:"foreignKey:WorkflowID" json:"workflow,omitempty"`
	Tenant   Tenant    `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Agent    Agent     `gorm:"foreignKey:AgentID" json:"agent,omitempty"`
	Campaign *Campaign `gorm:"foreignKey:CampaignID" json:"campaign,omitempty"`
}

// TableName returns the table name for WorkflowExecution
func (WorkflowExecution) TableName() string {
	return "workflow_executions"
}

// Duration returns the execution duration
func (e *WorkflowExecution) Duration() *time.Duration {
	if e.StartedAt == nil || e.CompletedAt == nil {
		return nil
	}
	d := e.CompletedAt.Sub(*e.StartedAt)
	return &d
}

// IsComplete returns true if the execution is complete
func (e *WorkflowExecution) IsComplete() bool {
	switch e.Status {
	case ExecutionStatusSuccess, ExecutionStatusFailed, ExecutionStatusCancelled, ExecutionStatusTimeout:
		return true
	default:
		return false
	}
}
