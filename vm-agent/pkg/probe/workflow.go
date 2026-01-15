// Package probe provides workflow execution functionality.
package probe

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Workflow represents a workflow definition
type Workflow struct {
	ID          string                 `yaml:"id" json:"id"`
	Name        string                 `yaml:"name" json:"name"`
	Description string                 `yaml:"description,omitempty" json:"description,omitempty"`
	Version     string                 `yaml:"version,omitempty" json:"version,omitempty"`
	Timeout     time.Duration          `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Env         map[string]string      `yaml:"env,omitempty" json:"env,omitempty"`
	Vars        map[string]interface{} `yaml:"vars,omitempty" json:"vars,omitempty"` // Template variables (like Salt Pillar)
	Steps       []Step                 `yaml:"steps" json:"steps"`
	OnSuccess   []Step                 `yaml:"on_success,omitempty" json:"on_success,omitempty"`
	OnFailure   []Step                 `yaml:"on_failure,omitempty" json:"on_failure,omitempty"`
	OnCancel    []Step                 `yaml:"on_cancel,omitempty" json:"on_cancel,omitempty"`
}

// Step represents a single step in a workflow
type Step struct {
	ID              string            `yaml:"id" json:"id"`
	Name            string            `yaml:"name" json:"name"`
	Type            StepType          `yaml:"type" json:"type"`
	Command         string            `yaml:"command,omitempty" json:"command,omitempty"`
	Script          string            `yaml:"script,omitempty" json:"script,omitempty"`
	Args            []string          `yaml:"args,omitempty" json:"args,omitempty"`
	Env             map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	WorkDir         string            `yaml:"work_dir,omitempty" json:"work_dir,omitempty"`
	Timeout         time.Duration     `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	RetryCount      int               `yaml:"retry_count,omitempty" json:"retry_count,omitempty"`
	RetryDelay      time.Duration     `yaml:"retry_delay,omitempty" json:"retry_delay,omitempty"`
	ContinueOnError bool              `yaml:"continue_on_error,omitempty" json:"continue_on_error,omitempty"`
	Condition       string            `yaml:"condition,omitempty" json:"condition,omitempty"`
	RunAs           string            `yaml:"run_as,omitempty" json:"run_as,omitempty"`
	Template        *TemplateConfig   `yaml:"template,omitempty" json:"template,omitempty"` // Template step configuration
}

// TemplateConfig contains configuration for template steps
// Cross-platform notes:
//   - Mode: Unix octal (e.g., "0644") - on Windows, mapped to ACLs
//   - Owner: username/UID (Unix) or username/SID (Windows)
//   - Group: group name/GID (Unix only, ignored on Windows)
type TemplateConfig struct {
	// Source is the template source (HTTP URL or control-plane://templates/{id})
	Source string `yaml:"source" json:"source"`
	// Dest is the destination path (supports variable interpolation)
	Dest string `yaml:"dest" json:"dest"`
	// Mode is the file permissions in Unix octal format (e.g., "0644", "0755")
	// On Windows: mapped to ACLs - owner gets user bits, Everyone gets other bits
	// Example: "0644" → owner: read+write, Everyone: read
	Mode string `yaml:"mode,omitempty" json:"mode,omitempty"`
	// Owner is the file owner
	// Unix: username or numeric UID
	// Windows: username or SID string (e.g., "S-1-5-21-...")
	Owner string `yaml:"owner,omitempty" json:"owner,omitempty"`
	// Group is the file group (Unix only, ignored on Windows)
	// Unix: group name or numeric GID
	Group string `yaml:"group,omitempty" json:"group,omitempty"`
	// Backup enables creating a backup before overwriting
	Backup bool `yaml:"backup,omitempty" json:"backup,omitempty"`
	// DiffOnly only reports diff without writing
	DiffOnly bool `yaml:"diff_only,omitempty" json:"diff_only,omitempty"`
	// CreateDirs creates parent directories if they don't exist
	CreateDirs bool `yaml:"create_dirs,omitempty" json:"create_dirs,omitempty"`
}

// StepType represents the type of step
type StepType string

const (
	StepTypeCommand  StepType = "command"
	StepTypeScript   StepType = "script"
	StepTypeFile     StepType = "file"
	StepTypeHTTP     StepType = "http"
	StepTypeValidate StepType = "validate"
	StepTypeTemplate StepType = "template" // Salt Stack-like template deployment
)

