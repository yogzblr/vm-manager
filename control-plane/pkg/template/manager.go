// Package template provides template management for the control plane.
package template

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/yourorg/control-plane/pkg/db/models"
)

// Manager manages templates
type Manager struct {
	db     *gorm.DB
	logger *zap.Logger
}

// NewManager creates a new template manager
func NewManager(db *gorm.DB, logger *zap.Logger) *Manager {
	return &Manager{
		db:     db,
		logger: logger,
	}
}

// CreateTemplateRequest represents a request to create a template
type CreateTemplateRequest struct {
	TenantID    string                 `json:"tenant_id" binding:"required"`
	Name        string                 `json:"name" binding:"required"`
	Description string                 `json:"description"`
	Content     string                 `json:"content" binding:"required"`
	ContentType string                 `json:"content_type"`
	Tags        map[string]interface{} `json:"tags"`
	Metadata    map[string]interface{} `json:"metadata"`
	CreatedBy   string                 `json:"created_by"`
}

// Create creates a new template
func (m *Manager) Create(ctx context.Context, req *CreateTemplateRequest) (*models.Template, error) {
	// Validate template content (basic validation)
	if len(req.Content) == 0 {
		return nil, fmt.Errorf("template content cannot be empty")
	}

	contentType := req.ContentType
	if contentType == "" {
		contentType = "text/plain"
	}

	template := &models.Template{
		ID:          uuid.New().String(),
		TenantID:    req.TenantID,
		Name:        req.Name,
		Description: req.Description,
		Content:     req.Content,
		ContentType: contentType,
		Version:     1,
		Status:      models.TemplateStatusDraft,
		Tags:        req.Tags,
		Metadata:    req.Metadata,
		CreatedBy:   req.CreatedBy,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := m.db.Create(template).Error; err != nil {
		return nil, fmt.Errorf("failed to create template: %w", err)
	}

	// Create initial version record
	version := &models.TemplateVersion{
		ID:         uuid.New().String(),
		TemplateID: template.ID,
		TenantID:   req.TenantID,
		Version:    1,
		Content:    req.Content,
		ChangedBy:  req.CreatedBy,
		ChangeNote: "Initial version",
		CreatedAt:  time.Now(),
	}

	if err := m.db.Create(version).Error; err != nil {
		m.logger.Warn("failed to create version record", zap.Error(err))
	}

	m.logger.Info("template created",
		zap.String("template_id", template.ID),
		zap.String("tenant_id", req.TenantID),
		zap.String("name", req.Name))

	return template, nil
}

// Get retrieves a template by ID
func (m *Manager) Get(ctx context.Context, tenantID, templateID string) (*models.Template, error) {
	var template models.Template
	if err := m.db.Where("id = ? AND tenant_id = ?", templateID, tenantID).First(&template).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("template not found")
		}
		return nil, fmt.Errorf("failed to get template: %w", err)
	}
	return &template, nil
}

// GetByName retrieves a template by name
func (m *Manager) GetByName(ctx context.Context, tenantID, name string) (*models.Template, error) {
	var template models.Template
	if err := m.db.Where("name = ? AND tenant_id = ? AND status != ?", name, tenantID, models.TemplateStatusDeleted).First(&template).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("template not found")
		}
		return nil, fmt.Errorf("failed to get template: %w", err)
	}
	return &template, nil
}

// GetContent retrieves only the template content
func (m *Manager) GetContent(ctx context.Context, tenantID, templateID string) (string, error) {
	template, err := m.Get(ctx, tenantID, templateID)
	if err != nil {
		return "", err
	}
	return template.Content, nil
}

// UpdateTemplateRequest represents a request to update a template
type UpdateTemplateRequest struct {
	Name        *string                 `json:"name"`
	Description *string                 `json:"description"`
	Content     *string                 `json:"content"`
	ContentType *string                 `json:"content_type"`
	Status      *models.TemplateStatus  `json:"status"`
	Tags        map[string]interface{}  `json:"tags"`
	Metadata    map[string]interface{}  `json:"metadata"`
	ChangedBy   string                  `json:"changed_by"`
	ChangeNote  string                  `json:"change_note"`
}

