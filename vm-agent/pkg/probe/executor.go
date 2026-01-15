// Package probe provides workflow execution functionality.
package probe

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Executor executes workflows
type Executor struct {
	mu           sync.RWMutex
	workDir      string
	maxConcurrent int
	activeJobs   int32
	jobs         map[string]*Job
	logger       *zap.Logger
	semaphore    chan struct{}
}

// ExecutorConfig contains executor configuration
type ExecutorConfig struct {
	WorkDir       string
	MaxConcurrent int
}

// Job represents a running workflow job
type Job struct {
	ID         string
	Workflow   *Workflow
	Result     *WorkflowResult
	Status     StepStatus
	StartedAt  time.Time
	EndedAt    time.Time
	CancelFunc context.CancelFunc
	Done       chan struct{}
}

// NewExecutor creates a new workflow executor
func NewExecutor(cfg *ExecutorConfig, logger *zap.Logger) (*Executor, error) {
	// Create work directory if it doesn't exist
	if err := os.MkdirAll(cfg.WorkDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create work directory: %w", err)
	}

	maxConcurrent := cfg.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 5
	}

	return &Executor{
		workDir:       cfg.WorkDir,
		maxConcurrent: maxConcurrent,
		jobs:          make(map[string]*Job),
		logger:        logger,
		semaphore:     make(chan struct{}, maxConcurrent),
	}, nil
}

// Execute starts workflow execution
func (e *Executor) Execute(workflowData []byte) (string, error) {
	workflow, err := ParseWorkflow(workflowData)
	if err != nil {
		return "", fmt.Errorf("failed to parse workflow: %w", err)
	}

	if err := workflow.Validate(); err != nil {
		return "", fmt.Errorf("workflow validation failed: %w", err)
	}

	// Generate workflow ID if not provided
	if workflow.ID == "" {
		workflow.ID = uuid.New().String()
	}

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())

	job := &Job{
		ID:         workflow.ID,
		Workflow:   workflow,
		Status:     StepStatusPending,
		CancelFunc: cancel,
		Done:       make(chan struct{}),
		Result: &WorkflowResult{
			WorkflowID: workflow.ID,
			Name:       workflow.Name,
			Status:     StepStatusPending,
			Steps:      make([]StepResult, 0),
		},
	}

	e.mu.Lock()
	e.jobs[job.ID] = job
	e.mu.Unlock()

	// Start execution in background
	go e.executeJob(ctx, job)

	return job.ID, nil
}

// executeJob executes a workflow job
func (e *Executor) executeJob(ctx context.Context, job *Job) {
	defer close(job.Done)

	// Acquire semaphore
	select {
	case e.semaphore <- struct{}{}:
		defer func() { <-e.semaphore }()
	case <-ctx.Done():
		job.Status = StepStatusCancelled
		job.Result.Status = StepStatusCancelled
		return
	}

	atomic.AddInt32(&e.activeJobs, 1)
	defer atomic.AddInt32(&e.activeJobs, -1)

	job.StartedAt = time.Now()
	job.Status = StepStatusRunning
	job.Result.StartedAt = job.StartedAt
	job.Result.Status = StepStatusRunning

	workflow := job.Workflow

	// Create workflow timeout context
	if workflow.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, workflow.Timeout)
		defer cancel()
	}

	e.logger.Info("starting workflow execution",
		zap.String("workflow_id", job.ID),
		zap.String("workflow_name", workflow.Name))

	// Execute steps
	success := true
	for _, step := range workflow.Steps {
		select {
		case <-ctx.Done():
			job.Status = StepStatusCancelled
			job.Result.Status = StepStatusCancelled
			e.executeHooks(ctx, job, workflow.OnCancel)
			return
		default:
		}

		result := e.executeStep(ctx, job, &step)
		job.Result.Steps = append(job.Result.Steps, *result)

		if result.Status == StepStatusFailed && !step.ContinueOnError {
			success = false
			break
		}
	}

	job.EndedAt = time.Now()
	job.Result.EndedAt = job.EndedAt
	job.Result.Duration = job.EndedAt.Sub(job.StartedAt)

	if success {
		job.Status = StepStatusSuccess
		job.Result.Status = StepStatusSuccess
		e.executeHooks(ctx, job, workflow.OnSuccess)
	} else {
		job.Status = StepStatusFailed
		job.Result.Status = StepStatusFailed
		e.executeHooks(ctx, job, workflow.OnFailure)
	}

	e.logger.Info("workflow execution completed",
		zap.String("workflow_id", job.ID),
		zap.String("status", string(job.Status)),
		zap.Duration("duration", job.Result.Duration))
}

// executeStep executes a single step
func (e *Executor) executeStep(ctx context.Context, job *Job, step *Step) *StepResult {
	result := &StepResult{
		StepID:    step.ID,
		StepName:  step.Name,
		Status:    StepStatusRunning,
		StartedAt: time.Now(),
	}

	// Check condition
	if step.Condition != "" {
		if !e.evaluateCondition(ctx, step.Condition, job) {
			result.Status = StepStatusSkipped
			result.EndedAt = time.Now()
			result.Duration = result.EndedAt.Sub(result.StartedAt)
			return result
		}
	}

	// Create step timeout context
	stepCtx := ctx
	if step.Timeout > 0 {
		var cancel context.CancelFunc
		stepCtx, cancel = context.WithTimeout(ctx, step.Timeout)
		defer cancel()
	}

	// Execute with retries
	var lastErr error
	for attempt := 0; attempt <= step.RetryCount; attempt++ {
		result.RetryCount = attempt

		if attempt > 0 {
			e.logger.Info("retrying step",
				zap.String("step_id", step.ID),
				zap.Int("attempt", attempt))
			time.Sleep(step.RetryDelay)
		}

		var output string
		var exitCode int
		var err error

		switch step.Type {
		case StepTypeCommand:
			output, exitCode, err = e.executeCommand(stepCtx, step, job)
		case StepTypeScript:
			output, exitCode, err = e.executeScript(stepCtx, step, job)
		default:
			err = fmt.Errorf("unsupported step type: %s", step.Type)
			exitCode = 1
		}

		result.Output = output
		result.ExitCode = exitCode

		if err == nil && exitCode == 0 {
			result.Status = StepStatusSuccess
			break
		}

		lastErr = err
		if exitCode != 0 {
			lastErr = fmt.Errorf("command exited with code %d", exitCode)
		}
	}

	if result.Status != StepStatusSuccess {
		result.Status = StepStatusFailed
		if lastErr != nil {
			result.Error = lastErr.Error()
		}
	}

	result.EndedAt = time.Now()
	result.Duration = result.EndedAt.Sub(result.StartedAt)

	e.logger.Info("step completed",
		zap.String("workflow_id", job.ID),
		zap.String("step_id", step.ID),
		zap.String("status", string(result.Status)),
		zap.Duration("duration", result.Duration))

	return result
}

