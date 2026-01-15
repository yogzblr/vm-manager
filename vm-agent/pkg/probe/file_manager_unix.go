//go:build !windows
// +build !windows

// Package probe provides workflow execution functionality.
package probe

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"syscall"
)

// getDefaultBackupDir returns the default backup directory for Unix systems
func getDefaultBackupDir() string {
	return "/var/lib/vm-agent/backups"
}

// setFilePermissions sets file permissions on Unix systems
// mode: Unix octal mode string (e.g., "0644")
// owner: username or UID
// group: group name or GID
func setFilePermissions(path, mode, owner, group string) error {
	var errs []error

	// Set file mode
	fileMode := os.FileMode(0644)
	if mode != "" {
		if parsed, err := strconv.ParseUint(mode, 8, 32); err == nil {
			fileMode = os.FileMode(parsed)
		} else {
			errs = append(errs, fmt.Errorf("invalid mode %q: %w", mode, err))
		}
	}
	if err := os.Chmod(path, fileMode); err != nil {
		errs = append(errs, fmt.Errorf("failed to set file mode: %w", err))
	}

	// Set owner and group
	if owner != "" || group != "" {
		uid, gid := -1, -1

		if owner != "" {
			if u, err := user.Lookup(owner); err == nil {
				uid, _ = strconv.Atoi(u.Uid)
			} else if parsed, err := strconv.Atoi(owner); err == nil {
				uid = parsed
			} else {
				errs = append(errs, fmt.Errorf("unknown user: %s", owner))
			}
		}

		if group != "" {
			if g, err := user.LookupGroup(group); err == nil {
				gid, _ = strconv.Atoi(g.Gid)
			} else if parsed, err := strconv.Atoi(group); err == nil {
				gid = parsed
			} else {
				errs = append(errs, fmt.Errorf("unknown group: %s", group))
			}
		}

		if uid != -1 || gid != -1 {
			if err := os.Chown(path, uid, gid); err != nil {
				errs = append(errs, fmt.Errorf("failed to set ownership: %w", err))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("permission errors: %v", errs)
	}
	return nil
}

// getFileOwnership retrieves file ownership information on Unix systems
func getFileOwnership(stat os.FileInfo, info *FileInfo) {
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
}
