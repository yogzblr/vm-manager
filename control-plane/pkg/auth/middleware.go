// Package auth provides authentication utilities for the control plane.
package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/yourorg/control-plane/pkg/db/models"
)

// contextKey is a custom type for context keys
type contextKey string

const (
	ContextKeyClaims   contextKey = "claims"
	ContextKeyTenantID contextKey = "tenant_id"
	ContextKeyAgentID  contextKey = "agent_id"
	ContextKeyUserID   contextKey = "user_id"
)

// Middleware provides authentication middleware
type Middleware struct {
	jwtManager *JWTManager
	db         *gorm.DB
	logger     *zap.Logger
}

// NewMiddleware creates a new authentication middleware
func NewMiddleware(jwtManager *JWTManager, db *gorm.DB, logger *zap.Logger) *Middleware {
	return &Middleware{
		jwtManager: jwtManager,
		db:         db,
		logger:     logger,
	}
}

// Authenticate returns a Gin middleware for JWT authentication
func (m *Middleware) Authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := m.extractToken(c)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing authorization token",
			})
			return
		}

		claims, err := m.jwtManager.ValidateToken(token)
		if err != nil {
			m.logger.Debug("token validation failed",
				zap.Error(err))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid token",
			})
			return
		}

		// Verify tenant exists and is active
		if claims.TenantID != "" {
			var tenant models.Tenant
			if err := m.db.Where("id = ? AND status = ?", claims.TenantID, models.TenantStatusActive).First(&tenant).Error; err != nil {
				m.logger.Debug("tenant not found or inactive",
					zap.String("tenant_id", claims.TenantID),
					zap.Error(err))
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"error": "tenant not found or suspended",
				})
				return
			}
		}

		// Set claims in context
		c.Set(string(ContextKeyClaims), claims)
		c.Set(string(ContextKeyTenantID), claims.TenantID)
		if claims.AgentID != "" {
			c.Set(string(ContextKeyAgentID), claims.AgentID)
		}
		if claims.UserID != "" {
			c.Set(string(ContextKeyUserID), claims.UserID)
		}

		c.Next()
	}
}

// AuthenticateAgent returns middleware specifically for agent authentication
func (m *Middleware) AuthenticateAgent() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := m.extractToken(c)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing authorization token",
			})
			return
		}

		claims, err := m.jwtManager.ValidateToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid token",
			})
			return
		}

		if claims.Type != "agent" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "agent token required",
			})
			return
		}

		// Verify agent exists
		var agent models.Agent
		if err := m.db.Where("id = ? AND tenant_id = ?", claims.AgentID, claims.TenantID).First(&agent).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "agent not found",
			})
			return
		}

		c.Set(string(ContextKeyClaims), claims)
		c.Set(string(ContextKeyTenantID), claims.TenantID)
		c.Set(string(ContextKeyAgentID), claims.AgentID)

		c.Next()
	}
}

// RequireScopes returns middleware that requires specific scopes
func (m *Middleware) RequireScopes(requiredScopes ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, exists := c.Get(string(ContextKeyClaims))
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "authentication required",
			})
			return
		}

		authClaims := claims.(*Claims)

		// Check if user has all required scopes
		for _, required := range requiredScopes {
			found := false
			for _, scope := range authClaims.Scopes {
				if scope == required || scope == "*" {
					found = true
					break
				}
			}
			if !found {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error":          "insufficient permissions",
					"required_scope": required,
				})
				return
			}
		}

		c.Next()
	}
}

// RequireTenant returns middleware that requires tenant context
func (m *Middleware) RequireTenant() gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID, exists := c.Get(string(ContextKeyTenantID))
		if !exists || tenantID == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "tenant context required",
			})
			return
		}

		c.Next()
	}
}

// extractToken extracts the token from the request
func (m *Middleware) extractToken(c *gin.Context) string {
	// Try Authorization header first
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
			return parts[1]
		}
	}

	// Try X-API-Key header
	apiKey := c.GetHeader("X-API-Key")
	if apiKey != "" {
		return apiKey
	}

	// Try query parameter
	return c.Query("token")
}

// APIKeyAuth returns middleware for API key authentication
func (m *Middleware) APIKeyAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "API key required",
			})
			return
		}

		// Hash the key and look it up
		keyHash := HashToken(apiKey)

		var tenantKey models.TenantAPIKey
		if err := m.db.Where("key_hash = ? AND (expires_at IS NULL OR expires_at > NOW()) AND revoked_at IS NULL", keyHash).First(&tenantKey).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid API key",
			})
			return
		}

		// Update last used
		m.db.Model(&tenantKey).Update("last_used_at", "NOW()")

		// Verify tenant is active
		var tenant models.Tenant
		if err := m.db.Where("id = ? AND status = ?", tenantKey.TenantID, models.TenantStatusActive).First(&tenant).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "tenant not found or suspended",
			})
			return
		}

		// Set context
		c.Set(string(ContextKeyTenantID), tenantKey.TenantID)

		c.Next()
	}
}

// GetClaimsFromContext extracts claims from context
func GetClaimsFromContext(ctx context.Context) *Claims {
	if claims, ok := ctx.Value(ContextKeyClaims).(*Claims); ok {
		return claims
	}
	return nil
}

// GetTenantIDFromContext extracts tenant ID from context
func GetTenantIDFromContext(ctx context.Context) string {
	if tenantID, ok := ctx.Value(ContextKeyTenantID).(string); ok {
		return tenantID
	}
	return ""
}

// GetAgentIDFromContext extracts agent ID from context
func GetAgentIDFromContext(ctx context.Context) string {
	if agentID, ok := ctx.Value(ContextKeyAgentID).(string); ok {
		return agentID
	}
	return ""
}

// GetClaimsFromGin extracts claims from Gin context
func GetClaimsFromGin(c *gin.Context) *Claims {
	if claims, exists := c.Get(string(ContextKeyClaims)); exists {
		return claims.(*Claims)
	}
	return nil
}

// GetTenantIDFromGin extracts tenant ID from Gin context
func GetTenantIDFromGin(c *gin.Context) string {
	if tenantID, exists := c.Get(string(ContextKeyTenantID)); exists {
		return tenantID.(string)
	}
	return ""
}