// Update updates a template
func (m *Manager) Update(ctx context.Context, tenantID, templateID string, req *UpdateTemplateRequest) (*models.Template, error) {
	template, err := m.Get(ctx, tenantID, templateID)
	if err != nil {
		return nil, err
	}

	updates := make(map[string]interface{})
	contentChanged := false

	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Content != nil && *req.Content != template.Content {
		updates["content"] = *req.Content
		updates["version"] = template.Version + 1
		contentChanged = true
	}
	if req.ContentType != nil {
		updates["content_type"] = *req.ContentType
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if req.Tags != nil {
		updates["tags"] = req.Tags
	}
	if req.Metadata != nil {
		updates["metadata"] = req.Metadata
	}

	if len(updates) == 0 {
		return template, nil
	}

	updates["updated_at"] = time.Now()

	if err := m.db.Model(template).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update template: %w", err)
	}

	// Create version record if content changed
	if contentChanged {
		version := &models.TemplateVersion{
			ID:         uuid.New().String(),
			TemplateID: templateID,
			TenantID:   tenantID,
			Version:    template.Version + 1,
			Content:    *req.Content,
			ChangedBy:  req.ChangedBy,
			ChangeNote: req.ChangeNote,
			CreatedAt:  time.Now(),
		}

		if err := m.db.Create(version).Error; err != nil {
			m.logger.Warn("failed to create version record", zap.Error(err))
		}
	}

	m.logger.Info("template updated",
		zap.String("template_id", templateID),
		zap.String("tenant_id", tenantID))

	return m.Get(ctx, tenantID, templateID)
}

// Delete soft-deletes a template
func (m *Manager) Delete(ctx context.Context, tenantID, templateID string) error {
	result := m.db.Model(&models.Template{}).
		Where("id = ? AND tenant_id = ?", templateID, tenantID).
		Update("status", models.TemplateStatusDeleted)

	if result.Error != nil {
		return fmt.Errorf("failed to delete template: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("template not found")
	}

	m.logger.Info("template deleted",
		zap.String("template_id", templateID),
		zap.String("tenant_id", tenantID))

	return nil
}

// ListTemplatesRequest represents a request to list templates
type ListTemplatesRequest struct {
	TenantID string
	Status   models.TemplateStatus
	Tags     map[string]string
	Limit    int
	Offset   int
}

// List lists templates
func (m *Manager) List(ctx context.Context, req *ListTemplatesRequest) ([]models.Template, int64, error) {
	query := m.db.Model(&models.Template{}).Where("tenant_id = ?", req.TenantID)

	if req.Status != "" {
		query = query.Where("status = ?", req.Status)
	} else {
		query = query.Where("status != ?", models.TemplateStatusDeleted)
	}

	// Tag filtering (simplified - checks if tags JSON contains key-value)
	for key, value := range req.Tags {
		query = query.Where("JSON_EXTRACT(tags, ?) = ?", "$."+key, value)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count templates: %w", err)
	}

	if req.Limit > 0 {
		query = query.Limit(req.Limit)
	}
	if req.Offset > 0 {
		query = query.Offset(req.Offset)
	}

	var templates []models.Template
	if err := query.Order("created_at DESC").Find(&templates).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list templates: %w", err)
	}

	return templates, total, nil
}

// GetVersions retrieves all versions of a template
func (m *Manager) GetVersions(ctx context.Context, tenantID, templateID string) ([]models.TemplateVersion, error) {
	var versions []models.TemplateVersion
	if err := m.db.Where("template_id = ? AND tenant_id = ?", templateID, tenantID).
		Order("version DESC").
		Find(&versions).Error; err != nil {
		return nil, fmt.Errorf("failed to get template versions: %w", err)
	}
	return versions, nil
}

// GetVersion retrieves a specific version of a template
func (m *Manager) GetVersion(ctx context.Context, tenantID, templateID string, version int) (*models.TemplateVersion, error) {
	var templateVersion models.TemplateVersion
	if err := m.db.Where("template_id = ? AND tenant_id = ? AND version = ?", templateID, tenantID, version).
		First(&templateVersion).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("template version not found")
		}
		return nil, fmt.Errorf("failed to get template version: %w", err)
	}
	return &templateVersion, nil
}

// Activate activates a template
func (m *Manager) Activate(ctx context.Context, tenantID, templateID string) error {
	result := m.db.Model(&models.Template{}).
		Where("id = ? AND tenant_id = ? AND status = ?", templateID, tenantID, models.TemplateStatusDraft).
		Update("status", models.TemplateStatusActive)

	if result.Error != nil {
		return fmt.Errorf("failed to activate template: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("template not found or not in draft status")
	}

	return nil
}

// Deprecate deprecates a template
func (m *Manager) Deprecate(ctx context.Context, tenantID, templateID string) error {
	result := m.db.Model(&models.Template{}).
		Where("id = ? AND tenant_id = ? AND status = ?", templateID, tenantID, models.TemplateStatusActive).
		Update("status", models.TemplateStatusDeprecated)

	if result.Error != nil {
		return fmt.Errorf("failed to deprecate template: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("template not found or not active")
	}

	return nil
}
