// Package health provides health monitoring for the vm-agent.
package health

import (
	"context"
	"os"
	"runtime"
	"time"

	"golang.org/x/sys/unix"
)

// PikoChecker checks the health of the Piko connection
type PikoChecker struct {
	isConnected func() bool
	lastError   func() error
}

// NewPikoChecker creates a new Piko health checker
func NewPikoChecker(isConnected func() bool, lastError func() error) *PikoChecker {
	return &PikoChecker{
		isConnected: isConnected,
		lastError:   lastError,
	}
}

// Name returns the checker name
func (c *PikoChecker) Name() string {
	return "piko"
}

// Check performs the health check
func (c *PikoChecker) Check(ctx context.Context) *Component {
	component := &Component{
		Name:        c.Name(),
		LastChecked: time.Now(),
		Details:     make(map[string]any),
	}

	if c.isConnected() {
		component.Status = StatusHealthy
		component.Message = "connected to Piko server"
	} else {
		component.Status = StatusUnhealthy
		if err := c.lastError(); err != nil {
			component.Message = err.Error()
		} else {
			component.Message = "disconnected from Piko server"
		}
	}

	return component
}

// WebhookChecker checks the health of the webhook server
type WebhookChecker struct {
	isRunning func() bool
	port      int
}

// NewWebhookChecker creates a new webhook health checker
func NewWebhookChecker(isRunning func() bool, port int) *WebhookChecker {
	return &WebhookChecker{
		isRunning: isRunning,
		port:      port,
	}
}

// Name returns the checker name
func (c *WebhookChecker) Name() string {
	return "webhook"
}

// Check performs the health check
func (c *WebhookChecker) Check(ctx context.Context) *Component {
	component := &Component{
		Name:        c.Name(),
		LastChecked: time.Now(),
		Details: map[string]any{
			"port": c.port,
		},
	}

	if c.isRunning() {
		component.Status = StatusHealthy
		component.Message = "webhook server running"
	} else {
		component.Status = StatusUnhealthy
		component.Message = "webhook server not running"
	}

	return component
}

// ProbeChecker checks the health of the probe executor
type ProbeChecker struct {
	activeJobs     func() int
	maxConcurrent  int
	lastExecStatus func() error
}

// NewProbeChecker creates a new probe health checker
func NewProbeChecker(activeJobs func() int, maxConcurrent int, lastExecStatus func() error) *ProbeChecker {
	return &ProbeChecker{
		activeJobs:     activeJobs,
		maxConcurrent:  maxConcurrent,
		lastExecStatus: lastExecStatus,
	}
}

// Name returns the checker name
func (c *ProbeChecker) Name() string {
	return "probe"
}

// Check performs the health check
func (c *ProbeChecker) Check(ctx context.Context) *Component {
	activeJobs := c.activeJobs()
	component := &Component{
		Name:        c.Name(),
		LastChecked: time.Now(),
		Details: map[string]any{
			"active_jobs":   activeJobs,
			"max_concurrent": c.maxConcurrent,
		},
	}

	if activeJobs >= c.maxConcurrent {
		component.Status = StatusDegraded
		component.Message = "at maximum concurrent jobs"
	} else {
		component.Status = StatusHealthy
		component.Message = "probe executor ready"
	}

	return component
}

// SystemChecker checks system resources
type SystemChecker struct {
	minDiskSpace uint64 // minimum disk space in bytes
	dataDir      string
}

// NewSystemChecker creates a new system health checker
func NewSystemChecker(minDiskSpace uint64, dataDir string) *SystemChecker {
	return &SystemChecker{
		minDiskSpace: minDiskSpace,
		dataDir:      dataDir,
	}
}

// Name returns the checker name
func (c *SystemChecker) Name() string {
	return "system"
}

// Check performs the health check
func (c *SystemChecker) Check(ctx context.Context) *Component {
	component := &Component{
		Name:        c.Name(),
		LastChecked: time.Now(),
		Details:     make(map[string]any),
	}

	// Check memory
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	component.Details["memory_alloc_mb"] = memStats.Alloc / 1024 / 1024
	component.Details["memory_sys_mb"] = memStats.Sys / 1024 / 1024
	component.Details["goroutines"] = runtime.NumGoroutine()

	// Check disk space (Linux-specific)
	if c.dataDir != "" {
		if diskFree, diskTotal, err := getDiskSpace(c.dataDir); err == nil {
			component.Details["disk_free_gb"] = diskFree / 1024 / 1024 / 1024
			component.Details["disk_total_gb"] = diskTotal / 1024 / 1024 / 1024
			component.Details["disk_free_pct"] = float64(diskFree) / float64(diskTotal) * 100

			if diskFree < c.minDiskSpace {
				component.Status = StatusDegraded
				component.Message = "low disk space"
				return component
			}
		}
	}

	component.Status = StatusHealthy
	component.Message = "system resources OK"
	return component
}

// getDiskSpace returns free and total disk space for a path
func getDiskSpace(path string) (free, total uint64, err error) {
	var stat unix.Statfs_t
	if err = unix.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	free = stat.Bavail * uint64(stat.Bsize)
	total = stat.Blocks * uint64(stat.Bsize)
	return
}

// ControlPlaneChecker checks connectivity to control plane
type ControlPlaneChecker struct {
	isConnected func() bool
	lastPing    func() time.Time
}

// NewControlPlaneChecker creates a new control plane health checker
func NewControlPlaneChecker(isConnected func() bool, lastPing func() time.Time) *ControlPlaneChecker {
	return &ControlPlaneChecker{
		isConnected: isConnected,
		lastPing:    lastPing,
	}
}

// Name returns the checker name
func (c *ControlPlaneChecker) Name() string {
	return "control_plane"
}

// Check performs the health check
func (c *ControlPlaneChecker) Check(ctx context.Context) *Component {
	component := &Component{
		Name:        c.Name(),
		LastChecked: time.Now(),
		Details:     make(map[string]any),
	}

	lastPing := c.lastPing()
	component.Details["last_ping"] = lastPing

	if c.isConnected() {
		component.Status = StatusHealthy
		component.Message = "connected to control plane"
	} else {
		// Check if we've been disconnected for too long
		if time.Since(lastPing) > 10*time.Minute {
			component.Status = StatusUnhealthy
			component.Message = "disconnected from control plane for >10 minutes"
		} else {
			component.Status = StatusDegraded
			component.Message = "temporarily disconnected from control plane"
		}
	}

	return component
}

// SelfChecker checks the agent's own health
type SelfChecker struct {
	startTime time.Time
	pid       int
}

// NewSelfChecker creates a new self health checker
func NewSelfChecker() *SelfChecker {
	return &SelfChecker{
		startTime: time.Now(),
		pid:       os.Getpid(),
	}
}

// Name returns the checker name
func (c *SelfChecker) Name() string {
	return "self"
}

// Check performs the health check
func (c *SelfChecker) Check(ctx context.Context) *Component {
	return &Component{
		Name:        c.Name(),
		Status:      StatusHealthy,
		Message:     "agent running",
		LastChecked: time.Now(),
		Details: map[string]any{
			"pid":       c.pid,
			"uptime_s":  time.Since(c.startTime).Seconds(),
			"go_version": runtime.Version(),
		},
	}
}
