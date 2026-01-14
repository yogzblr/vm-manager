// Package models contains database models for the control plane.
package models

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"
)

// KeyType represents the type of key
type KeyType string

const (
	KeyTypeAPI          KeyType = "api"
	KeyTypeInstallation KeyType = "installation"
)

// GenerateKey generates a random key
func GenerateKey(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// HashKey creates a SHA256 hash of a key
func HashKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

// KeyGenerator provides key generation utilities
type KeyGenerator struct{}

// NewKeyGenerator creates a new key generator
func NewKeyGenerator() *KeyGenerator {
	return &KeyGenerator{}
}

// GenerateAPIKey generates a new API key
func (g *KeyGenerator) GenerateAPIKey() (key string, hash string, err error) {
	key, err = GenerateKey(32)
	if err != nil {
		return "", "", err
	}
	hash = HashKey(key)
	return key, hash, nil
}

// GenerateInstallationKey generates a new installation key
func (g *KeyGenerator) GenerateInstallationKey() (key string, hash string, err error) {
	key, err = GenerateKey(24)
	if err != nil {
		return "", "", err
	}
	hash = HashKey(key)
	return key, hash, nil
}

// NewInstallationKey creates a new installation key record
func NewInstallationKey(tenantID, description string, tags map[string]interface{}, expiryHours int, usageLimit int) (*InstallationKey, string, error) {
	gen := NewKeyGenerator()
	key, hash, err := gen.GenerateInstallationKey()
	if err != nil {
		return nil, "", err
	}

	id, err := GenerateKey(16)
	if err != nil {
		return nil, "", err
	}

	ik := &InstallationKey{
		ID:          id,
		TenantID:    tenantID,
		KeyHash:     hash,
		Description: description,
		Tags:        tags,
		UsageLimit:  usageLimit,
		UsageCount:  0,
		ExpiresAt:   time.Now().Add(time.Duration(expiryHours) * time.Hour),
		CreatedAt:   time.Now(),
	}

	return ik, key, nil
}

// NewTenantAPIKey creates a new tenant API key record
func NewTenantAPIKey(tenantID, name string, scopes []interface{}, expiryHours int) (*TenantAPIKey, string, error) {
	gen := NewKeyGenerator()
	key, hash, err := gen.GenerateAPIKey()
	if err != nil {
		return nil, "", err
	}

	id, err := GenerateKey(16)
	if err != nil {
		return nil, "", err
	}

	var expiresAt *time.Time
	if expiryHours > 0 {
		exp := time.Now().Add(time.Duration(expiryHours) * time.Hour)
		expiresAt = &exp
	}

	ak := &TenantAPIKey{
		ID:        id,
		TenantID:  tenantID,
		Name:      name,
		KeyHash:   hash,
		Scopes:    scopes,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}

	return ak, key, nil
}
