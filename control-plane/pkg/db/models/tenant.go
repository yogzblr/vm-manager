// Package models contains database models for the control plane.
package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// TenantStatus represents the status of a tenant
type TenantStatus string

const (
	TenantStatusActive    TenantStatus = "active"
	TenantStatusSuspended TenantStatus = "suspended"
	TenantStatusDeleted   TenantStatus = "deleted"
)

// Tenant represents a tenant in the system
type Tenant struct {
	ID          string         `gorm:"primaryKey;size:64" json:"id"`
	Name        string         `gorm:"size:255;uniqueIndex;not null" json:"name"`
	Description string         `gorm:"type:text" json:"description,omitempty"`
	Status      TenantStatus   `gorm:"type:enum('active','suspended','deleted');default:'active'" json:"status"`
	Settings    JSONMap        `gorm:"type:json" json:"settings,omitempty"`
	QuotaAgents int            `gorm:"default:1000" json:"quota_agents"`
	QuotaWorkflows int         `gorm:"default:100" json:"quota_workflows"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`

	// Relationships
	Agents     []Agent     `gorm:"foreignKey:TenantID" json:"agents,omitempty"`
	Workflows  []Workflow  `gorm:"foreignKey:TenantID" json:"workflows,omitempty"`
	Campaigns  []Campaign  `gorm:"foreignKey:TenantID" json:"campaigns,omitempty"`
	APIKeys    []TenantAPIKey `gorm:"foreignKey:TenantID" json:"api_keys,omitempty"`
}

// TableName returns the table name for Tenant
func (Tenant) TableName() string {
	return "tenants"
}

// TenantAPIKey represents an API key for tenant authentication
type TenantAPIKey struct {
	ID         string     `gorm:"primaryKey;size:64" json:"id"`
	TenantID   string     `gorm:"size:64;not null" json:"tenant_id"`
	Name       string     `gorm:"size:255;not null" json:"name"`
	KeyHash    string     `gorm:"size:255;not null" json:"-"`
	Scopes     JSONArray  `gorm:"type:json" json:"scopes,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`

	Tenant Tenant `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
}

// TableName returns the table name for TenantAPIKey
func (TenantAPIKey) TableName() string {
	return "tenant_api_keys"
}

// JSONMap is a custom type for JSON object fields
type JSONMap map[string]interface{}

// Value implements the driver.Valuer interface
func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan implements the sql.Scanner interface
func (j *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, j)
}

// JSONArray is a custom type for JSON array fields
type JSONArray []interface{}

// Value implements the driver.Valuer interface
func (j JSONArray) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan implements the sql.Scanner interface
func (j *JSONArray) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, j)
}

// StringArray is a custom type for JSON string array fields
type StringArray []string

// Value implements the driver.Valuer interface
func (s StringArray) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return json.Marshal(s)
}

// Scan implements the sql.Scanner interface
func (s *StringArray) Scan(value interface{}) error {
	if value == nil {
		*s = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, s)
}
