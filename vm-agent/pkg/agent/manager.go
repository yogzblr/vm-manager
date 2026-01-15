// Package agent provides the main agent manager.
package agent

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/yourorg/vm-agent/internal/version"
	"github.com/yourorg/vm-agent/pkg/config"
	"github.com/yourorg/vm-agent/pkg/health"
	"github.com/yourorg/vm-agent/pkg/lifecycle"
	"github.com/yourorg/vm-agent/pkg/piko"
	"github.com/yourorg/vm-agent/pkg/probe"
	"github.com/yourorg/vm-agent/pkg/webhook"
)

// Manager is the main agent manager
type Manager struct {
	mu            sync.RWMutex
	cfg           *config.Config
	logger        *zap.Logger
	pikoClient    *piko.Client
	webhookServer *webhook.Server
	probeExecutor *probe.Executor
	healthMonitor *health.Monitor
	healthReporter *health.Reporter
	upgrader      *lifecycle.Upgrader
	configurator  *lifecycle.Configurator
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	running       bool
}

// NewManager creates a new agent manager
func NewManager(cfg *config.Config) (*Manager, error) {
	// Initialize logger
	logger, err := initLogger(cfg.Logging)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	return &Manager{
		cfg:    cfg,
		logger: logger,
	}, nil
}

// initLogger initializes the logger
func initLogger(cfg config.LoggingConfig) (*zap.Logger, error) {
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		level = zapcore.InfoLevel
	}

	zapConfig := zap.Config{
		Level:       zap.NewAtomicLevelAt(level),
		Development: false,
		Sampling: &zap.SamplingConfig{
			Initial:    100,
			Thereafter: 100,
		},
		Encoding:         cfg.Format,
		EncoderConfig:    zap.NewProductionEncoderConfig(),
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	if cfg.File != "" {
		zapConfig.OutputPaths = append(zapConfig.OutputPaths, cfg.File)
	}

	return zapConfig.Build()
}

// Run starts the agent
func (m *Manager) Run() error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return fmt.Errorf("agent already running")
	}
	m.running = true
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.mu.Unlock()

	m.logger.Info("starting vm-agent",
		zap.String("version", version.Version),
		zap.String("agent_id", m.cfg.Agent.ID),
		zap.String("tenant_id", m.cfg.Agent.TenantID))

	// Initialize components
	if err := m.initComponents(); err != nil {
		return fmt.Errorf("failed to initialize components: %w", err)
	}

	// Start components
	if err := m.startComponents(); err != nil {
		return fmt.Errorf("failed to start components: %w", err)
	}

	// Wait for shutdown signal
	m.waitForShutdown()

	return nil
}