// executeCommand executes a command step
func (e *Executor) executeCommand(ctx context.Context, step *Step, job *Job) (string, int, error) {
	var cmd *exec.Cmd

	if len(step.Args) > 0 {
		cmd = exec.CommandContext(ctx, step.Args[0], step.Args[1:]...)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", step.Command)
	}

	// Set working directory
	if step.WorkDir != "" {
		cmd.Dir = step.WorkDir
	} else {
		cmd.Dir = e.workDir
	}

	// Set environment
	cmd.Env = os.Environ()
	for k, v := range job.Workflow.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	for k, v := range step.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\n--- stderr ---\n" + stderr.String()
	}

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return output, 1, err
		}
	}

	return output, exitCode, nil
}

// executeScript executes a script step
func (e *Executor) executeScript(ctx context.Context, step *Step, job *Job) (string, int, error) {
	// Create temporary script file
	tmpDir := filepath.Join(e.workDir, "scripts")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", 1, fmt.Errorf("failed to create script directory: %w", err)
	}

	scriptPath := filepath.Join(tmpDir, fmt.Sprintf("%s-%s.sh", job.ID, step.ID))
	if err := os.WriteFile(scriptPath, []byte(step.Script), 0755); err != nil {
		return "", 1, fmt.Errorf("failed to write script: %w", err)
	}
	defer os.Remove(scriptPath)

	// Execute the script
	step.Args = []string{"sh", scriptPath}
	return e.executeCommand(ctx, step, job)
}

// executeHooks executes workflow hooks
func (e *Executor) executeHooks(ctx context.Context, job *Job, hooks []Step) {
	for _, hook := range hooks {
		result := e.executeStep(ctx, job, &hook)
		job.Result.Steps = append(job.Result.Steps, *result)
	}
}

// evaluateCondition evaluates a step condition
func (e *Executor) evaluateCondition(ctx context.Context, condition string, job *Job) bool {
	// Simple condition evaluation - executes as shell command
	cmd := exec.CommandContext(ctx, "sh", "-c", condition)
	cmd.Dir = e.workDir
	return cmd.Run() == nil
}

// GetStatus returns the status of a workflow
func (e *Executor) GetStatus(workflowID string) (*WorkflowResult, error) {
	e.mu.RLock()
	job, ok := e.jobs[workflowID]
	e.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("workflow not found: %s", workflowID)
	}

	return job.Result, nil
}

// Cancel cancels a running workflow
func (e *Executor) Cancel(workflowID string) error {
	e.mu.RLock()
	job, ok := e.jobs[workflowID]
	e.mu.RUnlock()

	if !ok {
		return fmt.Errorf("workflow not found: %s", workflowID)
	}

	if job.CancelFunc != nil {
		job.CancelFunc()
	}

	return nil
}

// ActiveJobs returns the number of active jobs
func (e *Executor) ActiveJobs() int {
	return int(atomic.LoadInt32(&e.activeJobs))
}

// WaitForJob waits for a job to complete
func (e *Executor) WaitForJob(workflowID string) error {
	e.mu.RLock()
	job, ok := e.jobs[workflowID]
	e.mu.RUnlock()

	if !ok {
		return fmt.Errorf("workflow not found: %s", workflowID)
	}

	<-job.Done
	return nil
}

// StreamOutput streams the output of a running job
func (e *Executor) StreamOutput(workflowID string, w io.Writer) error {
	e.mu.RLock()
	job, ok := e.jobs[workflowID]
	e.mu.RUnlock()

	if !ok {
		return fmt.Errorf("workflow not found: %s", workflowID)
	}

	// Wait for job to complete while streaming output
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	lastStep := 0
	for {
		select {
		case <-job.Done:
			// Stream final output
			for i := lastStep; i < len(job.Result.Steps); i++ {
				fmt.Fprintf(w, "[%s] %s\n", job.Result.Steps[i].Status, job.Result.Steps[i].Output)
			}
			return nil
		case <-ticker.C:
			// Stream new output
			for i := lastStep; i < len(job.Result.Steps); i++ {
				fmt.Fprintf(w, "[%s] %s\n", job.Result.Steps[i].Status, job.Result.Steps[i].Output)
			}
			lastStep = len(job.Result.Steps)
		}
	}
}

// Cleanup removes completed jobs older than maxAge
func (e *Executor) Cleanup(maxAge time.Duration) int {
	e.mu.Lock()
	defer e.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	removed := 0

	for id, job := range e.jobs {
		if job.Status != StepStatusRunning && job.EndedAt.Before(cutoff) {
			delete(e.jobs, id)
			removed++
		}
	}

	return removed
}
