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

	"github.com/yourorg/vm-agent/pkg/config"
)

// Repairer handles agent self-repair
type Repairer struct {
	logger     *zap.Logger
	dataDir    string
	configPath string
}

// NewRepairer creates a new repairer
func NewRepairer(dataDir, configPath string, logger *zap.Logger) *Repairer {
	return &Repairer{
		logger:     logger,
		dataDir:    dataDir,
		configPath: configPath,
	}
}

// RepairResult contains repair operation results
type RepairResult struct {
	Success       bool                `json:"success"`
	Issues        []RepairIssue       `json:"issues"`
	Repaired      []RepairIssue       `json:"repaired"`
	FailedRepairs []RepairIssue       `json:"failed_repairs,omitempty"`
	Duration      time.Duration       `json:"duration"`
}

// RepairIssue represents a detected issue
type RepairIssue struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
	Repaired    bool   `json:"repaired,omitempty"`
	Error       string `json:"error,omitempty"`
}

// Diagnose performs a diagnostic check without repairs
func (r *Repairer) Diagnose(ctx context.Context) (*RepairResult, error) {
	result := &RepairResult{
		Success: true,
		Issues:  make([]RepairIssue, 0),
	}

	// Check directories
	r.checkDirectories(result)

	// Check configuration
	r.checkConfiguration(result)

	// Check service
	r.checkService(result)

	// Check permissions
	r.checkPermissions(result)

	// Check connectivity
	r.checkConnectivity(ctx, result)

	// Determine overall success
	for _, issue := range result.Issues {
		if issue.Severity == "critical" {
			result.Success = false
			break
		}
	}

	return result, nil
}

// Repair performs diagnosis and repairs
func (r *Repairer) Repair(ctx context.Context) (*RepairResult, error) {
	startTime := time.Now()

	result := &RepairResult{
		Success:       true,
		Issues:        make([]RepairIssue, 0),
		Repaired:      make([]RepairIssue, 0),
		FailedRepairs: make([]RepairIssue, 0),
	}

	// Check and repair directories
	r.repairDirectories(result)

	// Check and repair configuration
	r.repairConfiguration(result)

	// Check and repair service
	r.repairService(result)

	// Check and repair permissions
	r.repairPermissions(result)

	result.Duration = time.Since(startTime)

	// Determine overall success
	for _, issue := range result.FailedRepairs {
		if issue.Severity == "critical" {
			result.Success = false
		}
	}

	return result, nil
}

// checkDirectories checks required directories
func (r *Repairer) checkDirectories(result *RepairResult) {
	dirs := []string{
		r.dataDir,
		filepath.Join(r.dataDir, "work"),
		filepath.Join(r.dataDir, "logs"),
		filepath.Join(r.dataDir, "backup"),
	}

	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			result.Issues = append(result.Issues, RepairIssue{
				Type:        "missing_directory",
				Description: fmt.Sprintf("Directory not found: %s", dir),
				Severity:    "warning",
			})
		}
	}
}

// repairDirectories repairs missing directories
func (r *Repairer) repairDirectories(result *RepairResult) {
	dirs := []string{
		r.dataDir,
		filepath.Join(r.dataDir, "work"),
		filepath.Join(r.dataDir, "logs"),
		filepath.Join(r.dataDir, "backup"),
	}

	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			issue := RepairIssue{
				Type:        "missing_directory",
				Description: fmt.Sprintf("Directory not found: %s", dir),
				Severity:    "warning",
			}

			if err := os.MkdirAll(dir, 0755); err != nil {
				issue.Error = err.Error()
				result.FailedRepairs = append(result.FailedRepairs, issue)
			} else {
				issue.Repaired = true
				result.Repaired = append(result.Repaired, issue)
			}

			result.Issues = append(result.Issues, issue)
		}
	}
}

// checkConfiguration checks configuration validity
func (r *Repairer) checkConfiguration(result *RepairResult) {
	loader := config.NewLoader()
	loader.SetConfigPath(r.configPath)

	cfg, err := loader.Load()
	if err != nil {
		result.Issues = append(result.Issues, RepairIssue{
			Type:        "invalid_config",
			Description: fmt.Sprintf("Failed to load configuration: %v", err),
			Severity:    "critical",
		})
		return
	}

	validator := config.NewValidator()
	if err := validator.Validate(cfg); err != nil {
		result.Issues = append(result.Issues, RepairIssue{
			Type:        "invalid_config",
			Description: fmt.Sprintf("Configuration validation failed: %v", err),
			Severity:    "critical",
		})
	}
}

