// Package models contains database models for the control plane.
package models

import (
	"time"
)

// AgentStatus represents the status of an agent
type AgentStatus string

const (
	AgentStatusOnline   AgentStatus = "online"
	AgentStatusOffline  AgentStatus = "offline"
	AgentStatusDegraded AgentStatus = "degraded"
	AgentStatusUnknown  AgentStatus = "unknown"
)

// Agent represents a registered agent
type Agent struct {
	ID           string       `gorm:"primaryKey;size:64" json:"id"`
	TenantID     string       `gorm:"size:64;not null;index" json:"tenant_id"`
	Hostname     string       `gorm:"size:255;not null" json:"hostname"`
	OS           string       `gorm:"size:64" json:"os,omitempty"`
	Arch         string       `gorm:"size:64" json:"arch,omitempty"`
	Version      string       `gorm:"size:64" json:"version,omitempty"`
	Status       AgentStatus  `gorm:"type:enum('online','offline','degraded','unknown');default:'unknown'" json:"status"`
	Tags         JSONMap      `gorm:"type:json" json:"tags,omitempty"`
	Metadata     JSONMap      `gorm:"type:json" json:"metadata,omitempty"`
	LastSeenAt   *time.Time   `json:"last_seen_at,omitempty"`
	RegisteredAt time.Time    `json:"registered_at"`
	UpdatedAt    time.Time    `json:"updated_at"`

	// Relationships
	Tenant       Tenant          `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
	Tokens       []AgentToken    `gorm:"foreignKey:AgentID" json:"tokens,omitempty"`
	Executions   []WorkflowExecution `gorm:"foreignKey:AgentID" json:"executions,omitempty"`
	HealthReports []AgentHealthReport `gorm:"foreignKey:AgentID" json:"health_reports,omitempty"`
}

// TableName returns the table name for Agent
func (Agent) TableName() string {
	return "agents"
}

// AgentToken represents a JWT token for agent authentication
type AgentToken struct {
	ID        string     `gorm:"primaryKey;size:64" json:"id"`
	AgentID   string     `gorm:"size:64;not null;index" json:"agent_id"`
	TenantID  string     `gorm:"size:64;not null;index" json:"tenant_id"`
	TokenHash string     `gorm:"size:255;not null" json:"-"`
	ExpiresAt time.Time  `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`

	Agent  Agent  `gorm:"foreignKey:AgentID" json:"agent,omitempty"`
	Tenant Tenant `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
}

// TableName returns the table name for AgentToken
func (AgentToken) TableName() string {
	return "agent_tokens"
}

// AgentHealthReport represents a health report from an agent
type AgentHealthReport struct {
	ID         string      `gorm:"primaryKey;size:64" json:"id"`
	AgentID    string      `gorm:"size:64;not null;index" json:"agent_id"`
	TenantID   string      `gorm:"size:64;not null;index" json:"tenant_id"`
	Status     AgentStatus `gorm:"type:enum('healthy','degraded','unhealthy','unknown');not null" json:"status"`
	Components JSONMap     `gorm:"type:json" json:"components,omitempty"`
	ReportedAt time.Time   `json:"reported_at"`

	Agent  Agent  `gorm:"foreignKey:AgentID" json:"agent,omitempty"`
	Tenant Tenant `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
}

// TableName returns the table name for AgentHealthReport
func (AgentHealthReport) TableName() string {
	return "agent_health_reports"
}

// InstallationKey represents a one-time installation key
type InstallationKey struct {
	ID          string     `gorm:"primaryKey;size:64" json:"id"`
	TenantID    string     `gorm:"size:64;not null;index" json:"tenant_id"`
	KeyHash     string     `gorm:"size:255;not null" json:"-"`
	Description string     `gorm:"type:text" json:"description,omitempty"`
	Tags        JSONMap    `gorm:"type:json" json:"tags,omitempty"`
	UsageLimit  int        `gorm:"default:1" json:"usage_limit"`
	UsageCount  int        `gorm:"default:0" json:"usage_count"`
	ExpiresAt   time.Time  `json:"expires_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UsedAt      *time.Time `json:"used_at,omitempty"`

	Tenant Tenant `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
}

// TableName returns the table name for InstallationKey
func (InstallationKey) TableName() string {
	return "installation_keys"
}

// IsValid returns true if the installation key is valid
func (k *InstallationKey) IsValid() bool {
	if k.UsageCount >= k.UsageLimit {
		return false
	}
	if time.Now().After(k.ExpiresAt) {
		return false
	}
	return true
}
