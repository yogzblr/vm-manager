// Package probe provides workflow execution functionality.
package probe

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// FileManager handles file operations for template deployment
type FileManager struct {
	// BackupDir is the directory where backups are stored
	BackupDir string
	// DryRun if true, only report changes without making them
	DryRun bool
}

// FileManagerConfig contains configuration for the file manager
type FileManagerConfig struct {
	BackupDir string
	DryRun    bool
}

// NewFileManager creates a new file manager
func NewFileManager(cfg *FileManagerConfig) *FileManager {
	backupDir := cfg.BackupDir
	if backupDir == "" {
		backupDir = "/var/lib/vm-agent/backups"
	}

	return &FileManager{
		BackupDir: backupDir,
		DryRun:    cfg.DryRun,
	}
}

// FileInfo contains information about a file
type FileInfo struct {
	Path      string
	Exists    bool
	Size      int64
	Mode      os.FileMode
	Owner     string
	Group     string
	OwnerUID  int
	GroupGID  int
	ModTime   time.Time
	Hash      string
	IsDir     bool
	IsSymlink bool
}

// DeployResult contains the result of a file deployment
type DeployResult struct {
	// Status indicates what happened: created, updated, unchanged, error
	Status string
	// Path is the destination path
	Path string
	// BackupPath is the path to the backup file (if created)
	BackupPath string
	// Diff contains the unified diff between old and new content
	Diff string
	// Changed indicates whether the file was modified
	Changed bool
	// OldHash is the hash of the original file
	OldHash string
	// NewHash is the hash of the new file
	NewHash string
	// Error contains any error message
	Error string
}

// DeployOptions contains options for file deployment
type DeployOptions struct {
	// Dest is the destination path
	Dest string
	// Content is the content to write
	Content string
	// Mode is the file permissions (e.g., "0644")
	Mode string
	// Owner is the file owner (username or UID)
	Owner string
	// Group is the file group (group name or GID)
	Group string
	// Backup enables creating a backup before overwriting
	Backup bool
	// DiffOnly only reports diff without writing
	DiffOnly bool
	// CreateDirs creates parent directories if they don't exist
	CreateDirs bool
	// DirMode is the permissions for created directories
	DirMode string
}

// Deploy deploys content to a file with backup and diff support
func (m *FileManager) Deploy(opts *DeployOptions) *DeployResult {
	result := &DeployResult{
		Path:   opts.Dest,
		Status: "error",
	}

	// Calculate hash of new content
	result.NewHash = hashContent(opts.Content)

	// Check if destination exists
	existingInfo, err := m.GetFileInfo(opts.Dest)
	if err != nil && !os.IsNotExist(err) {
		result.Error = fmt.Sprintf("failed to stat destination: %v", err)
		return result
	}

	if existingInfo != nil && existingInfo.Exists {
		result.OldHash = existingInfo.Hash

		// Compare hashes to determine if file changed
		if result.OldHash == result.NewHash {
			result.Status = "unchanged"
			result.Changed = false
			return result
		}

		// Read existing content for diff
		existingContent, err := os.ReadFile(opts.Dest)
		if err == nil {
			result.Diff = generateDiff(opts.Dest, string(existingContent), opts.Content)
		}

		// If DiffOnly, return here
		if opts.DiffOnly || m.DryRun {
			result.Status = "would_update"
			result.Changed = true
			return result
		}

		// Create backup if enabled
		if opts.Backup {
			backupPath, err := m.Backup(opts.Dest)
			if err != nil {
				result.Error = fmt.Sprintf("failed to create backup: %v", err)
				return result
			}
			result.BackupPath = backupPath
		}

		result.Status = "updated"
		result.Changed = true
	} else {
		// File doesn't exist
		if opts.DiffOnly || m.DryRun {
			result.Status = "would_create"
			result.Changed = true
			return result
		}

		result.Status = "created"
		result.Changed = true
	}

	// Create parent directories if needed
	if opts.CreateDirs {
		dirMode := os.FileMode(0755)
		if opts.DirMode != "" {
			if parsed, err := strconv.ParseUint(opts.DirMode, 8, 32); err == nil {
				dirMode = os.FileMode(parsed)
			}
		}
		if err := os.MkdirAll(filepath.Dir(opts.Dest), dirMode); err != nil {
			result.Error = fmt.Sprintf("failed to create directories: %v", err)
			result.Status = "error"
			return result
		}
	}

	// Write to temporary file first (atomic write)
	tempFile, err := os.CreateTemp(filepath.Dir(opts.Dest), ".tmp-")
	if err != nil {
		result.Error = fmt.Sprintf("failed to create temp file: %v", err)
		result.Status = "error"
		return result
	}
	tempPath := tempFile.Name()

	// Write content
	if _, err := tempFile.WriteString(opts.Content); err != nil {
		tempFile.Close()
		os.Remove(tempPath)
		result.Error = fmt.Sprintf("failed to write content: %v", err)
		result.Status = "error"
		return result
	}
	tempFile.Close()

	// Set file mode
	fileMode := os.FileMode(0644)
	if opts.Mode != "" {
		if parsed, err := strconv.ParseUint(opts.Mode, 8, 32); err == nil {
			fileMode = os.FileMode(parsed)
		}
	}
	if err := os.Chmod(tempPath, fileMode); err != nil {
		os.Remove(tempPath)
		result.Error = fmt.Sprintf("failed to set file mode: %v", err)
		result.Status = "error"
		return result
	}

	// Set owner and group
	if opts.Owner != "" || opts.Group != "" {
		uid, gid := -1, -1

		if opts.Owner != "" {
			if u, err := user.Lookup(opts.Owner); err == nil {
				uid, _ = strconv.Atoi(u.Uid)
			} else if parsed, err := strconv.Atoi(opts.Owner); err == nil {
				uid = parsed
			}
		}

		if opts.Group != "" {
			if g, err := user.LookupGroup(opts.Group); err == nil {
				gid, _ = strconv.Atoi(g.Gid)
			} else if parsed, err := strconv.Atoi(opts.Group); err == nil {
				gid = parsed
			}
		}

		if uid != -1 || gid != -1 {
			if err := os.Chown(tempPath, uid, gid); err != nil {
				// Non-fatal, log but continue
				result.Error = fmt.Sprintf("warning: failed to set ownership: %v", err)
			}
		}
	}

	// Atomic rename
	if err := os.Rename(tempPath, opts.Dest); err != nil {
		os.Remove(tempPath)
		result.Error = fmt.Sprintf("failed to rename temp file: %v", err)
		result.Status = "error"
		return result
	}

	return result
}

