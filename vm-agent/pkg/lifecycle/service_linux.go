//go:build linux

// Package lifecycle handles agent lifecycle management.
package lifecycle

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const systemdServiceTemplate = `[Unit]
Description=VM Agent - Multi-Tenant VM Management Agent
After=network.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/vm-agent run --config %s
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=vm-agent

# Security settings
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/var/lib/vm-agent

[Install]
WantedBy=multi-user.target
`

const systemdServicePath = "/etc/systemd/system/vm-agent.service"

// installLinuxService installs the systemd service
func installLinuxService(configPath string) error {
	// Generate service file content
	content := fmt.Sprintf(systemdServiceTemplate, configPath)

	// Write service file
	if err := os.WriteFile(systemdServicePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}

	// Reload systemd
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	// Enable service
	if err := exec.Command("systemctl", "enable", "vm-agent").Run(); err != nil {
		return fmt.Errorf("failed to enable service: %w", err)
	}

	// Start service
	if err := exec.Command("systemctl", "start", "vm-agent").Run(); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	return nil
}

// getLinuxServiceStatus returns the service status
func getLinuxServiceStatus() string {
	output, err := exec.Command("systemctl", "is-active", "vm-agent").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

// startLinuxService starts the systemd service
func startLinuxService() error {
	return exec.Command("systemctl", "start", "vm-agent").Run()
}

// stopLinuxService stops the systemd service
func stopLinuxService() error {
	return exec.Command("systemctl", "stop", "vm-agent").Run()
}

// removeLinuxService removes the systemd service
func removeLinuxService() error {
	// Stop service
	exec.Command("systemctl", "stop", "vm-agent").Run()

	// Disable service
	exec.Command("systemctl", "disable", "vm-agent").Run()

	// Remove service file
	if err := os.Remove(systemdServicePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove service file: %w", err)
	}

	// Reload systemd
	return exec.Command("systemctl", "daemon-reload").Run()
}

// restartLinuxServiceWithTimeout restarts the service with a timeout
func restartLinuxServiceWithTimeout(timeoutSecs int) error {
	cmd := exec.Command("systemctl", "restart", "vm-agent")
	return cmd.Run()
}

// enableLinuxService enables the service to start on boot
func enableLinuxService() error {
	return exec.Command("systemctl", "enable", "vm-agent").Run()
}

// disableLinuxService disables the service from starting on boot
func disableLinuxService() error {
	return exec.Command("systemctl", "disable", "vm-agent").Run()
}

// getLinuxServiceLogs returns recent service logs
func getLinuxServiceLogs(lines int) (string, error) {
	output, err := exec.Command("journalctl", "-u", "vm-agent", "-n", fmt.Sprintf("%d", lines), "--no-pager").Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}
