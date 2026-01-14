// Package auth provides authentication utilities for the control plane.
package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims represents JWT claims
type Claims struct {
	jwt.RegisteredClaims
	TenantID string   `json:"tenant_id"`
	AgentID  string   `json:"agent_id,omitempty"`
	UserID   string   `json:"user_id,omitempty"`
	Scopes   []string `json:"scopes,omitempty"`
	Type     string   `json:"type"` // "user", "agent", "api"
}

// JWTManager manages JWT token operations
type JWTManager struct {
	secret        []byte
	issuer        string
	defaultExpiry time.Duration
}

// NewJWTManager creates a new JWT manager
func NewJWTManager(secret, issuer string, defaultExpiry time.Duration) *JWTManager {
	return &JWTManager{
		secret:        []byte(secret),
		issuer:        issuer,
		defaultExpiry: defaultExpiry,
	}
}

// GenerateAgentToken generates a JWT token for an agent
func (m *JWTManager) GenerateAgentToken(tenantID, agentID string, expiry time.Duration) (string, error) {
	if expiry == 0 {
		expiry = m.defaultExpiry
	}

	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   agentID,
			ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
		TenantID: tenantID,
		AgentID:  agentID,
		Type:     "agent",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// GenerateUserToken generates a JWT token for a user
func (m *JWTManager) GenerateUserToken(tenantID, userID string, scopes []string, expiry time.Duration) (string, error) {
	if expiry == 0 {
		expiry = m.defaultExpiry
	}

	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
		TenantID: tenantID,
		UserID:   userID,
		Scopes:   scopes,
		Type:     "user",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// GenerateAPIToken generates a JWT token for API access
func (m *JWTManager) GenerateAPIToken(tenantID string, scopes []string, expiry time.Duration) (string, error) {
	if expiry == 0 {
		expiry = m.defaultExpiry
	}

	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   tenantID,
			ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
		TenantID: tenantID,
		Scopes:   scopes,
		Type:     "api",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// ValidateToken validates a JWT token and returns the claims
func (m *JWTManager) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return m.secret, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

// RefreshToken refreshes a token with a new expiry
func (m *JWTManager) RefreshToken(tokenString string, expiry time.Duration) (string, error) {
	claims, err := m.ValidateToken(tokenString)
	if err != nil {
		return "", err
	}

	if expiry == 0 {
		expiry = m.defaultExpiry
	}

	now := time.Now()
	claims.ExpiresAt = jwt.NewNumericDate(now.Add(expiry))
	claims.IssuedAt = jwt.NewNumericDate(now)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// HashToken creates a SHA256 hash of a token for storage
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// TokenType represents the type of token
type TokenType string

const (
	TokenTypeAgent TokenType = "agent"
	TokenTypeUser  TokenType = "user"
	TokenTypeAPI   TokenType = "api"
)
