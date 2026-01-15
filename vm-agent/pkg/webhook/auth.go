// Package webhook provides HTTP webhook server functionality.
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Authenticator authenticates incoming webhook requests
type Authenticator struct {
	jwtSecret     []byte
	hmacSecret    []byte
	allowedTokens map[string]bool
}

// AuthConfig contains authentication configuration
type AuthConfig struct {
	JWTSecret     string
	HMACSecret    string
	AllowedTokens []string
}

// NewAuthenticator creates a new authenticator
func NewAuthenticator(cfg *AuthConfig) *Authenticator {
	auth := &Authenticator{
		allowedTokens: make(map[string]bool),
	}

	if cfg.JWTSecret != "" {
		auth.jwtSecret = []byte(cfg.JWTSecret)
	}

	if cfg.HMACSecret != "" {
		auth.hmacSecret = []byte(cfg.HMACSecret)
	}

	for _, token := range cfg.AllowedTokens {
		auth.allowedTokens[token] = true
	}

	return auth
}

// Authenticate authenticates a request
func (a *Authenticator) Authenticate(r *http.Request) bool {
	// Try JWT authentication
	if a.jwtSecret != nil {
		if a.authenticateJWT(r) {
			return true
		}
	}

	// Try HMAC authentication
	if a.hmacSecret != nil {
		if a.authenticateHMAC(r) {
			return true
		}
	}

	// Try token authentication
	if len(a.allowedTokens) > 0 {
		if a.authenticateToken(r) {
			return true
		}
	}

	// If no authentication methods configured, allow all
	if a.jwtSecret == nil && a.hmacSecret == nil && len(a.allowedTokens) == 0 {
		return true
	}

	return false
}

// authenticateJWT verifies JWT tokens
func (a *Authenticator) authenticateJWT(r *http.Request) bool {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return false
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return false
	}

	tokenString := parts[1]

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return a.jwtSecret, nil
	})

	if err != nil || !token.Valid {
		return false
	}

	// Check expiration
	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		if exp, ok := claims["exp"].(float64); ok {
			if time.Unix(int64(exp), 0).Before(time.Now()) {
				return false
			}
		}
	}

	return true
}

// authenticateHMAC verifies HMAC signatures
func (a *Authenticator) authenticateHMAC(r *http.Request) bool {
	signature := r.Header.Get("X-Signature-256")
	if signature == "" {
		return false
	}

	// Remove "sha256=" prefix if present
	signature = strings.TrimPrefix(signature, "sha256=")

	// Get request body for signature verification
	// Note: This requires the body to be read and stored earlier
	body := r.Header.Get("X-Original-Body-Hash")
	if body == "" {
		return false
	}

	mac := hmac.New(sha256.New, a.hmacSecret)
	mac.Write([]byte(body))
	expectedMAC := hex.EncodeToString(mac.Sum(nil))

	return subtle.ConstantTimeCompare([]byte(signature), []byte(expectedMAC)) == 1
}

// authenticateToken verifies static tokens
func (a *Authenticator) authenticateToken(r *http.Request) bool {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return false
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 {
		return false
	}

	tokenType := strings.ToLower(parts[0])
	token := parts[1]

	if tokenType != "bearer" && tokenType != "token" {
		return false
	}

	return a.allowedTokens[token]
}

// GenerateJWT generates a new JWT token
func (a *Authenticator) GenerateJWT(claims map[string]interface{}, expiry time.Duration) (string, error) {
	now := time.Now()
	jwtClaims := jwt.MapClaims{
		"iat": now.Unix(),
		"exp": now.Add(expiry).Unix(),
	}

	for k, v := range claims {
		jwtClaims[k] = v
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims)
	return token.SignedString(a.jwtSecret)
}

// GenerateHMAC generates an HMAC signature for data
func (a *Authenticator) GenerateHMAC(data []byte) string {
	mac := hmac.New(sha256.New, a.hmacSecret)
	mac.Write(data)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// TokenClaims represents JWT token claims
type TokenClaims struct {
	TenantID string `json:"tenant_id"`
	AgentID  string `json:"agent_id"`
	Scope    string `json:"scope"`
}

// ParseTokenClaims extracts claims from a JWT token
func (a *Authenticator) ParseTokenClaims(r *http.Request) (*TokenClaims, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, jwt.ErrTokenMalformed
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return nil, jwt.ErrTokenMalformed
	}

	tokenString := parts[1]

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return a.jwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, jwt.ErrTokenMalformed
	}

	tc := &TokenClaims{}
	if v, ok := claims["tenant_id"].(string); ok {
		tc.TenantID = v
	}
	if v, ok := claims["agent_id"].(string); ok {
		tc.AgentID = v
	}
	if v, ok := claims["scope"].(string); ok {
		tc.Scope = v
	}

	return tc, nil
}

// Middleware returns an HTTP middleware for authentication
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.Authenticate(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
