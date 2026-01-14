// Package lifecycle handles agent lifecycle management.
package lifecycle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"go.uber.org/zap"

	"github.com/yourorg/vm-agent/pkg/config"
)

// Installer handles agent installation
type Installer struct {
	logger         *zap.Logger
	dataDir        string
	configPath     string
	controlPlaneURL string
	httpClient     *http.Client
}

// InstallerConfig contains installer configuration
type InstallerConfig struct {
	DataDir         string
	ConfigPath      string
	ControlPlaneURL string
}

// NewInstaller creates a new installer
func NewInstaller(cfg *InstallerConfig, logger *zap.Logger) *Installer {
	return &Installer{
		logger:          logger,
		dataDir:         cfg.DataDir,
		configPath:      cfg.ConfigPath,
		controlPlaneURL: cfg.ControlPlaneURL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Install performs agent installation
func (i *Installer) Install(ctx context.Context, opts *InstallOptions) error {
	i.logger.Info("starting agent installation",
		zap.String("tenant_id", opts.TenantID))

	// Step 1: Create directories
	if err := i.createDirectories(); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Step 2: Register with control plane
	token, agentID, err := i.registerAgent(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to register agent: %w", err)
	}

	// Step 3: Generate configuration
	cfg := i.generateConfig(opts, token, agentID)

	// Step 4: Save configuration
	loader := config.NewLoader()
	if err := loader.SaveConfig(cfg, i.configPath); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	// Step 5: Install as service
	if err := i.installService(opts); err != nil {
		return fmt.Errorf("failed to install service: %w", err)
	}

	i.logger.Info("agent installation completed",
		zap.String("agent_id", agentID))

	return nil
}

// InstallOptions contains installation options
type InstallOptions struct {
	TenantID        string
	InstallationKey string
	PikoServerURL   string
	ControlPlaneURL string
	AgentID         string // Optional, generated if empty
	Tags            map[string]string
}

// createDirectories creates necessary directories
func (i *Installer) createDirectories() error {
	dirs := []string{
		i.dataDir,
		filepath.Join(i.dataDir, "work"),
		filepath.Join(i.dataDir, "logs"),
		filepath.Join(i.dataDir, "backup"),
		filepath.Dir(i.configPath),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// registerAgent registers the agent with the control plane
func (i *Installer) registerAgent(ctx context.Context, opts *InstallOptions) (token string, agentID string, err error) {
	if opts.ControlPlaneURL == "" {
		opts.ControlPlaneURL = i.controlPlaneURL
	}

	if opts.ControlPlaneURL == "" {
		return "", "", fmt.Errorf("control plane URL not configured")
	}

	hostname, _ := os.Hostname()
	if opts.AgentID == "" {
		opts.AgentID = hostname
	}

	reqBody := map[string]interface{}{
		"installation_key": opts.InstallationKey,
		"agent_id":         opts.AgentID,
		"hostname":         hostname,
		"os":               runtime.GOOS,
		"arch":             runtime.GOARCH,
		"tags":             opts.Tags,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal registration request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/agents/register", opts.ControlPlaneURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := i.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("registration request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("registration failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Token   string `json:"token"`
		AgentID string `json:"agent_id"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("failed to decode registration response: %w", err)
	}

	return result.Token, result.AgentID, nil
}

// generateConfig generates the agent configuration
func (i *Installer) generateConfig(opts *InstallOptions, token, agentID string) *config.Config {
	cfg := &config.Config{
		Agent: config.AgentConfig{
			ID:              agentID,
			TenantID:        opts.TenantID,
			ControlPlaneURL: opts.ControlPlaneURL,
			Token:           token,
			DataDir:         i.dataDir,
		},
		Piko: config.PikoConfig{
			ServerURL: opts.PikoServerURL,
			Endpoint:  fmt.Sprintf("tenant-%s/%s", opts.TenantID, agentID),
			Reconnect: config.ReconnectConfig{
				InitialDelay: time.Second,
				MaxDelay:     60 * time.Second,
				Multiplier:   2.0,
			},
		},
		Webhook: config.WebhookConfig{
			ListenAddr: "0.0.0.0",
			Port:       9999,
			TLSEnabled: false,
		},
		Probe: config.ProbeConfig{
			WorkDir:        filepath.Join(i.dataDir, "work"),
			DefaultTimeout: 5 * time.Minute,
			MaxConcurrent:  5,
		},
		Health: config.HealthConfig{
			CheckInterval:  30 * time.Second,
			ReportInterval: 5 * time.Minute,
			ReportURL:      fmt.Sprintf("%s/api/v1/agents/health", opts.ControlPlaneURL),
		},
		Logging: config.LoggingConfig{
			Level:  "info",
			Format: "json",
			File:   filepath.Join(i.dataDir, "logs", "agent.log"),
		},
	}

	return cfg
}

// installService installs the agent as a system service
func (i *Installer) installService(opts *InstallOptions) error {
	switch runtime.GOOS {
	case "linux":
		return installLinuxService(i.configPath)
	case "windows":
		return installWindowsService(i.configPath)
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// VerifyChecksum verifies a file's SHA256 checksum
func VerifyChecksum(filePath, expectedChecksum string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("failed to hash file: %w", err)
	}

	actualChecksum := hex.EncodeToString(hash.Sum(nil))
	if actualChecksum != expectedChecksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum)
	}

	return nil
}

// GetInstallInfo returns information about the current installation
type InstallInfo struct {
	AgentID    string            `json:"agent_id"`
	TenantID   string            `json:"tenant_id"`
	Version    string            `json:"version"`
	DataDir    string            `json:"data_dir"`
	ConfigPath string            `json:"config_path"`
	ServiceStatus string         `json:"service_status"`
	InstalledAt time.Time        `json:"installed_at"`
	Tags       map[string]string `json:"tags,omitempty"`
}

// GetInstallInfo returns installation information
func (i *Installer) GetInstallInfo() (*InstallInfo, error) {
	loader := config.NewLoader()
	loader.SetConfigPath(i.configPath)

	cfg, err := loader.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Get service status
	var serviceStatus string
	switch runtime.GOOS {
	case "linux":
		serviceStatus = getLinuxServiceStatus()
	case "windows":
		serviceStatus = getWindowsServiceStatus()
	default:
		serviceStatus = "unknown"
	}

	info := &InstallInfo{
		AgentID:       cfg.Agent.ID,
		TenantID:      cfg.Agent.TenantID,
		DataDir:       i.dataDir,
		ConfigPath:    i.configPath,
		ServiceStatus: serviceStatus,
	}

	return info, nil
}
