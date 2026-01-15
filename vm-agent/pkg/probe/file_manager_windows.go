//go:build windows
// +build windows

// Package probe provides workflow execution functionality.
package probe

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
)

// getDefaultBackupDir returns the default backup directory for Windows systems
func getDefaultBackupDir() string {
	// Use %LOCALAPPDATA%\vm-agent\backups
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		localAppData = os.Getenv("USERPROFILE")
		if localAppData != "" {
			localAppData = localAppData + "\\AppData\\Local"
		} else {
			localAppData = "C:\\ProgramData"
		}
	}
	return localAppData + "\\vm-agent\\backups"
}

// setFilePermissions sets file permissions on Windows systems using ACLs
// mode: Unix octal mode string (e.g., "0644") - mapped to Windows ACLs
// owner: username or SID string
// group: ignored on Windows (uses owner for primary access)
func setFilePermissions(path, mode, owner, group string) error {
	var errs []error

	// Convert Unix mode to Windows ACL
	if mode != "" {
		if err := setWindowsACLFromMode(path, mode, owner); err != nil {
			errs = append(errs, err)
		}
	} else if owner != "" {
		// If no mode but owner specified, grant full control to owner
		if err := setWindowsACLFromMode(path, "0600", owner); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("permission errors: %v", errs)
	}
	return nil
}

// setWindowsACLFromMode converts Unix mode to Windows ACL and applies it
func setWindowsACLFromMode(path, mode, owner string) error {
	// Parse Unix mode
	fileMode, err := ParseUnixMode(mode)
	if err != nil {
		return err
	}

	// Get SIDs for owner and Everyone
	var ownerSID *windows.SID
	if owner != "" {
		ownerSID, err = lookupWindowsSID(owner)
		if err != nil {
			// Fall back to current user
			ownerSID, err = getCurrentUserSID()
			if err != nil {
				return fmt.Errorf("failed to get owner SID: %w", err)
			}
		}
	} else {
		ownerSID, err = getCurrentUserSID()
		if err != nil {
			return fmt.Errorf("failed to get current user SID: %w", err)
		}
	}

	// Get Everyone SID for "other" permissions
	everyoneSID, err := windows.CreateWellKnownSid(windows.WinWorldSid)
	if err != nil {
		return fmt.Errorf("failed to get Everyone SID: %w", err)
	}

	// Build ACL based on Unix mode
	var accessEntries []windows.EXPLICIT_ACCESS

	// Owner permissions (user bits: mode & 0700)
	ownerPerms := (fileMode >> 6) & 0x7
	if ownerAccess := unixPermsToWindowsAccess(ownerPerms); ownerAccess != 0 {
		accessEntries = append(accessEntries, windows.EXPLICIT_ACCESS{
			AccessPermissions: ownerAccess,
			AccessMode:        windows.GRANT_ACCESS,
			Inheritance:       windows.NO_INHERITANCE,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_USER,
				TrusteeValue: windows.TrusteeValueFromSID(ownerSID),
			},
		})
	}

	// Other/World permissions (other bits: mode & 0007)
	otherPerms := fileMode & 0x7
	if otherAccess := unixPermsToWindowsAccess(otherPerms); otherAccess != 0 {
		accessEntries = append(accessEntries, windows.EXPLICIT_ACCESS{
			AccessPermissions: otherAccess,
			AccessMode:        windows.GRANT_ACCESS,
			Inheritance:       windows.NO_INHERITANCE,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_WELL_KNOWN_GROUP,
				TrusteeValue: windows.TrusteeValueFromSID(everyoneSID),
			},
		})
	}

	if len(accessEntries) == 0 {
		// No permissions at all - make it fully restricted to owner
		accessEntries = append(accessEntries, windows.EXPLICIT_ACCESS{
			AccessPermissions: windows.GENERIC_ALL,
			AccessMode:        windows.GRANT_ACCESS,
			Inheritance:       windows.NO_INHERITANCE,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_USER,
				TrusteeValue: windows.TrusteeValueFromSID(ownerSID),
			},
		})
	}

	// Create new ACL
	acl, err := windows.ACLFromEntries(accessEntries, nil)
	if err != nil {
		return fmt.Errorf("failed to create ACL: %w", err)
	}

	// Apply ACL to file
	err = windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		acl,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to set file security: %w", err)
	}

	return nil
}

// unixPermsToWindowsAccess converts Unix permission bits (rwx) to Windows access mask
func unixPermsToWindowsAccess(perms os.FileMode) uint32 {
	var access uint32 = 0

	// Read permission
	if perms&0x4 != 0 {
		access |= windows.GENERIC_READ
	}

	// Write permission
	if perms&0x2 != 0 {
		access |= windows.GENERIC_WRITE
	}

	// Execute permission
	if perms&0x1 != 0 {
		access |= windows.GENERIC_EXECUTE
	}

	return access
}

// lookupWindowsSID looks up a SID by username or SID string
func lookupWindowsSID(nameOrSID string) (*windows.SID, error) {
	// First try to parse as SID string
	sid, err := windows.StringToSid(nameOrSID)
	if err == nil {
		return sid, nil
	}

	// Try to lookup by account name
	sid, _, _, err = windows.LookupSID("", nameOrSID)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup SID for %q: %w", nameOrSID, err)
	}
	return sid, nil
}

// getCurrentUserSID returns the SID of the current user
func getCurrentUserSID() (*windows.SID, error) {
	token := windows.GetCurrentProcessToken()

	// Get token user info
	tokenUser, err := token.GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("failed to get token user: %w", err)
	}

	return tokenUser.User.Sid, nil
}

// getFileOwnership retrieves file ownership information on Windows systems
func getFileOwnership(stat os.FileInfo, info *FileInfo) {
	// On Windows, we need to query the security descriptor
	// For simplicity, we'll use os.Stat path from FileInfo
	path := info.Path
	if path == "" {
		return
	}

	// Get security info
	sd, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION,
	)
	if err != nil {
		return
	}

	owner, _, err := sd.Owner()
	if err != nil {
		return
	}

	// Convert SID to string
	info.Owner = owner.String()

	// Try to get account name
	account, domain, _, err := owner.LookupAccount("")
	if err == nil {
		if domain != "" {
			info.Owner = domain + "\\" + account
		} else {
			info.Owner = account
		}
	}

	// Windows doesn't have numeric UID/GID in the same sense
	// We'll leave OwnerUID and GroupGID as 0
}

// FileAttributeData represents Windows file attribute data
type FileAttributeData struct {
	FileAttributes uint32
	CreationTime   windows.Filetime
	LastAccessTime windows.Filetime
	LastWriteTime  windows.Filetime
	FileSizeHigh   uint32
	FileSizeLow    uint32
}
