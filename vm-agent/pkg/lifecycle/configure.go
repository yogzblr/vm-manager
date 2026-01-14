// Package lifecycle handles agent lifecycle management.
package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"go.uber.org/zap"

	"github.com/yourorg/vm-agent/pkg/config"
)

// Configurator handles agent configuration
type Configurator struct {
	logger     *zap.Logger
	configPath string
	loader     *config.Loader
}

// NewConfigurator creates a new configurator
func NewConfigurator(configPath string, logger *zap.Logger) *Configurator {
	loader := config.NewLoader()
	loader.SetConfigPath(configPath)

	return &Configurator{
		logger:     logger,
		configPath: configPath,
		loader:     loader,
	}
}

// GetConfig returns the current configuration
func (c *Configurator) GetConfig() (*config.Config, error) {
	return c.loader.Load()
}

// UpdateConfig updates the configuration
func (c *Configurator) UpdateConfig(data []byte) error {
	// Parse the update
	var updates map[string]interface{}
	if err := json.Unmarshal(data, &updates); err != nil {
		return fmt.Errorf("failed to parse configuration update: %w", err)
	}

	// Load current config
	cfg, err := c.loader.Load()
	if err != nil {
		return fmt.Errorf("failed to load current config: %w", err)
	}

	// Apply updates
	if err := c.applyUpdates(cfg, updates); err != nil {
		return fmt.Errorf("failed to apply updates: %w", err)
	}

	// Validate
	validator := config.NewValidator()
	if err := validator.Validate(cfg); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	// Save
	if err := c.loader.SaveConfig(cfg, c.configPath); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	c.logger.Info("configuration updated")

	return nil
}

// applyUpdates applies configuration updates
func (c *Configurator) applyUpdates(cfg *config.Config, updates map[string]interface{}) error {
	// Convert to JSON and back for easier merging
	data, err := json.Marshal(updates)
	if err != nil {
		return err
	}

	// Decode into temporary config
	var tempCfg config.Config
	if err := json.Unmarshal(data, &tempCfg); err != nil {
		return err
	}

	// Merge with existing config
	resolver := config.NewPriorityResolver()
	merged := config.MergeConfigs(cfg, &tempCfg, config.SourceRemote, resolver)
	*cfg = *merged

	return nil
}

// ConfigureFromEnv applies configuration from environment variables
func (c *Configurator) ConfigureFromEnv() error {
	cfg, err := c.loader.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Check for environment overrides
	if v := os.Getenv("VM_AGENT_ID"); v != "" {
		cfg.Agent.ID = v
	}
	if v := os.Getenv("VM_AGENT_TENANT_ID"); v != "" {
		cfg.Agent.TenantID = v
	}
	if v := os.Getenv("VM_AGENT_TOKEN"); v != "" {
		cfg.Agent.Token = v
	}
	if v := os.Getenv("VM_AGENT_CONTROL_PLANE_URL"); v != "" {
		cfg.Agent.ControlPlaneURL = v
	}
	if v := os.Getenv("VM_AGENT_PIKO_SERVER_URL"); v != "" {
		cfg.Piko.ServerURL = v
	}

	return c.loader.SaveConfig(cfg, c.configPath)
}

// ValidateConfig validates the current configuration
func (c *Configurator) ValidateConfig() error {
	cfg, err := c.loader.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	validator := config.NewValidator()
	return validator.Validate(cfg)
}

// ResetConfig resets configuration to defaults
func (c *Configurator) ResetConfig(ctx context.Context) error {
	cfg, err := c.loader.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Preserve essential fields
	agentID := cfg.Agent.ID
	tenantID := cfg.Agent.TenantID
	token := cfg.Agent.Token
	controlPlaneURL := cfg.Agent.ControlPlaneURL
	dataDir := cfg.Agent.DataDir

	// Load defaults
	defaultLoader := config.NewLoader()
	defaultCfg, err := defaultLoader.Load()
	if err != nil {
		return fmt.Errorf("failed to load defaults: %w", err)
	}

	// Restore essential fields
	defaultCfg.Agent.ID = agentID
	defaultCfg.Agent.TenantID = tenantID
	defaultCfg.Agent.Token = token
	defaultCfg.Agent.ControlPlaneURL = controlPlaneURL
	defaultCfg.Agent.DataDir = dataDir

	// Save
	if err := c.loader.SaveConfig(defaultCfg, c.configPath); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	c.logger.Info("configuration reset to defaults")

	return nil
}

// ConfigProvider implements the webhook.ConfigProvider interface
type ConfigProvider struct {
	configurator *Configurator
}

// NewConfigProvider creates a new config provider
func NewConfigProvider(configurator *Configurator) *ConfigProvider {
	return &ConfigProvider{configurator: configurator}
}

// GetConfig returns the current configuration
func (p *ConfigProvider) GetConfig() interface{} {
	cfg, err := p.configurator.GetConfig()
	if err != nil {
		return map[string]string{"error": err.Error()}
	}
	return cfg
}

// UpdateConfig updates the configuration
func (p *ConfigProvider) UpdateConfig(data []byte) error {
	return p.configurator.UpdateConfig(data)
}