// GetFileInfo retrieves information about a file
func (m *FileManager) GetFileInfo(path string) (*FileInfo, error) {
	info := &FileInfo{
		Path:   path,
		Exists: false,
	}

	stat, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return info, nil
		}
		return nil, err
	}

	info.Exists = true
	info.Size = stat.Size()
	info.Mode = stat.Mode()
	info.ModTime = stat.ModTime()
	info.IsDir = stat.IsDir()
	info.IsSymlink = stat.Mode()&os.ModeSymlink != 0

	// Get owner/group info (Linux-specific)
	if sys, ok := stat.Sys().(*syscall.Stat_t); ok {
		info.OwnerUID = int(sys.Uid)
		info.GroupGID = int(sys.Gid)

		if u, err := user.LookupId(strconv.Itoa(info.OwnerUID)); err == nil {
			info.Owner = u.Username
		}
		if g, err := user.LookupGroupId(strconv.Itoa(info.GroupGID)); err == nil {
			info.Group = g.Name
		}
	}

	// Calculate hash for regular files
	if !info.IsDir && !info.IsSymlink {
		hash, err := hashFile(path)
		if err == nil {
			info.Hash = hash
		}
	}

	return info, nil
}

// Backup creates a backup of a file
func (m *FileManager) Backup(path string) (string, error) {
	// Create backup directory if it doesn't exist
	if err := os.MkdirAll(m.BackupDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Generate backup filename with timestamp
	timestamp := time.Now().Format("20060102-150405")
	baseName := filepath.Base(path)
	backupName := fmt.Sprintf("%s.%s.bak", baseName, timestamp)
	backupPath := filepath.Join(m.BackupDir, backupName)

	// Copy file to backup location
	if err := copyFile(path, backupPath); err != nil {
		return "", fmt.Errorf("failed to copy file to backup: %w", err)
	}

	return backupPath, nil
}

// Restore restores a file from backup
func (m *FileManager) Restore(backupPath, destPath string) error {
	return copyFile(backupPath, destPath)
}

// Compare compares two files and returns whether they are identical
func (m *FileManager) Compare(path1, path2 string) (bool, error) {
	hash1, err := hashFile(path1)
	if err != nil {
		return false, err
	}

	hash2, err := hashFile(path2)
	if err != nil {
		return false, err
	}

	return hash1 == hash2, nil
}

// hashFile calculates SHA256 hash of a file
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// hashContent calculates SHA256 hash of content
func hashContent(content string) string {
	h := sha256.New()
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcStat, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcStat.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return nil
}

// generateDiff generates a simple unified diff between two strings
func generateDiff(filename, old, new string) string {
	oldLines := strings.Split(old, "\n")
	newLines := strings.Split(new, "\n")

	var diff strings.Builder
	diff.WriteString(fmt.Sprintf("--- %s (original)\n", filename))
	diff.WriteString(fmt.Sprintf("+++ %s (new)\n", filename))

	// Simple line-by-line diff (not a proper unified diff algorithm)
	maxLines := len(oldLines)
	if len(newLines) > maxLines {
		maxLines = len(newLines)
	}

	inHunk := false
	hunkStart := 0

	for i := 0; i < maxLines; i++ {
		oldLine := ""
		newLine := ""
		if i < len(oldLines) {
			oldLine = oldLines[i]
		}
		if i < len(newLines) {
			newLine = newLines[i]
		}

		if oldLine != newLine {
			if !inHunk {
				hunkStart = i + 1
				inHunk = true
				diff.WriteString(fmt.Sprintf("@@ -%d +%d @@\n", hunkStart, hunkStart))
			}
			if i < len(oldLines) {
				diff.WriteString(fmt.Sprintf("-%s\n", oldLine))
			}
			if i < len(newLines) {
				diff.WriteString(fmt.Sprintf("+%s\n", newLine))
			}
		} else if inHunk {
			// Context line
			diff.WriteString(fmt.Sprintf(" %s\n", oldLine))
			inHunk = false
		}
	}

	return diff.String()
}