// initComponents initializes all agent components
func (m *Manager) initComponents() error {
	var err error

	// Initialize probe executor
	m.probeExecutor, err = probe.NewExecutor(&probe.ExecutorConfig{
		WorkDir:       m.cfg.Probe.WorkDir,
		MaxConcurrent: m.cfg.Probe.MaxConcurrent,
	}, m.logger)
	if err != nil {
		return fmt.Errorf("failed to create probe executor: %w", err)
	}

	// Initialize health monitor
	m.healthMonitor = health.NewMonitor(
		m.cfg.Agent.ID,
		m.cfg.Agent.TenantID,
		version.Version,
		m.cfg.Health.CheckInterval,
		m.logger,
	)

	// Initialize upgrader
	m.upgrader = lifecycle.NewUpgrader(m.cfg.Agent.DataDir, m.logger)

	// Initialize configurator
	m.configurator = lifecycle.NewConfigurator("/etc/vm-agent/config.yaml", m.logger)

	// Initialize webhook handlers
	webhookHandlers := webhook.NewHandlers(
		m.logger,
		m.probeExecutor,
		m.healthMonitor,
		lifecycle.NewConfigProvider(m.configurator),
		m.upgrader,
	)

	// Initialize webhook authenticator
	webhookAuth := webhook.NewAuthenticator(&webhook.AuthConfig{
		JWTSecret: m.cfg.Agent.Token,
	})

	// Initialize webhook server
	m.webhookServer = webhook.NewServer(&webhook.ServerConfig{
		ListenAddr: m.cfg.Webhook.ListenAddr,
		Port:       m.cfg.Webhook.Port,
		TLSEnabled: m.cfg.Webhook.TLSEnabled,
		CertFile:   m.cfg.Webhook.CertFile,
		KeyFile:    m.cfg.Webhook.KeyFile,
	}, webhookHandlers, webhookAuth, m.logger)

	// Initialize Piko client
	m.pikoClient = piko.NewClient(&piko.ClientConfig{
		ServerURL:   m.cfg.Piko.ServerURL,
		Endpoint:    m.cfg.Piko.Endpoint,
		Token:       m.cfg.Agent.Token,
		TenantID:    m.cfg.Agent.TenantID,
		HTTPHandler: m.webhookServer.Handler(),
		Reconnect: &piko.ReconnectConfig{
			InitialDelay: m.cfg.Piko.Reconnect.InitialDelay,
			MaxDelay:     m.cfg.Piko.Reconnect.MaxDelay,
			Multiplier:   m.cfg.Piko.Reconnect.Multiplier,
		},
	}, m.logger)

	// Initialize health reporter
	m.healthReporter = health.NewReporter(
		m.healthMonitor,
		m.cfg.Health.ReportURL,
		m.cfg.Agent.Token,
		m.cfg.Health.ReportInterval,
		m.logger,
	)

	// Register health checkers
	m.healthMonitor.RegisterChecker(health.NewSelfChecker())
	m.healthMonitor.RegisterChecker(health.NewPikoChecker(
		m.pikoClient.IsConnected,
		m.pikoClient.LastError,
	))
	m.healthMonitor.RegisterChecker(health.NewWebhookChecker(
		m.webhookServer.IsRunning,
		m.cfg.Webhook.Port,
	))
	m.healthMonitor.RegisterChecker(health.NewProbeChecker(
		m.probeExecutor.ActiveJobs,
		m.cfg.Probe.MaxConcurrent,
		func() error { return nil },
	))
	m.healthMonitor.RegisterChecker(health.NewSystemChecker(
		100*1024*1024, // 100MB minimum disk space
		m.cfg.Agent.DataDir,
	))

	return nil
}

// startComponents starts all agent components
func (m *Manager) startComponents() error {
	// Start health monitor
	m.healthMonitor.Start(m.ctx)

	// Start health reporter
	m.healthReporter.Start(m.ctx)

	// Start Piko client
	if err := m.pikoClient.Start(m.ctx); err != nil {
		return fmt.Errorf("failed to start Piko client: %w", err)
	}

	// Start webhook server (for local access)
	if err := m.webhookServer.Start(m.ctx); err != nil {
		return fmt.Errorf("failed to start webhook server: %w", err)
	}

	m.logger.Info("all components started")

	return nil
}

// waitForShutdown waits for shutdown signal and performs graceful shutdown
func (m *Manager) waitForShutdown() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigCh
	m.logger.Info("received shutdown signal", zap.String("signal", sig.String()))

	m.Shutdown()
}

// Shutdown performs graceful shutdown
func (m *Manager) Shutdown() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	m.mu.Unlock()

	m.logger.Info("shutting down agent")

	// Cancel context
	m.cancel()

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop components in reverse order
	if m.webhookServer != nil {
		m.webhookServer.Stop(ctx)
	}

	if m.pikoClient != nil {
		m.pikoClient.Stop()
	}

	if m.healthReporter != nil {
		m.healthReporter.Stop()
	}

	if m.healthMonitor != nil {
		m.healthMonitor.Stop()
	}

	// Sync logger
	m.logger.Sync()

	m.logger.Info("agent shutdown complete")
}

// HealthCheck returns the current health status
func (m *Manager) HealthCheck() *health.Status {
	if m.healthMonitor == nil {
		return &health.Status{
			Overall: health.StatusUnknown,
		}
	}
	return m.healthMonitor.GetStatus()
}

// GetConfig returns the current configuration
func (m *Manager) GetConfig() *config.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

// UpdateConfig updates the agent configuration
func (m *Manager) UpdateConfig(newCfg *config.Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Validate new config
	validator := config.NewValidator()
	if err := validator.Validate(newCfg); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	m.cfg = newCfg
	return nil
}
