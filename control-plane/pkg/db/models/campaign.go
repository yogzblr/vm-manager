// Package models contains database models for the control plane.
package models

import (
	"time"
)

// CampaignStatus represents the status of a campaign
type CampaignStatus string

const (
	CampaignStatusDraft       CampaignStatus = "draft"
	CampaignStatusRunning     CampaignStatus = "running"
	CampaignStatusPaused      CampaignStatus = "paused"
	CampaignStatusCompleted   CampaignStatus = "completed"
	CampaignStatusFailed      CampaignStatus = "failed"
	CampaignStatusCancelled   CampaignStatus = "cancelled"
	CampaignStatusRollingBack CampaignStatus = "rolling_back"
)

// Campaign represents a phased workflow rollout campaign
type Campaign struct {
	ID             string         `gorm:"primaryKey;size:64" json:"id"`
	TenantID       string         `gorm:"size:64;not null;index" json:"tenant_id"`
	WorkflowID     string         `gorm:"size:64;not null;index" json:"workflow_id"`
	Name           string         `gorm:"size:255;not null" json:"name"`
	Description    string         `gorm:"type:text" json:"description,omitempty"`
	Status         CampaignStatus `gorm:"type:enum('draft','running','paused','completed','failed','cancelled','rolling_back');default:'draft'" json:"status"`
	TargetSelector JSONMap        `gorm:"type:json;not null" json:"target_selector"`
	PhaseConfig    JSONMap        `gorm:"type:json;not null" json:"phase_config"`
	Progress       JSONMap        `gorm:"type:json" json:"progress,omitempty"`
	CreatedBy      string         `gorm:"size:255" json:"created_by,omitempty"`
	StartedAt      *time.Time     `json:"started_at,omitempty"`
	CompletedAt    *time.Time     `json:"completed_at,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`

	// Relationships
	Tenant     Tenant              `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Workflow   Workflow            `gorm:"foreignKey:WorkflowID" json:"workflow,omitempty"`
	Phases     []CampaignPhase     `gorm:"foreignKey:CampaignID" json:"phases,omitempty"`
	Executions []WorkflowExecution `gorm:"foreignKey:CampaignID" json:"executions,omitempty"`
}

// TableName returns the table name for Campaign
func (Campaign) TableName() string {
	return "campaigns"
}

// PhaseStatus represents the status of a campaign phase
type PhaseStatus string

const (
	PhaseStatusPending   PhaseStatus = "pending"
	PhaseStatusRunning   PhaseStatus = "running"
	PhaseStatusSuccess   PhaseStatus = "success"
	PhaseStatusFailed    PhaseStatus = "failed"
	PhaseStatusCancelled PhaseStatus = "cancelled"
)

// CampaignPhase represents a phase in a campaign
type CampaignPhase struct {
	ID           string      `gorm:"primaryKey;size:64" json:"id"`
	CampaignID   string      `gorm:"size:64;not null;index" json:"campaign_id"`
	PhaseName    string      `gorm:"size:64;not null" json:"phase_name"`
	PhaseOrder   int         `gorm:"not null" json:"phase_order"`
	TargetCount  int         `gorm:"default:0" json:"target_count"`
	SuccessCount int         `gorm:"default:0" json:"success_count"`
	FailureCount int         `gorm:"default:0" json:"failure_count"`
	Status       PhaseStatus `gorm:"type:enum('pending','running','success','failed','cancelled');default:'pending'" json:"status"`
	StartedAt    *time.Time  `json:"started_at,omitempty"`
	CompletedAt  *time.Time  `json:"completed_at,omitempty"`

	Campaign Campaign `gorm:"foreignKey:CampaignID" json:"campaign,omitempty"`
}

// TableName returns the table name for CampaignPhase
func (CampaignPhase) TableName() string {
	return "campaign_phases"
}

// SuccessRate returns the success rate of the phase
func (p *CampaignPhase) SuccessRate() float64 {
	total := p.SuccessCount + p.FailureCount
	if total == 0 {
		return 0
	}
	return float64(p.SuccessCount) / float64(total) * 100
}

// IsComplete returns true if the phase is complete
func (p *CampaignPhase) IsComplete() bool {
	switch p.Status {
	case PhaseStatusSuccess, PhaseStatusFailed, PhaseStatusCancelled:
		return true
	default:
		return false
	}
}

// PhaseConfig represents the configuration for a campaign phase
type PhaseConfig struct {
	Name             string  `json:"name"`
	Percentage       float64 `json:"percentage"`
	SuccessThreshold float64 `json:"success_threshold"`
	WaitMinutes      int     `json:"wait_minutes"`
}

// CampaignProgress represents the progress of a campaign
type CampaignProgress struct {
	CurrentPhase     string  `json:"current_phase"`
	TotalAgents      int     `json:"total_agents"`
	CompletedAgents  int     `json:"completed_agents"`
	SuccessfulAgents int     `json:"successful_agents"`
	FailedAgents     int     `json:"failed_agents"`
	SuccessRate      float64 `json:"success_rate"`
}
