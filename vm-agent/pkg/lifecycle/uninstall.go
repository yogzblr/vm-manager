// Package lifecycle handles agent lifecycle management.
package lifecycle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"go.uber.org/zap"
)

// Uninstaller handles agent uninstallation
type Uninstaller struct {
	logger     *zap.Logger
	dataDir    string
	configPath string
}

// NewUninstaller creates a new uninstaller
func NewUninstaller(dataDir, configPath string, logger *zap.Logger) *Uninstaller {
	return &Uninstaller{
		logger:     logger,
		dataDir:    dataDir,
		configPath: configPath,
	}
}

// UninstallOptions contains uninstallation options
type UninstallOptions struct {
	KeepData     bool   // Keep data directory
	KeepConfig   bool   // Keep configuration file
	KeepLogs     bool   // Keep log files
	Deregister   bool   // Deregister from control plane
	ControlPlane string // Control plane URL for deregistration
	Token        string // Token for deregistration
}

// UninstallResult contains uninstallation results
type UninstallResult struct {
	Success      bool          `json:"success"`
	StoppedService bool        `json:"stopped_service"`
	RemovedService bool        `json:"removed_service"`
	RemovedData    bool        `json:"removed_data"`
	RemovedConfig  bool        `json:"removed_config"`
	Deregistered   bool        `json:"deregistered"`
	Errors       []string      `json:"errors,omitempty"`
	Duration     time.Duration `json:"duration"`
}

// Uninstall performs agent uninstallation
func (u *Uninstaller) Uninstall(ctx context.Context, opts *UninstallOptions) (*UninstallResult, error) {
	startTime := time.Now()
	result := &UninstallResult{
		Success: true,
		Errors:  make([]string, 0),
	}

	u.logger.Info("starting agent uninstallation")

	// Step 1: Deregister from control plane (if requested)
	if opts.Deregister && opts.ControlPlane != "" {
		if err := u.deregister(ctx, opts); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("deregistration failed: %v", err))
		} else {
			result.Deregistered = true
		}
	}

	// Step 2: Stop service
	if err := u.stopService(); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to stop service: %v", err))
	} else {
		result.StoppedService = true
	}

	// Step 3: Remove service
	if err := u.removeService(); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to remove service: %v", err))
	} else {
		result.RemovedService = true
	}

	// Step 4: Remove data directory (if not keeping)
	if !opts.KeepData {
		if err := u.removeDataDir(opts); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to remove data directory: %v", err))
		} else {
			result.RemovedData = true
		}
	}

	// Step 5: Remove configuration (if not keeping)
	if !opts.KeepConfig {
		if err := os.Remove(u.configPath); err != nil && !os.IsNotExist(err) {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to remove config: %v", err))
		} else {
			result.RemovedConfig = true
		}
	}

	result.Duration = time.Since(startTime)
	result.Success = len(result.Errors) == 0

	u.logger.Info("agent uninstallation completed",
		zap.Bool("success", result.Success),
		zap.Duration("duration", result.Duration))

	return result, nil
}

// deregister deregisters the agent from the control plane
func (u *Uninstaller) deregister(ctx context.Context, opts *UninstallOptions) error {
	// Would make HTTP DELETE request to control plane
	// For now, just log
	u.logger.Info("deregistering from control plane",
		zap.String("control_plane", opts.ControlPlane))
	return nil
}

// stopService stops the agent service
func (u *Uninstaller) stopService() error {
	switch runtime.GOOS {
	case "linux":
		return stopLinuxService()
	case "windows":
		return stopWindowsService()
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// removeService removes the agent service
func (u *Uninstaller) removeService() error {
	switch runtime.GOOS {
	case "linux":
		return removeLinuxService()
	case "windows":
		return removeWindowsService()
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// removeDataDir removes the data directory
func (u *Uninstaller) removeDataDir(opts *UninstallOptions) error {
	if opts.KeepLogs {
		// Remove everything except logs
		entries, err := os.ReadDir(u.dataDir)
		if err != nil {
			return err
		}

		for _, entry := range entries {
			if entry.Name() == "logs" {
				continue
			}
			path := filepath.Join(u.dataDir, entry.Name())
			if err := os.RemoveAll(path); err != nil {
				return err
			}
		}
		return nil
	}

	return os.RemoveAll(u.dataDir)
}

// Purge completely removes all agent files and registry entries
func (u *Uninstaller) Purge(ctx context.Context) error {
	opts := &UninstallOptions{
		KeepData:   false,
		KeepConfig: false,
		KeepLogs:   false,
	}

	result, err := u.Uninstall(ctx, opts)
	if err != nil {
		return err
	}

	if !result.Success {
		return fmt.Errorf("purge failed with errors: %v", result.Errors)
	}

	// Remove binary
	binaryPath, _ := os.Executable()
	if binaryPath != "" {
		if err := os.Remove(binaryPath); err != nil && !os.IsNotExist(err) {
			u.logger.Warn("failed to remove binary",
				zap.String("path", binaryPath),
				zap.Error(err))
		}
	}

	return nil
}

// PreUninstallCheck performs checks before uninstallation
func (u *Uninstaller) PreUninstallCheck() ([]string, error) {
	warnings := make([]string, 0)

	// Check for running workflows
	// (Would need access to workflow executor)

	// Check for pending upgrades
	// (Would need access to upgrader)

	// Check for unsaved data
	workDir := filepath.Join(u.dataDir, "work")
	entries, err := os.ReadDir(workDir)
	if err == nil && len(entries) > 0 {
		warnings = append(warnings, fmt.Sprintf("Work directory contains %d items that will be removed", len(entries)))
	}

	return warnings, nil
}