// ParseWorkflow parses a workflow from YAML
func ParseWorkflow(data []byte) (*Workflow, error) {
	var workflow Workflow
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		return nil, fmt.Errorf("failed to parse workflow: %w", err)
	}

	// Set defaults
	if workflow.Timeout == 0 {
		workflow.Timeout = 30 * time.Minute
	}

	for i := range workflow.Steps {
		if workflow.Steps[i].Timeout == 0 {
			workflow.Steps[i].Timeout = 5 * time.Minute
		}
		if workflow.Steps[i].Type == "" {
			workflow.Steps[i].Type = StepTypeCommand
		}
	}

	return &workflow, nil
}

// Validate validates a workflow
func (w *Workflow) Validate() error {
	if w.Name == "" {
		return fmt.Errorf("workflow name is required")
	}

	if len(w.Steps) == 0 {
		return fmt.Errorf("workflow must have at least one step")
	}

	seenIDs := make(map[string]bool)
	for i, step := range w.Steps {
		if step.ID == "" {
			return fmt.Errorf("step %d: ID is required", i)
		}
		if seenIDs[step.ID] {
			return fmt.Errorf("step %d: duplicate ID %q", i, step.ID)
		}
		seenIDs[step.ID] = true

		if step.Name == "" {
			return fmt.Errorf("step %s: name is required", step.ID)
		}

		if err := step.Validate(); err != nil {
			return fmt.Errorf("step %s: %w", step.ID, err)
		}
	}

	return nil
}

// Validate validates a step
func (s *Step) Validate() error {
	switch s.Type {
	case StepTypeCommand:
		if s.Command == "" && len(s.Args) == 0 {
			return fmt.Errorf("command or args required for command step")
		}
	case StepTypeScript:
		if s.Script == "" {
			return fmt.Errorf("script required for script step")
		}
	case StepTypeTemplate:
		if s.Template == nil {
			return fmt.Errorf("template configuration required for template step")
		}
		if err := s.Template.Validate(); err != nil {
			return fmt.Errorf("template config: %w", err)
		}
	case StepTypeFile:
		// File operations validated at execution time
	case StepTypeHTTP:
		// HTTP operations validated at execution time
	case StepTypeValidate:
		// Validation operations validated at execution time
	default:
		return fmt.Errorf("unknown step type: %s", s.Type)
	}

	if s.Timeout < 0 {
		return fmt.Errorf("timeout must be positive")
	}

	if s.RetryCount < 0 {
		return fmt.Errorf("retry_count must be non-negative")
	}

	return nil
}

// Validate validates a template configuration
func (t *TemplateConfig) Validate() error {
	if t.Source == "" {
		return fmt.Errorf("source is required")
	}
	if t.Dest == "" {
		return fmt.Errorf("dest is required")
	}
	return nil
}

// StepResult represents the result of a step execution
type StepResult struct {
	StepID      string        `json:"step_id"`
	StepName    string        `json:"step_name"`
	Status      StepStatus    `json:"status"`
	ExitCode    int           `json:"exit_code"`
	Output      string        `json:"output"`
	Error       string        `json:"error,omitempty"`
	StartedAt   time.Time     `json:"started_at"`
	EndedAt     time.Time     `json:"ended_at"`
	Duration    time.Duration `json:"duration"`
	RetryCount  int           `json:"retry_count"`
}

// StepStatus represents the status of a step
type StepStatus string

const (
	StepStatusPending   StepStatus = "pending"
	StepStatusRunning   StepStatus = "running"
	StepStatusSuccess   StepStatus = "success"
	StepStatusFailed    StepStatus = "failed"
	StepStatusSkipped   StepStatus = "skipped"
	StepStatusCancelled StepStatus = "cancelled"
)

// WorkflowResult represents the result of a workflow execution
type WorkflowResult struct {
	WorkflowID  string        `json:"workflow_id"`
	Name        string        `json:"name"`
	Status      StepStatus    `json:"status"`
	Steps       []StepResult  `json:"steps"`
	StartedAt   time.Time     `json:"started_at"`
	EndedAt     time.Time     `json:"ended_at"`
	Duration    time.Duration `json:"duration"`
	Error       string        `json:"error,omitempty"`
}
