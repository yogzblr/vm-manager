// Package models contains database models for the control plane.
package models

import (
	"time"
)

// TemplateStatus represents the status of a template
type TemplateStatus string

const (
	TemplateStatusDraft      TemplateStatus = "draft"
	TemplateStatusActive     TemplateStatus = "active"
	TemplateStatusDeprecated TemplateStatus = "deprecated"
	TemplateStatusDeleted    TemplateStatus = "deleted"
)

// Template represents a configuration template (like Salt Stack templates)
type Template struct {
	ID          string         `gorm:"primaryKey;size:64" json:"id"`
	TenantID    string         `gorm:"size:64;not null;index" json:"tenant_id"`
	Name        string         `gorm:"size:255;not null" json:"name"`
	Description string         `gorm:"type:text" json:"description,omitempty"`
	Content     string         `gorm:"type:longtext;not null" json:"content"`
	ContentType string         `gorm:"size:100;default:'text/plain'" json:"content_type"`
	Version     int            `gorm:"default:1" json:"version"`
	Status      TemplateStatus `gorm:"type:enum('draft','active','deprecated','deleted');default:'draft'" json:"status"`
	Tags        JSONMap        `gorm:"type:json" json:"tags,omitempty"`
	Metadata    JSONMap        `gorm:"type:json" json:"metadata,omitempty"`
	CreatedBy   string         `gorm:"size:255" json:"created_by,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`

	// Relationships
	Tenant Tenant `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
}

// TableName returns the table name for Template
func (Template) TableName() string {
	return "templates"
}

// TemplateVersion represents a version history entry for a template
type TemplateVersion struct {
	ID         string    `gorm:"primaryKey;size:64" json:"id"`
	TemplateID string    `gorm:"size:64;not null;index" json:"template_id"`
	TenantID   string    `gorm:"size:64;not null;index" json:"tenant_id"`
	Version    int       `gorm:"not null" json:"version"`
	Content    string    `gorm:"type:longtext;not null" json:"content"`
	ChangedBy  string    `gorm:"size:255" json:"changed_by,omitempty"`
	ChangeNote string    `gorm:"type:text" json:"change_note,omitempty"`
	CreatedAt  time.Time `json:"created_at"`

	// Relationships
	Template Template `gorm:"foreignKey:TemplateID" json:"template,omitempty"`
	Tenant   Tenant   `gorm:"foreignKey:TenantID" json:"tenant,omitempty"`
}

// TableName returns the table name for TemplateVersion
func (TemplateVersion) TableName() string {
	return "template_versions"
}
