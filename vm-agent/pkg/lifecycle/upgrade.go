// Package lifecycle handles agent lifecycle management.
package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/yourorg/vm-agent/internal/version"
)

// Upgrader handles agent self-upgrade
type Upgrader struct {
	mu          sync.Mutex
	logger      *zap.Logger
	dataDir     string
	currentBin  string
	httpClient  *http.Client
	inProgress  bool
	lastStatus  *UpgradeStatus
}

// UpgradeStatus represents upgrade status
type UpgradeStatus struct {
	InProgress  bool      `json:"in_progress"`
	Version     string    `json:"version,omitempty"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
	Status      string    `json:"status,omitempty"`
	Error       string    `json:"error,omitempty"`
}

// NewUpgrader creates a new upgrader
func NewUpgrader(dataDir string, logger *zap.Logger) *Upgrader {
	// Get current binary path
	currentBin, _ := os.Executable()

	return &Upgrader{
		logger:     logger,
		dataDir:    dataDir,
		currentBin: currentBin,
		httpClient: &http.Client{
			Timeout: 10 * time.Minute,
		},
		lastStatus: &UpgradeStatus{},
	}
}

// StartUpgrade initiates an upgrade
func (u *Upgrader) StartUpgrade(version, downloadURL, checksum string) error {
	u.mu.Lock()
	if u.inProgress {
		u.mu.Unlock()
		return fmt.Errorf("upgrade already in progress")
	}
	u.inProgress = true
	u.lastStatus = &UpgradeStatus{
		InProgress: true,
		Version:    version,
		StartedAt:  time.Now(),
		Status:     "starting",
	}
	u.mu.Unlock()

	go u.performUpgrade(version, downloadURL, checksum)

	return nil
}

// performUpgrade performs the upgrade process
func (u *Upgrader) performUpgrade(targetVersion, downloadURL, checksum string) {
	defer func() {
		u.mu.Lock()
		u.inProgress = false
		u.lastStatus.InProgress = false
		u.lastStatus.CompletedAt = time.Now()
		u.mu.Unlock()
	}()

	u.updateStatus("downloading", "")

	// Step 1: Download new binary
	tempPath := filepath.Join(u.dataDir, "upgrade", fmt.Sprintf("vm-agent-%s.tmp", targetVersion))
	if err := os.MkdirAll(filepath.Dir(tempPath), 0755); err != nil {
		u.updateStatus("failed", fmt.Sprintf("failed to create upgrade directory: %v", err))
		return
	}

	if err := u.downloadBinary(downloadURL, tempPath); err != nil {
		u.updateStatus("failed", fmt.Sprintf("download failed: %v", err))
		os.Remove(tempPath)
		return
	}

	u.updateStatus("verifying", "")

	// Step 2: Verify checksum
	if err := u.verifyChecksum(tempPath, checksum); err != nil {
		u.updateStatus("failed", fmt.Sprintf("checksum verification failed: %v", err))
		os.Remove(tempPath)
		return
	}

	u.updateStatus("backing_up", "")

	// Step 3: Backup current binary
	backupPath := filepath.Join(u.dataDir, "backup", fmt.Sprintf("vm-agent-%s.bak", version.Version))
	if err := u.backupBinary(backupPath); err != nil {
		u.updateStatus("failed", fmt.Sprintf("backup failed: %v", err))
		os.Remove(tempPath)
		return
	}

	u.updateStatus("replacing", "")

	// Step 4: Replace binary
	if err := u.replaceBinary(tempPath); err != nil {
		u.updateStatus("failed", fmt.Sprintf("binary replacement failed: %v", err))
		// Attempt rollback
		u.rollback(backupPath)
		os.Remove(tempPath)
		return
	}

	u.updateStatus("restarting", "")

	// Step 5: Restart service
	if err := u.restartService(); err != nil {
		u.updateStatus("failed", fmt.Sprintf("service restart failed: %v", err))
		// Attempt rollback
		u.rollback(backupPath)
		return
	}

	// Step 6: Verify new version (would be done by the new process)
	u.updateStatus("success", "")
	u.logger.Info("upgrade completed successfully",
		zap.String("from_version", version.Version),
		zap.String("to_version", targetVersion))
}

// downloadBinary downloads the new binary
func (u *Upgrader) downloadBinary(url, destPath string) error {
	resp, err := u.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// verifyChecksum verifies the SHA256 checksum of a file
func (u *Upgrader) verifyChecksum(filePath, expectedChecksum string) error {
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

// backupBinary creates a backup of the current binary
func (u *Upgrader) backupBinary(backupPath string) error {
	if err := os.MkdirAll(filepath.Dir(backupPath), 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	src, err := os.Open(u.currentBin)
	if err != nil {
		return fmt.Errorf("failed to open current binary: %w", err)
	}
	defer src.Close()

	dst, err := os.OpenFile(backupPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("failed to create backup file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("failed to copy binary: %w", err)
	}

	return nil
}

// replaceBinary replaces the current binary with the new one
func (u *Upgrader) replaceBinary(newBinPath string) error {
	// On Windows, we can't replace a running binary directly
	if runtime.GOOS == "windows" {
		return u.replaceWindowsBinary(newBinPath)
	}

	// On Unix, we can atomically replace using rename
	return os.Rename(newBinPath, u.currentBin)
}

// replaceWindowsBinary handles binary replacement on Windows
func (u *Upgrader) replaceWindowsBinary(newBinPath string) error {
	// Rename current binary to .old
	oldPath := u.currentBin + ".old"
	if err := os.Rename(u.currentBin, oldPath); err != nil {
		return fmt.Errorf("failed to rename current binary: %w", err)
	}

	// Move new binary into place
	if err := os.Rename(newBinPath, u.currentBin); err != nil {
		// Attempt to restore old binary
		os.Rename(oldPath, u.currentBin)
		return fmt.Errorf("failed to move new binary: %w", err)
	}

	// Schedule old binary for deletion
	// (Will be cleaned up on next boot or by cleanup routine)

	return nil
}

// restartService restarts the agent service
func (u *Upgrader) restartService() error {
	switch runtime.GOOS {
	case "linux":
		return u.restartLinuxService()
	case "windows":
		return u.restartWindowsService()
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// restartLinuxService restarts the systemd service
func (u *Upgrader) restartLinuxService() error {
	cmd := exec.Command("systemctl", "restart", "vm-agent")
	return cmd.Run()
}

// restartWindowsService restarts the Windows service
func (u *Upgrader) restartWindowsService() error {
	// Stop service
	stopCmd := exec.Command("sc", "stop", "vm-agent")
	if err := stopCmd.Run(); err != nil {
		// Service might not be running, continue
	}

	// Wait for service to stop
	time.Sleep(2 * time.Second)

	// Start service
	startCmd := exec.Command("sc", "start", "vm-agent")
	return startCmd.Run()
}

// rollback rolls back to the backup binary
func (u *Upgrader) rollback(backupPath string) error {
	u.logger.Warn("rolling back upgrade",
		zap.String("backup_path", backupPath))

	// Copy backup back
	src, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("failed to open backup: %w", err)
	}
	defer src.Close()

	dst, err := os.OpenFile(u.currentBin, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("failed to open current binary for rollback: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("failed to restore backup: %w", err)
	}

	return nil
}

// updateStatus updates the upgrade status
func (u *Upgrader) updateStatus(status, errorMsg string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.lastStatus.Status = status
	if errorMsg != "" {
		u.lastStatus.Error = errorMsg
	}
	u.logger.Info("upgrade status update",
		zap.String("status", status),
		zap.String("error", errorMsg))
}

// GetUpgradeStatus returns the current upgrade status
func (u *Upgrader) GetUpgradeStatus() *UpgradeStatus {
	u.mu.Lock()
	defer u.mu.Unlock()
	status := *u.lastStatus
	return &status
}

// CheckForUpdates checks for available updates
func (u *Upgrader) CheckForUpdates(ctx context.Context, checkURL string) (*UpdateInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checkURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Current-Version", version.Version)
	req.Header.Set("X-OS", runtime.GOOS)
	req.Header.Set("X-Arch", runtime.GOARCH)

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("update check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil // No update available
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update check returned status %d", resp.StatusCode)
	}

	var info UpdateInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode update info: %w", err)
	}

	return &info, nil
}

// UpdateInfo contains information about an available update
type UpdateInfo struct {
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
	Checksum    string `json:"checksum"`
	ReleaseDate string `json:"release_date"`
	Changelog   string `json:"changelog,omitempty"`
	Required    bool   `json:"required"`
}
