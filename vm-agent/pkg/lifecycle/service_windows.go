//go:build windows

// Package lifecycle handles agent lifecycle management.
package lifecycle

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const serviceName = "vm-agent"
const serviceDisplayName = "VM Agent"
const serviceDescription = "Multi-Tenant VM Management Agent"

// installWindowsService installs the Windows service
func installWindowsService(configPath string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	// Check if service already exists
	s, err := m.OpenService(serviceName)
	if err == nil {
		s.Close()
		return fmt.Errorf("service already exists")
	}

	// Create service
	config := mgr.Config{
		DisplayName:  serviceDisplayName,
		Description:  serviceDescription,
		StartType:    mgr.StartAutomatic,
		ErrorControl: mgr.ErrorNormal,
	}

	s, err = m.CreateService(serviceName, exePath, config, "run", "--config", configPath)
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}
	defer s.Close()

	// Set recovery options
	recoveryActions := []mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 10 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
	}
	if err := s.SetRecoveryActions(recoveryActions, 86400); err != nil {
		// Non-fatal error
	}

	// Start service
	if err := s.Start(); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	return nil
}

// getWindowsServiceStatus returns the service status
func getWindowsServiceStatus() string {
	m, err := mgr.Connect()
	if err != nil {
		return "unknown"
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return "not_installed"
	}
	defer s.Close()

	status, err := s.Query()
	if err != nil {
		return "unknown"
	}

	switch status.State {
	case svc.Running:
		return "running"
	case svc.Stopped:
		return "stopped"
	case svc.StartPending:
		return "starting"
	case svc.StopPending:
		return "stopping"
	case svc.Paused:
		return "paused"
	default:
		return "unknown"
	}
}

// startWindowsService starts the Windows service
func startWindowsService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("failed to open service: %w", err)
	}
	defer s.Close()

	return s.Start()
}

// stopWindowsService stops the Windows service
func stopWindowsService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("failed to open service: %w", err)
	}
	defer s.Close()

	status, err := s.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("failed to stop service: %w", err)
	}

	// Wait for service to stop
	timeout := time.Now().Add(30 * time.Second)
	for status.State != svc.Stopped {
		if time.Now().After(timeout) {
			return fmt.Errorf("timeout waiting for service to stop")
		}
		time.Sleep(500 * time.Millisecond)
		status, err = s.Query()
		if err != nil {
			return fmt.Errorf("failed to query service status: %w", err)
		}
	}

	return nil
}

// removeWindowsService removes the Windows service
func removeWindowsService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("failed to open service: %w", err)
	}
	defer s.Close()

	// Stop if running
	status, _ := s.Query()
	if status.State == svc.Running {
		s.Control(svc.Stop)
		time.Sleep(2 * time.Second)
	}

	// Delete service
	return s.Delete()
}

// WindowsService implements the Windows service interface
type WindowsService struct {
	stopCh chan struct{}
	run    func() error
}

// Execute implements svc.Handler
func (ws *WindowsService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown

	changes <- svc.Status{State: svc.StartPending}

	// Start the actual service
	go func() {
		if err := ws.run(); err != nil {
			// Log error
		}
	}()

	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				close(ws.stopCh)
				return false, 0
			default:
				// Unexpected control request
			}
		}
	}
}

// RunAsService runs the agent as a Windows service
func RunAsService(run func() error) error {
	ws := &WindowsService{
		stopCh: make(chan struct{}),
		run:    run,
	}

	return svc.Run(serviceName, ws)
}

// IsWindowsService returns true if running as a Windows service
func IsWindowsService() bool {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return false
	}
	return isService
}
