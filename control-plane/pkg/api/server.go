// Package api provides HTTP API handlers for the control plane.
package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/yourorg/control-plane/pkg/agent"
	"github.com/yourorg/control-plane/pkg/audit"
	"github.com/yourorg/control-plane/pkg/auth"
	"github.com/yourorg/control-plane/pkg/campaign"
	"github.com/yourorg/control-plane/pkg/template"
	"github.com/yourorg/control-plane/pkg/tenant"
	"github.com/yourorg/control-plane/pkg/workflow"
)

// ServerConfig represents server configuration
type ServerConfig struct {
	Host            string        `json:"host" yaml:"host"`
	Port            int           `json:"port" yaml:"port"`
	ReadTimeout     time.Duration `json:"read_timeout" yaml:"read_timeout"`
	WriteTimeout    time.Duration `json:"write_timeout" yaml:"write_timeout"`
	ShutdownTimeout time.Duration `json:"shutdown_timeout" yaml:"shutdown_timeout"`
	Debug           bool          `json:"debug" yaml:"debug"`
	TrustedProxies  []string      `json:"trusted_proxies" yaml:"trusted_proxies"`
}

// DefaultServerConfig returns default server configuration
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Host:            "0.0.0.0",
		Port:            8080,
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    30 * time.Second,
		ShutdownTimeout: 10 * time.Second,
		Debug:           false,
	}
}

// Server represents the HTTP server
type Server struct {
	config   *ServerConfig
	logger   *zap.Logger
	db       *gorm.DB
	router   *gin.Engine
	server   *http.Server
	handlers *Handlers
	jwtAuth  *auth.JWTAuth
}

// Dependencies contains all dependencies needed by the server
type Dependencies struct {
	DB              *gorm.DB
	Logger          *zap.Logger
	JWTAuth         *auth.JWTAuth
	TenantManager   *tenant.Manager
	AgentRegistry   *agent.Registry
	AgentRegistrar  *agent.Registrar
	WorkflowManager *workflow.Manager
	CampaignManager *campaign.Manager
	TemplateManager *template.Manager
	AuditLogger     *audit.Logger
}

// NewServer creates a new HTTP server
func NewServer(config *ServerConfig, deps *Dependencies) *Server {
	if !config.Debug {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(RequestLogger(deps.Logger))

	if len(config.TrustedProxies) > 0 {
		router.SetTrustedProxies(config.TrustedProxies)
	}

	handlers := NewHandlers(
		deps.Logger,
		deps.TenantManager,
		deps.AgentRegistry,
		deps.AgentRegistrar,
		deps.WorkflowManager,
		deps.CampaignManager,
		deps.TemplateManager,
		deps.AuditLogger,
	)

	s := &Server{
		config:   config,
		logger:   deps.Logger,
		db:       deps.DB,
		router:   router,
		handlers: handlers,
		jwtAuth:  deps.JWTAuth,
	}

	s.setupRoutes()

	return s
}

// setupRoutes configures all routes
func (s *Server) setupRoutes() {
	// Health checks (no auth)
	s.router.GET("/health", s.handlers.HealthCheck)
	s.router.GET("/ready", s.handlers.Readiness)

	// API v1 routes
	v1 := s.router.Group("/api/v1")

	// Public routes (agent registration)
	public := v1.Group("")
	{
		public.POST("/agents/register", s.handlers.RegisterAgent)
	}

	// Agent routes (agent auth)
	agentRoutes := v1.Group("/agent")
	agentRoutes.Use(auth.AuthMiddleware(s.jwtAuth))
	agentRoutes.Use(auth.RequireTokenType("agent"))
	{
		agentRoutes.POST("/heartbeat", s.handlers.AgentHeartbeat)
		agentRoutes.POST("/health", s.handlers.AgentHealthReport)
	}

	// Authenticated routes
	authenticated := v1.Group("")
	authenticated.Use(auth.AuthMiddleware(s.jwtAuth))
	{
		// Tenant routes (admin only)
		tenants := authenticated.Group("/tenants")
		tenants.Use(auth.RequireScope("admin"))
		{
			tenants.GET("", s.handlers.ListTenants)
			tenants.POST("", s.handlers.CreateTenant)
			tenants.GET("/:tenant_id", s.handlers.GetTenant)
			tenants.PUT("/:tenant_id", s.handlers.UpdateTenant)
		}

		// Agent management routes
		agents := authenticated.Group("/agents")
		{
			agents.GET("", s.handlers.ListAgents)
			agents.GET("/:agent_id", s.handlers.GetAgent)
			agents.POST("/:agent_id/heartbeat", s.handlers.AgentHeartbeat)
			agents.POST("/:agent_id/health", s.handlers.AgentHealthReport)
		}

		// Workflow routes
		workflows := authenticated.Group("/workflows")
		{
			workflows.GET("", s.handlers.ListWorkflows)
			workflows.POST("", s.handlers.CreateWorkflow)
			workflows.GET("/:workflow_id", s.handlers.GetWorkflow)
			workflows.PUT("/:workflow_id", s.handlers.UpdateWorkflow)
			workflows.DELETE("/:workflow_id", s.handlers.DeleteWorkflow)
		}

		// Campaign routes
		campaigns := authenticated.Group("/campaigns")
		{
			campaigns.GET("", s.handlers.ListCampaigns)
			campaigns.POST("", s.handlers.CreateCampaign)
			campaigns.GET("/:campaign_id", s.handlers.GetCampaign)
			campaigns.POST("/:campaign_id/start", s.handlers.StartCampaign)
			campaigns.POST("/:campaign_id/pause", s.handlers.PauseCampaign)
			campaigns.POST("/:campaign_id/cancel", s.handlers.CancelCampaign)
			campaigns.GET("/:campaign_id/progress", s.handlers.GetCampaignProgress)
		}

		// Template routes (Salt Stack-like template management)
		templates := authenticated.Group("/templates")
		{
			templates.GET("", s.handlers.ListTemplates)
			templates.POST("", s.handlers.CreateTemplate)
			templates.GET("/:template_id", s.handlers.GetTemplate)
			templates.GET("/:template_id/content", s.handlers.GetTemplateContent)
			templates.PUT("/:template_id", s.handlers.UpdateTemplate)
			templates.DELETE("/:template_id", s.handlers.DeleteTemplate)
			templates.GET("/:template_id/versions", s.handlers.GetTemplateVersions)
			templates.POST("/:template_id/activate", s.handlers.ActivateTemplate)
		}
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	s.server = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
	}

	s.logger.Info("starting HTTP server", zap.String("address", addr))

	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("failed to start server: %w", err)
	}

	return nil
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("shutting down HTTP server")

	shutdownCtx, cancel := context.WithTimeout(ctx, s.config.ShutdownTimeout)
	defer cancel()

	return s.server.Shutdown(shutdownCtx)
}

// Router returns the gin router for testing
func (s *Server) Router() *gin.Engine {
	return s.router
}

// RequestLogger returns a gin middleware for logging requests
func RequestLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		fields := []zap.Field{
			zap.Int("status", status),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.Duration("latency", latency),
			zap.String("client_ip", c.ClientIP()),
		}

		if len(c.Errors) > 0 {
			fields = append(fields, zap.String("errors", c.Errors.String()))
		}

		if status >= 500 {
			logger.Error("request completed", fields...)
		} else if status >= 400 {
			logger.Warn("request completed", fields...)
		} else {
			logger.Info("request completed", fields...)
		}
	}
}

// CORS returns a gin middleware for CORS
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
