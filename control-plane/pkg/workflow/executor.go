// Package workflow provides workflow management for the control plane.
package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/yourorg/control-plane/pkg/db/models"
)

// Executor executes workflows on agents
type Executor struct {
	db         *gorm.DB
	pikoURL    string
	httpClient *http.Client
	logger     *zap.Logger
}

// NewExecutor creates a new workflow executor
func NewExecutor(db *gorm.DB, pikoURL string, logger *zap.Logger) *Executor {
	return &Executor{
		db:      db,
		pikoURL: pikoURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

// ExecuteRequest represents a request to execute a workflow
type ExecuteRequest struct {
	TenantID   string `json:"tenant_id" binding:"required"`
	WorkflowID string `json:"workflow_id" binding:"required"`
	AgentID    string `json:"agent_id" binding:"required"`
	CampaignID string `json:"campaign_id"`
}

// Execute starts workflow execution on an agent
func (e *Executor) Execute(ctx context.Context, req *ExecuteRequest) (*models.WorkflowExecution, error) {
	// Get workflow
	var workflow models.Workflow
	if err := e.db.Where("id = ? AND tenant_id = ?", req.WorkflowID, req.TenantID).First(&workflow).Error; err != nil {
		return nil, fmt.Errorf("workflow not found: %w", err)
	}

	if workflow.Status != models.WorkflowStatusActive {
		return nil, fmt.Errorf("workflow is not active")
	}

	// Verify agent exists
	var agent models.Agent
	if err := e.db.Where("id = ? AND tenant_id = ?", req.AgentID, req.TenantID).First(&agent).Error; err != nil {
		return nil, fmt.Errorf("agent not found: %w", err)
	}

	// Create execution record
	execution := &models.WorkflowExecution{
		ID:         uuid.New().String(),
		WorkflowID: req.WorkflowID,
		TenantID:   req.TenantID,
		AgentID:    req.AgentID,
		Status:     models.ExecutionStatusPending,
		CreatedAt:  time.Now(),
	}

	if req.CampaignID != "" {
		execution.CampaignID = &req.CampaignID
	}

	if err := e.db.Create(execution).Error; err != nil {
		return nil, fmt.Errorf("failed to create execution: %w", err)
	}

	// Send to agent via Piko
	go e.sendToAgent(execution, &workflow, &agent)

	e.logger.Info("workflow execution started",
		zap.String("execution_id", execution.ID),
		zap.String("workflow_id", req.WorkflowID),
		zap.String("agent_id", req.AgentID))

	return execution, nil
}

// sendToAgent sends the workflow to the agent for execution
func (e *Executor) sendToAgent(execution *models.WorkflowExecution, workflow *models.Workflow, agent *models.Agent) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Update status to running
	e.db.Model(execution).Updates(map[string]interface{}{
		"status":     models.ExecutionStatusRunning,
		"started_at": time.Now(),
	})

	// Build Piko endpoint URL
	endpoint := fmt.Sprintf("tenant-%s/%s", agent.TenantID, agent.ID)
	url := fmt.Sprintf("%s/piko/v1/proxy/%s/workflow/execute", e.pikoURL, endpoint)

	// Prepare workflow payload
	payload, err := json.Marshal(workflow.Definition)
	if err != nil {
		e.markFailed(execution, fmt.Sprintf("failed to marshal workflow: %v", err))
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		e.markFailed(execution, fmt.Sprintf("failed to create request: %v", err))
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		e.markFailed(execution, fmt.Sprintf("failed to send to agent: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		e.markFailed(execution, fmt.Sprintf("agent returned status %d", resp.StatusCode))
		return
	}

	// Parse response
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		e.logger.Warn("failed to decode agent response", zap.Error(err))
	}

	e.logger.Info("workflow sent to agent",
		zap.String("execution_id", execution.ID),
		zap.String("agent_id", agent.ID))
}

// markFailed marks an execution as failed
func (e *Executor) markFailed(execution *models.WorkflowExecution, errorMsg string) {
	now := time.Now()
	e.db.Model(execution).Updates(map[string]interface{}{
		"status":       models.ExecutionStatusFailed,
		"completed_at": now,
		"result": map[string]interface{}{
			"error": errorMsg,
		},
	})

	e.logger.Error("workflow execution failed",
		zap.String("execution_id", execution.ID),
		zap.String("error", errorMsg))
}

// UpdateExecutionResult updates the result of an execution
func (e *Executor) UpdateExecutionResult(ctx context.Context, executionID string, status models.ExecutionStatus, result map[string]interface{}) error {
	updates := map[string]interface{}{
		"status": status,
		"result": result,
	}

	if status == models.ExecutionStatusSuccess || status == models.ExecutionStatusFailed || status == models.ExecutionStatusCancelled {
		updates["completed_at"] = time.Now()
	}

	return e.db.Model(&models.WorkflowExecution{}).Where("id = ?", executionID).Updates(updates).Error
}

// GetExecution retrieves an execution by ID
func (e *Executor) GetExecution(ctx context.Context, tenantID, executionID string) (*models.WorkflowExecution, error) {
	var execution models.WorkflowExecution
	if err := e.db.Where("id = ? AND tenant_id = ?", executionID, tenantID).First(&execution).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("execution not found")
		}
		return nil, err
	}
	return &execution, nil
}

// ListExecutions lists executions
func (e *Executor) ListExecutions(ctx context.Context, tenantID string, workflowID string, limit, offset int) ([]models.WorkflowExecution, int64, error) {
	query := e.db.Model(&models.WorkflowExecution{}).Where("tenant_id = ?", tenantID)

	if workflowID != "" {
		query = query.Where("workflow_id = ?", workflowID)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	var executions []models.WorkflowExecution
	if err := query.Order("created_at DESC").Find(&executions).Error; err != nil {
		return nil, 0, err
	}

	return executions, total, nil
}

// CancelExecution cancels a running execution
func (e *Executor) CancelExecution(ctx context.Context, tenantID, executionID string) error {
	result := e.db.Model(&models.WorkflowExecution{}).
		Where("id = ? AND tenant_id = ? AND status IN ?", executionID, tenantID, []models.ExecutionStatus{models.ExecutionStatusPending, models.ExecutionStatusRunning}).
		Updates(map[string]interface{}{
			"status":       models.ExecutionStatusCancelled,
			"completed_at": time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("failed to cancel execution: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("execution not found or already completed")
	}

	return nil
}
