// Package agent provides agent management for the control plane.
package agent

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/yourorg/control-plane/pkg/db/models"
)

// KeyManager manages installation keys
type KeyManager struct {
	db     *gorm.DB
	logger *zap.Logger
}

// NewKeyManager creates a new key manager
func NewKeyManager(db *gorm.DB, logger *zap.Logger) *KeyManager {
	return &KeyManager{
		db:     db,
		logger: logger,
	}
}

// CreateKeyRequest represents a request to create an installation key
type CreateKeyRequest struct {
	TenantID    string                 `json:"tenant_id" binding:"required"`
	Description string                 `json:"description"`
	Tags        map[string]interface{} `json:"tags"`
	ExpiryHours int                    `json:"expiry_hours"`
	UsageLimit  int                    `json:"usage_limit"`
}

// CreateKeyResponse represents the response when creating a key
type CreateKeyResponse struct {
	KeyID     string    `json:"key_id"`
	Key       string    `json:"key"` // Only returned once at creation
	ExpiresAt time.Time `json:"expires_at"`
}

// CreateKey creates a new installation key
func (m *KeyManager) CreateKey(ctx context.Context, req *CreateKeyRequest) (*CreateKeyResponse, error) {
	expiryHours := req.ExpiryHours
	if expiryHours == 0 {
		expiryHours = 24 // Default 24 hours
	}

	usageLimit := req.UsageLimit
	if usageLimit == 0 {
		usageLimit = 1 // Default single use
	}

	key, rawKey, err := models.NewInstallationKey(
		req.TenantID,
		req.Description,
		req.Tags,
		expiryHours,
		usageLimit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}

	if err := m.db.Create(key).Error; err != nil {
		return nil, fmt.Errorf("failed to store key: %w", err)
	}

	m.logger.Info("installation key created",
		zap.String("key_id", key.ID),
		zap.String("tenant_id", req.TenantID),
		zap.Time("expires_at", key.ExpiresAt))

	return &CreateKeyResponse{
		KeyID:     key.ID,
		Key:       rawKey,
		ExpiresAt: key.ExpiresAt,
	}, nil
}

// GetKey retrieves a key by ID
func (m *KeyManager) GetKey(ctx context.Context, tenantID, keyID string) (*models.InstallationKey, error) {
	var key models.InstallationKey
	if err := m.db.Where("id = ? AND tenant_id = ?", keyID, tenantID).First(&key).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("key not found")
		}
		return nil, fmt.Errorf("failed to get key: %w", err)
	}
	return &key, nil
}

// ListKeys lists installation keys for a tenant
func (m *KeyManager) ListKeys(ctx context.Context, tenantID string, includeExpired bool) ([]models.InstallationKey, error) {
	query := m.db.Model(&models.InstallationKey{}).Where("tenant_id = ?", tenantID)

	if !includeExpired {
		query = query.Where("expires_at > ?", time.Now())
	}

	var keys []models.InstallationKey
	if err := query.Order("created_at DESC").Find(&keys).Error; err != nil {
		return nil, fmt.Errorf("failed to list keys: %w", err)
	}

	return keys, nil
}

// RevokeKey revokes an installation key
func (m *KeyManager) RevokeKey(ctx context.Context, tenantID, keyID string) error {
	// We revoke by setting usage_count to usage_limit
	result := m.db.Model(&models.InstallationKey{}).
		Where("id = ? AND tenant_id = ?", keyID, tenantID).
		Update("usage_count", gorm.Expr("usage_limit"))

	if result.Error != nil {
		return fmt.Errorf("failed to revoke key: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("key not found")
	}

	m.logger.Info("installation key revoked",
		zap.String("key_id", keyID),
		zap.String("tenant_id", tenantID))

	return nil
}

// DeleteKey deletes an installation key
func (m *KeyManager) DeleteKey(ctx context.Context, tenantID, keyID string) error {
	result := m.db.Where("id = ? AND tenant_id = ?", keyID, tenantID).Delete(&models.InstallationKey{})

	if result.Error != nil {
		return fmt.Errorf("failed to delete key: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("key not found")
	}

	return nil
}

// CleanupExpiredKeys removes expired keys
func (m *KeyManager) CleanupExpiredKeys(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)

	result := m.db.Where("expires_at < ?", cutoff).Delete(&models.InstallationKey{})
	if result.Error != nil {
		return 0, fmt.Errorf("failed to cleanup keys: %w", result.Error)
	}

	if result.RowsAffected > 0 {
		m.logger.Info("cleaned up expired keys",
			zap.Int64("count", result.RowsAffected))
	}

	return result.RowsAffected, nil
}

// GetKeyStats returns key usage statistics
func (m *KeyManager) GetKeyStats(ctx context.Context, tenantID string) (*KeyStats, error) {
	stats := &KeyStats{}

	// Total keys
	if err := m.db.Model(&models.InstallationKey{}).Where("tenant_id = ?", tenantID).Count(&stats.Total).Error; err != nil {
		return nil, err
	}

	// Active keys (not expired and not fully used)
	if err := m.db.Model(&models.InstallationKey{}).
		Where("tenant_id = ? AND expires_at > ? AND usage_count < usage_limit", tenantID, time.Now()).
		Count(&stats.Active).Error; err != nil {
		return nil, err
	}

	// Used keys
	if err := m.db.Model(&models.InstallationKey{}).
		Where("tenant_id = ? AND usage_count > 0", tenantID).
		Count(&stats.Used).Error; err != nil {
		return nil, err
	}

	// Expired keys
	if err := m.db.Model(&models.InstallationKey{}).
		Where("tenant_id = ? AND expires_at < ?", tenantID, time.Now()).
		Count(&stats.Expired).Error; err != nil {
		return nil, err
	}

	return stats, nil
}

// KeyStats contains key statistics
type KeyStats struct {
	Total   int64 `json:"total"`
	Active  int64 `json:"active"`
	Used    int64 `json:"used"`
	Expired int64 `json:"expired"`
}