// repairConfiguration attempts to repair configuration issues
func (r *Repairer) repairConfiguration(result *RepairResult) {
	loader := config.NewLoader()
	loader.SetConfigPath(r.configPath)

	cfg, err := loader.Load()
	if err != nil {
		// Try to restore from backup
		backupPath := r.configPath + ".bak"
		if _, err := os.Stat(backupPath); err == nil {
			issue := RepairIssue{
				Type:        "invalid_config",
				Description: "Configuration corrupted, restoring from backup",
				Severity:    "critical",
			}

			if err := copyFile(backupPath, r.configPath); err != nil {
				issue.Error = err.Error()
				result.FailedRepairs = append(result.FailedRepairs, issue)
			} else {
				issue.Repaired = true
				result.Repaired = append(result.Repaired, issue)
			}

			result.Issues = append(result.Issues, issue)
		}
		return
	}

	// Validate and fix specific fields
	validator := config.NewValidator()
	if err := validator.Validate(cfg); err != nil {
		// Configuration has validation errors - log but can't auto-fix
		result.Issues = append(result.Issues, RepairIssue{
			Type:        "config_validation_error",
			Description: fmt.Sprintf("Configuration validation failed: %v", err),
			Severity:    "warning",
		})
	}

	// Create backup
	backupPath := r.configPath + ".bak"
	copyFile(r.configPath, backupPath)
}

// checkService checks service status
func (r *Repairer) checkService(result *RepairResult) {
	var status string
	switch runtime.GOOS {
	case "linux":
		status = getLinuxServiceStatus()
	case "windows":
		status = getWindowsServiceStatus()
	default:
		return
	}

	if status != "running" && status != "active" {
		result.Issues = append(result.Issues, RepairIssue{
			Type:        "service_not_running",
			Description: fmt.Sprintf("Service status: %s", status),
			Severity:    "warning",
		})
	}
}

// repairService attempts to repair service issues
func (r *Repairer) repairService(result *RepairResult) {
	var status string
	switch runtime.GOOS {
	case "linux":
		status = getLinuxServiceStatus()
	case "windows":
		status = getWindowsServiceStatus()
	default:
		return
	}

	if status != "running" && status != "active" {
		issue := RepairIssue{
			Type:        "service_not_running",
			Description: "Attempting to restart service",
			Severity:    "warning",
		}

		var err error
		switch runtime.GOOS {
		case "linux":
			err = startLinuxService()
		case "windows":
			err = startWindowsService()
		}

		if err != nil {
			issue.Error = err.Error()
			result.FailedRepairs = append(result.FailedRepairs, issue)
		} else {
			issue.Repaired = true
			result.Repaired = append(result.Repaired, issue)
		}

		result.Issues = append(result.Issues, issue)
	}
}

// checkPermissions checks file permissions
func (r *Repairer) checkPermissions(result *RepairResult) {
	// Check config file permissions
	if info, err := os.Stat(r.configPath); err == nil {
		mode := info.Mode()
		if mode.Perm()&0077 != 0 {
			result.Issues = append(result.Issues, RepairIssue{
				Type:        "insecure_permissions",
				Description: fmt.Sprintf("Config file has insecure permissions: %v", mode.Perm()),
				Severity:    "warning",
			})
		}
	}
}

// repairPermissions fixes file permissions
func (r *Repairer) repairPermissions(result *RepairResult) {
	if info, err := os.Stat(r.configPath); err == nil {
		mode := info.Mode()
		if mode.Perm()&0077 != 0 {
			issue := RepairIssue{
				Type:        "insecure_permissions",
				Description: "Fixing config file permissions",
				Severity:    "warning",
			}

			if err := os.Chmod(r.configPath, 0600); err != nil {
				issue.Error = err.Error()
				result.FailedRepairs = append(result.FailedRepairs, issue)
			} else {
				issue.Repaired = true
				result.Repaired = append(result.Repaired, issue)
			}

			result.Issues = append(result.Issues, issue)
		}
	}
}

// checkConnectivity checks network connectivity
func (r *Repairer) checkConnectivity(ctx context.Context, result *RepairResult) {
	// Load config to get URLs
	loader := config.NewLoader()
	loader.SetConfigPath(r.configPath)

	cfg, err := loader.Load()
	if err != nil {
		return
	}

	// Check control plane connectivity
	if cfg.Agent.ControlPlaneURL != "" {
		// Simple connectivity check would go here
		// For now, just note that we would check
	}
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = destFile.ReadFrom(sourceFile)
	return err
}
