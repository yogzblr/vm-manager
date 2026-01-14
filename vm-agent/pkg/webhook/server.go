// Package webhook provides HTTP webhook server functionality.
package webhook

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Server represents the webhook HTTP server
type Server struct {
	mu         sync.RWMutex
	httpServer *http.Server
	listener   net.Listener
	listenAddr string
	port       int
	tlsEnabled bool
	certFile   string
	keyFile    string
	logger     *zap.Logger
	running    bool
	handlers   *Handlers
	auth       *Authenticator
}

// ServerConfig contains server configuration
type ServerConfig struct {
	ListenAddr string
	Port       int
	TLSEnabled bool
	CertFile   string
	KeyFile    string
}

// NewServer creates a new webhook server
func NewServer(cfg *ServerConfig, handlers *Handlers, auth *Authenticator, logger *zap.Logger) *Server {
	return &Server{
		listenAddr: cfg.ListenAddr,
		port:       cfg.Port,
		tlsEnabled: cfg.TLSEnabled,
		certFile:   cfg.CertFile,
		keyFile:    cfg.KeyFile,
		logger:     logger,
		handlers:   handlers,
		auth:       auth,
	}
}

// Start starts the webhook server
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("server already running")
	}

	addr := fmt.Sprintf("%s:%d", s.listenAddr, s.port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	s.listener = listener

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	s.httpServer = &http.Server{
		Handler:           mux,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		var err error
		if s.tlsEnabled {
			err = s.httpServer.ServeTLS(s.listener, s.certFile, s.keyFile)
		} else {
			err = s.httpServer.Serve(s.listener)
		}
		if err != nil && err != http.ErrServerClosed {
			s.logger.Error("server error", zap.Error(err))
		}
	}()

	s.running = true
	s.logger.Info("webhook server started",
		zap.String("addr", addr),
		zap.Bool("tls", s.tlsEnabled))

	return nil
}

// Stop stops the webhook server
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown server: %w", err)
	}

	s.running = false
	s.logger.Info("webhook server stopped")

	return nil
}

// IsRunning returns true if the server is running
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// registerRoutes registers all HTTP routes
func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Health endpoints
	mux.HandleFunc("/healthz", s.handlers.HealthzHandler)
	mux.HandleFunc("/readyz", s.handlers.ReadyzHandler)
	mux.HandleFunc("/status", s.wrapWithAuth(s.handlers.StatusHandler))

	// Webhook endpoints
	mux.HandleFunc("/hooks/", s.wrapWithAuth(s.handlers.WebhookHandler))

	// Workflow endpoints
	mux.HandleFunc("/workflow/execute", s.wrapWithAuth(s.handlers.ExecuteWorkflowHandler))
	mux.HandleFunc("/workflow/status", s.wrapWithAuth(s.handlers.WorkflowStatusHandler))
	mux.HandleFunc("/workflow/cancel", s.wrapWithAuth(s.handlers.CancelWorkflowHandler))

	// Agent management endpoints
	mux.HandleFunc("/agent/config", s.wrapWithAuth(s.handlers.ConfigHandler))
	mux.HandleFunc("/agent/upgrade", s.wrapWithAuth(s.handlers.UpgradeHandler))
}

// wrapWithAuth wraps a handler with authentication
func (s *Server) wrapWithAuth(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.auth != nil {
			if !s.auth.Authenticate(r) {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}
		handler(w, r)
	}
}

// Handler returns the HTTP handler for use with Piko
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	return mux
}

// GetPort returns the server port
func (s *Server) GetPort() int {
	return s.port
}

// GetListenAddr returns the listen address
func (s *Server) GetListenAddr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return fmt.Sprintf("%s:%d", s.listenAddr, s.port)
}
