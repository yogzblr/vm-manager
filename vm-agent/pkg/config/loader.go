// Package config handles configuration loading and management for the vm-agent.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

// Config represents the complete agent configuration
type Config struct {
	Agent    AgentConfig    `mapstructure:"agent"`
	Piko     PikoConfig     `mapstructure:"piko"`
	Webhook  WebhookConfig  `mapstructure:"webhook"`
	Probe    ProbeConfig    `mapstructure:"probe"`
	Health   HealthConfig   `mapstructure:"health"`
	Logging  LoggingConfig  `mapstructure:"logging"`
}

// AgentConfig contains agent-specific configuration
type AgentConfig struct {
	ID              string `mapstructure:"id"`
	TenantID        string `mapstructure:"tenant_id"`
	ControlPlaneURL string `mapstructure:"control_plane_url"`
	Token           string `mapstructure:"token"`
	DataDir         string `mapstructure:"data_dir"`
}

// PikoConfig contains Piko client configuration
type PikoConfig struct {
	ServerURL string          `mapstructure:"server_url"`
	Endpoint  string          `mapstructure:"endpoint"`
	Reconnect ReconnectConfig `mapstructure:"reconnect"`
}

// ReconnectConfig contains reconnection settings
type ReconnectConfig struct {
	InitialDelay time.Duration `mapstructure:"initial_delay"`
	MaxDelay     time.Duration `mapstructure:"max_delay"`
	Multiplier   float64       `mapstructure:"multiplier"`
}

// WebhookConfig contains webhook server configuration
type WebhookConfig struct {
	ListenAddr string `mapstructure:"listen_addr"`
	Port       int    `mapstructure:"port"`
	TLSEnabled bool   `mapstructure:"tls_enabled"`
	CertFile   string `mapstructure:"cert_file"`
	KeyFile    string `mapstructure:"key_file"`
}

// ProbeConfig contains probe executor configuration
type ProbeConfig struct {
	WorkDir        string        `mapstructure:"work_dir"`
	DefaultTimeout time.Duration `mapstructure:"default_timeout"`
	MaxConcurrent  int           `mapstructure:"max_concurrent"`
}

// HealthConfig contains health monitoring configuration
type HealthConfig struct {
	CheckInterval  time.Duration `mapstructure:"check_interval"`
	ReportInterval time.Duration `mapstructure:"report_interval"`
	ReportURL      string        `mapstructure:"report_url"`
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	File   string `mapstructure:"file"`
}

// Loader handles configuration loading from multiple sources
type Loader struct {
	v          *viper.Viper
	configPath string
}

// NewLoader creates a new configuration loader
func NewLoader() *Loader {
	return &Loader{
		v: viper.New(),
	}
}

// SetConfigPath sets the configuration file path
func (l *Loader) SetConfigPath(path string) {
	l.configPath = path
}

// Load loads the configuration from all sources
func (l *Loader) Load() (*Config, error) {
	l.setDefaults()

	// Load from config file if specified
	if l.configPath != "" {
		l.v.SetConfigFile(l.configPath)
	} else {
		// Search for config in standard locations
		l.v.SetConfigName("config")
		l.v.SetConfigType("yaml")
		l.v.AddConfigPath("/etc/vm-agent")
		l.v.AddConfigPath("$HOME/.vm-agent")
		l.v.AddConfigPath(".")
	}

	// Read config file (ignore if not found)
	if err := l.v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Override with environment variables
	l.v.SetEnvPrefix("VM_AGENT")
	l.v.AutomaticEnv()

	var cfg Config
	if err := l.v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

// setDefaults sets default configuration values
func (l *Loader) setDefaults() {
	// Agent defaults
	l.v.SetDefault("agent.id", getHostname())
	l.v.SetDefault("agent.data_dir", "/var/lib/vm-agent")

	// Piko defaults
	l.v.SetDefault("piko.reconnect.initial_delay", "1s")
	l.v.SetDefault("piko.reconnect.max_delay", "60s")
	l.v.SetDefault("piko.reconnect.multiplier", 2.0)

	// Webhook defaults
	l.v.SetDefault("webhook.listen_addr", "0.0.0.0")
	l.v.SetDefault("webhook.port", 9999)
	l.v.SetDefault("webhook.tls_enabled", false)

	// Probe defaults
	l.v.SetDefault("probe.work_dir", "/var/lib/vm-agent/work")
	l.v.SetDefault("probe.default_timeout", "300s")
	l.v.SetDefault("probe.max_concurrent", 5)

	// Health defaults
	l.v.SetDefault("health.check_interval", "30s")
	l.v.SetDefault("health.report_interval", "300s")

	// Logging defaults
	l.v.SetDefault("logging.level", "info")
	l.v.SetDefault("logging.format", "json")
}

// getHostname returns the hostname or a default value
func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

// GetConfigPath returns the path to the configuration file being used
func (l *Loader) GetConfigPath() string {
	return l.v.ConfigFileUsed()
}

// SaveConfig saves the current configuration to a file
func (l *Loader) SaveConfig(cfg *Config, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	l.v.Set("agent", cfg.Agent)
	l.v.Set("piko", cfg.Piko)
	l.v.Set("webhook", cfg.Webhook)
	l.v.Set("probe", cfg.Probe)
	l.v.Set("health", cfg.Health)
	l.v.Set("logging", cfg.Logging)

	return l.v.WriteConfigAs(path)
}
