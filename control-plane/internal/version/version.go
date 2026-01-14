// Package version provides build version information for the control plane.
package version

import (
	"fmt"
	"runtime"
)

// Build-time variables set via ldflags
var (
	Version   = "dev"
	BuildDate = "unknown"
	GitCommit = "unknown"
)

// Info contains version information
type Info struct {
	Version   string `json:"version"`
	BuildDate string `json:"build_date"`
	GitCommit string `json:"git_commit"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

// GetInfo returns the version information
func GetInfo() Info {
	return Info{
		Version:   Version,
		BuildDate: BuildDate,
		GitCommit: GitCommit,
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// String returns a formatted version string
func (i Info) String() string {
	return fmt.Sprintf("control-plane %s (commit: %s, built: %s, %s, %s)",
		i.Version, i.GitCommit, i.BuildDate, i.GoVersion, i.Platform)
}
