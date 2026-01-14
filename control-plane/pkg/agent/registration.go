// Package agent provides agent management for the control plane.
package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/yourorg/control-plane/pkg/auth"
	"github.com/yourorg/control-plane/pkg/db/models"
	"github.com/yourorg/control-plane/pkg/tenant"
)

// RegistrationService handles agent registration
type RegistrationService struct {
	db           *gorm.DB
	jwtManager   *auth.JWTManager
	quotaChecker *tenant.QuotaChecker
	logger       *zap.Logger
	tokenExpiry  time.Duration
}

// NewRegistrationService creates a new registration service
func NewRegistrationService(db *gorm.DB, jwtManager *auth.JWTManager, logger *zap.Logger) *RegistrationService {
	return &RegistrationService{
		db:           db,
		jwtManager:   jwtManager,
		quotaChecker: tenant.NewQuotaChecker(db),
		logger:       logger,
		tokenExpiry:  365 * 24 * time.Hour, // 1 year
	}
}

// RegisterRequest represents an agent registration request
type RegisterRequest struct {
	InstallationKey string                 `json:"installation_key" binding:"required"`
	AgentID         string                 `json:"agent_id"`
	Hostname        string                 `json:"hostname" binding:"required"`
	OS              string                 `json:"os"`
	Arch            string                 `json:"arch"`
	Version         string                 `json:"version"`
	Tags            map[string]interface{} `json:"tags"`
}

// RegisterResponse represents the registration response
type RegisterResponse struct {
	Token    string `json:"token"`
	AgentID  string `json:"agent_id"`
	TenantID string `json:"tenant_id"`
	Endpoint string `json:"endpoint"`
}

// Register registers a new agent
func (s *RegistrationService) Register(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error) {
	// Validate installation key
	tenantID, err := s.validateInstallationKey(req.InstallationKey)
	if err != nil {
		return nil, fmt.Errorf("invalid installation key: %w", err)
	}

	// Check quota
	if err := s.quotaChecker.CheckAgentQuota(tenantID); err != nil {
		return nil, err
	}

	// Generate agent ID if not provided
	agentID := req.AgentID
	if agentID == "" {
		agentID = req.Hostname
	}

	// Check if agent already exists
	var existingAgent models.Agent
	if err := s.db.Where("id = ? AND tenant_id = ?", agentID, tenantID).First(&existingAgent).Error; err == nil {
		// Agent exists, update and return new token
		return s.reRegisterAgent(ctx, &existingAgent, req)
	}

	// Create new agent
	agent := &models.Agent{
		ID:           agentID,
		TenantID:     tenantID,
		Hostname:     req.Hostname,
		OS:           req.OS,
		Arch:         req.Arch,
		Version:      req.Version,
		Status:       models.AgentStatusUnknown,
		Tags:         req.Tags,
		RegisteredAt: time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.db.Create(agent).Error; err != nil {
		return nil, fmt.Errorf("failed to create agent: %w", err)
	}

	// Generate token
	token, err := s.jwtManager.GenerateAgentToken(tenantID, agentID, s.tokenExpiry)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	// Store token hash
	tokenHash := auth.HashToken(token)
	agentToken := &models.AgentToken{
		ID:        uuid.New().String(),
		AgentID:   agentID,
		TenantID:  tenantID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(s.tokenExpiry),
		CreatedAt: time.Now(),
	}

	if err := s.db.Create(agentToken).Error; err != nil {
		return nil, fmt.Errorf("failed to store token: %w", err)
	}

	// Mark installation key as used
	s.markKeyUsed(req.InstallationKey)

	s.logger.Info("agent registered",
		zap.String("agent_id", agentID),
		zap.String("tenant_id", tenantID),
		zap.String("hostname", req.Hostname))

	return &RegisterResponse{
		Token:    token,
		AgentID:  agentID,
		TenantID: tenantID,
		Endpoint: fmt.Sprintf("tenant-%s/%s", tenantID, agentID),
	}, nil
}

// reRegisterAgent handles re-registration of an existing agent
func (s *RegistrationService) reRegisterAgent(ctx context.Context, agent *models.Agent, req *RegisterRequest) (*RegisterResponse, error) {
	// Update agent info
	updates := map[string]interface{}{
		"hostname":   req.Hostname,
		"os":         req.OS,
		"arch":       req.Arch,
		"version":    req.Version,
		"updated_at": time.Now(),
	}
	if req.Tags != nil {
		updates["tags"] = req.Tags
	}

	if err := s.db.Model(agent).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update agent: %w", err)
	}

	// Revoke old tokens
	s.db.Model(&models.AgentToken{}).Where("agent_id = ? AND revoked_at IS NULL", agent.ID).Update("revoked_at", time.Now())

	// Generate new token
	token, err := s.jwtManager.GenerateAgentToken(agent.TenantID, agent.ID, s.tokenExpiry)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	// Store new token hash
	tokenHash := auth.HashToken(token)
	agentToken := &models.AgentToken{
		ID:        uuid.New().String(),
		AgentID:   agent.ID,
		TenantID:  agent.TenantID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(s.tokenExpiry),
		CreatedAt: time.Now(),
	}

	if err := s.db.Create(agentToken).Error; err != nil {
		return nil, fmt.Errorf("failed to store token: %w", err)
	}

	s.logger.Info("agent re-registered",
		zap.String("agent_id", agent.ID),
		zap.String("tenant_id", agent.TenantID))

	return &RegisterResponse{
		Token:    token,
		AgentID:  agent.ID,
		TenantID: agent.TenantID,
		Endpoint: fmt.Sprintf("tenant-%s/%s", agent.TenantID, agent.ID),
	}, nil
}

// validateInstallationKey validates an installation key and returns the tenant ID
func (s *RegistrationService) validateInstallationKey(key string) (string, error) {
	keyHash := auth.HashToken(key)

	var installKey models.InstallationKey
	if err := s.db.Where("key_hash = ?", keyHash).First(&installKey).Error; err != nil {
		return "", fmt.Errorf("key not found")
	}

	if !installKey.IsValid() {
		return "", fmt.Errorf("key is expired or usage limit exceeded")
	}

	return installKey.TenantID, nil
}

// markKeyUsed marks an installation key as used
func (s *RegistrationService) markKeyUsed(key string) {
	keyHash := auth.HashToken(key)
	s.db.Model(&models.InstallationKey{}).
		Where("key_hash = ?", keyHash).
		Updates(map[string]interface{}{
			"usage_count": gorm.Expr("usage_count + 1"),
			"used_at":     time.Now(),
		})
}

// Deregister deregisters an agent
func (s *RegistrationService) Deregister(ctx context.Context, tenantID, agentID string) error {
	// Revoke all tokens
	s.db.Model(&models.AgentToken{}).Where("agent_id = ? AND tenant_id = ?", agentID, tenantID).Update("revoked_at", time.Now())

	// Delete agent
	result := s.db.Where("id = ? AND tenant_id = ?", agentID, tenantID).Delete(&models.Agent{})
	if result.Error != nil {
		return fmt.Errorf("failed to deregister agent: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("agent not found")
	}

	s.logger.Info("agent deregistered",
		zap.String("agent_id", agentID),
		zap.String("tenant_id", tenantID))

	return nil
}
