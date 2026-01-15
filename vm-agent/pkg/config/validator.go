// Package config handles configuration loading and management for the vm-agent.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationErrors represents multiple validation errors
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	var msgs []string
	for _, err := range e {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "; ")
}

// Validator validates configuration
type Validator struct {
	errors ValidationErrors
}

// NewValidator creates a new configuration validator
func NewValidator() *Validator {
	return &Validator{}
}

// Validate validates the configuration and returns any errors
func (v *Validator) Validate(cfg *Config) error {
	v.errors = nil

	v.validateAgent(cfg.Agent)
	v.validatePiko(cfg.Piko)
	v.validateWebhook(cfg.Webhook)
	v.validateProbe(cfg.Probe)
	v.validateHealth(cfg.Health)

	if len(v.errors) > 0 {
		return v.errors
	}
	return nil
}

// validateAgent validates agent configuration
func (v *Validator) validateAgent(cfg AgentConfig) {
	if cfg.ID == "" {
		v.addError("agent.id", "agent ID is required")
	}

	if cfg.TenantID == "" {
		v.addError("agent.tenant_id", "tenant ID is required")
	}

	if cfg.ControlPlaneURL != "" {
		if _, err := url.Parse(cfg.ControlPlaneURL); err != nil {
			v.addError("agent.control_plane_url", "invalid URL format")
		}
	}

	if cfg.DataDir != "" {
		if err := v.validateDirectory(cfg.DataDir, true); err != nil {
			v.addError("agent.data_dir", err.Error())
		}
	}
}

// validatePiko validates Piko configuration
func (v *Validator) validatePiko(cfg PikoConfig) {
	if cfg.ServerURL == "" {
		v.addError("piko.server_url", "Piko server URL is required")
	} else {
		if _, err := url.Parse(cfg.ServerURL); err != nil {
			v.addError("piko.server_url", "invalid URL format")
		}
	}

	if cfg.Endpoint == "" {
		v.addError("piko.endpoint", "Piko endpoint is required")
	}

	if cfg.Reconnect.InitialDelay <= 0 {
		v.addError("piko.reconnect.initial_delay", "must be positive")
	}

	if cfg.Reconnect.MaxDelay <= 0 {
		v.addError("piko.reconnect.max_delay", "must be positive")
	}

	if cfg.Reconnect.MaxDelay < cfg.Reconnect.InitialDelay {
		v.addError("piko.reconnect.max_delay", "must be greater than or equal to initial_delay")
	}

	if cfg.Reconnect.Multiplier < 1 {
		v.addError("piko.reconnect.multiplier", "must be at least 1")
	}
}

// validateWebhook validates webhook configuration
func (v *Validator) validateWebhook(cfg WebhookConfig) {
	if cfg.Port < 1 || cfg.Port > 65535 {
		v.addError("webhook.port", "must be between 1 and 65535")
	}

	if cfg.TLSEnabled {
		if cfg.CertFile == "" {
			v.addError("webhook.cert_file", "required when TLS is enabled")
		} else if err := v.validateFileExists(cfg.CertFile); err != nil {
			v.addError("webhook.cert_file", err.Error())
		}

		if cfg.KeyFile == "" {
			v.addError("webhook.key_file", "required when TLS is enabled")
		} else if err := v.validateFileExists(cfg.KeyFile); err != nil {
			v.addError("webhook.key_file", err.Error())
		}
	}
}

// validateProbe validates probe configuration
func (v *Validator) validateProbe(cfg ProbeConfig) {
	if cfg.WorkDir == "" {
		v.addError("probe.work_dir", "work directory is required")
	}

	if cfg.DefaultTimeout <= 0 {
		v.addError("probe.default_timeout", "must be positive")
	}

	if cfg.MaxConcurrent < 1 {
		v.addError("probe.max_concurrent", "must be at least 1")
	}
}

// validateHealth validates health configuration
func (v *Validator) validateHealth(cfg HealthConfig) {
	if cfg.CheckInterval <= 0 {
		v.addError("health.check_interval", "must be positive")
	}

	if cfg.ReportInterval <= 0 {
		v.addError("health.report_interval", "must be positive")
	}

	if cfg.ReportURL != "" {
		if _, err := url.Parse(cfg.ReportURL); err != nil {
			v.addError("health.report_url", "invalid URL format")
		}
	}
}

// addError adds a validation error
func (v *Validator) addError(field, message string) {
	v.errors = append(v.errors, ValidationError{
		Field:   field,
		Message: message,
	})
}

// validateDirectory validates a directory path
func (v *Validator) validateDirectory(path string, createIfMissing bool) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		if createIfMissing {
			return nil // Will be created later
		}
		return fmt.Errorf("directory does not exist")
	}
	if err != nil {
		return fmt.Errorf("failed to check directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path exists but is not a directory")
	}
	return nil
}

// validateFileExists validates that a file exists
func (v *Validator) validateFileExists(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("file does not exist")
	}
	if err != nil {
		return fmt.Errorf("failed to check file: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("path is a directory, not a file")
	}
	return nil
}

// ValidateForInstall validates configuration for installation
func ValidateForInstall(cfg *Config) error {
	v := NewValidator()

	// Minimal validation for installation
	if cfg.Agent.TenantID == "" {
		v.addError("agent.tenant_id", "tenant ID is required for installation")
	}

	if cfg.Piko.ServerURL == "" {
		v.addError("piko.server_url", "Piko server URL is required for installation")
	}

	if len(v.errors) > 0 {
		return v.errors
	}
	return nil
}

// ValidateForRegistration validates configuration for agent registration
func ValidateForRegistration(cfg *Config) error {
	v := NewValidator()

	if cfg.Agent.TenantID == "" {
		v.addError("agent.tenant_id", "tenant ID is required for registration")
	}

	if cfg.Agent.ControlPlaneURL == "" {
		v.addError("agent.control_plane_url", "control plane URL is required for registration")
	}

	if len(v.errors) > 0 {
		return v.errors
	}
	return nil
}
