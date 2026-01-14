// Package mcp provides MCP (Model Context Protocol) server implementation.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/yourorg/control-plane/pkg/agent"
	"github.com/yourorg/control-plane/pkg/audit"
	"github.com/yourorg/control-plane/pkg/campaign"
	"github.com/yourorg/control-plane/pkg/db/models"
	"github.com/yourorg/control-plane/pkg/workflow"
)

// ToolHandler handles tool invocations
type ToolHandler struct {
	db              *gorm.DB
	logger          *zap.Logger
	agentRegistry   *agent.Registry
	workflowManager *workflow.Manager
	campaignManager *campaign.Manager
	auditLogger     *audit.Logger
}

// NewToolHandler creates a new tool handler
func NewToolHandler(
	db *gorm.DB,
	logger *zap.Logger,
	agentRegistry *agent.Registry,
	workflowManager *workflow.Manager,
	campaignManager *campaign.Manager,
	auditLogger *audit.Logger,
) *ToolHandler {
	return &ToolHandler{
		db:              db,
		logger:          logger,
		agentRegistry:   agentRegistry,
		workflowManager: workflowManager,
		campaignManager: campaignManager,
		auditLogger:     auditLogger,
	}
}

// HandleTool handles a tool invocation
func (h *ToolHandler) HandleTool(ctx context.Context, name string, args map[string]interface{}) (*CallToolResult, error) {
	h.logger.Debug("handling tool", zap.String("name", name))

	switch name {
	case "list_agents":
		return h.listAgents(ctx, args)
	case "get_agent":
		return h.getAgent(ctx, args)
	case "list_workflows":
		return h.listWorkflows(ctx, args)
	case "get_workflow":
		return h.getWorkflow(ctx, args)
	case "create_workflow":
		return h.createWorkflow(ctx, args)
	case "execute_workflow":
		return h.executeWorkflow(ctx, args)
	case "list_campaigns":
		return h.listCampaigns(ctx, args)
	case "get_campaign":
		return h.getCampaign(ctx, args)
	case "create_campaign":
		return h.createCampaign(ctx, args)
	case "start_campaign":
		return h.startCampaign(ctx, args)
	case "get_campaign_progress":
		return h.getCampaignProgress(ctx, args)
	case "search_audit_logs":
		return h.searchAuditLogs(ctx, args)
	case "generate_workflow":
		return h.generateWorkflow(ctx, args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

func (h *ToolHandler) listAgents(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
	tenantID, _ := args["tenant_id"].(string)
	if tenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}

	status, _ := args["status"].(string)
	limit := getIntArg(args, "limit", 50)
	offset := getIntArg(args, "offset", 0)

	var tags map[string]string
	if tagsRaw, ok := args["tags"].(map[string]interface{}); ok {
		tags = make(map[string]string)
		for k, v := range tagsRaw {
			if s, ok := v.(string); ok {
				tags[k] = s
			}
		}
	}

	agents, total, err := h.agentRegistry.List(ctx, &agent.ListRequest{
		TenantID: tenantID,
		Status:   status,
		Tags:     tags,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"total":  total,
		"limit":  limit,
		"offset": offset,
		"agents": agents,
	}

	return h.jsonResult(result)
}

func (h *ToolHandler) getAgent(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
	tenantID, _ := args["tenant_id"].(string)
	agentID, _ := args["agent_id"].(string)

	if tenantID == "" || agentID == "" {
		return nil, fmt.Errorf("tenant_id and agent_id are required")
	}

	agentData, err := h.agentRegistry.Get(ctx, tenantID, agentID)
	if err != nil {
		return nil, err
	}

	return h.jsonResult(agentData)
}

func (h *ToolHandler) listWorkflows(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
	tenantID, _ := args["tenant_id"].(string)
	if tenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}

	status, _ := args["status"].(string)
	limit := getIntArg(args, "limit", 50)
	offset := getIntArg(args, "offset", 0)

	workflows, total, err := h.workflowManager.List(ctx, tenantID, models.WorkflowStatus(status), limit, offset)
	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"total":     total,
		"limit":     limit,
		"offset":    offset,
		"workflows": workflows,
	}

	return h.jsonResult(result)
}

func (h *ToolHandler) getWorkflow(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
	tenantID, _ := args["tenant_id"].(string)
	workflowID, _ := args["workflow_id"].(string)

	if tenantID == "" || workflowID == "" {
		return nil, fmt.Errorf("tenant_id and workflow_id are required")
	}

	wf, err := h.workflowManager.Get(ctx, tenantID, workflowID)
	if err != nil {
		return nil, err
	}

	return h.jsonResult(wf)
}

func (h *ToolHandler) createWorkflow(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
	tenantID, _ := args["tenant_id"].(string)
	name, _ := args["name"].(string)
	description, _ := args["description"].(string)
	definition, _ := args["definition"].(string)

	if tenantID == "" || name == "" || definition == "" {
		return nil, fmt.Errorf("tenant_id, name, and definition are required")
	}

	wf, err := h.workflowManager.Create(ctx, &workflow.CreateWorkflowRequest{
		TenantID:    tenantID,
		Name:        name,
		Description: description,
		Definition:  definition,
	})
	if err != nil {
		return nil, err
	}

	return h.jsonResult(wf)
}

func (h *ToolHandler) executeWorkflow(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
	tenantID, _ := args["tenant_id"].(string)
	workflowID, _ := args["workflow_id"].(string)
	agentID, _ := args["agent_id"].(string)

	if tenantID == "" || workflowID == "" || agentID == "" {
		return nil, fmt.Errorf("tenant_id, workflow_id, and agent_id are required")
	}

	var params map[string]interface{}
	if p, ok := args["parameters"].(map[string]interface{}); ok {
		params = p
	}

	// Create execution record
	executor := workflow.NewExecutor(h.db, h.logger)
	executionID, err := executor.StartExecution(ctx, &workflow.ExecutionRequest{
		TenantID:   tenantID,
		WorkflowID: workflowID,
		AgentID:    agentID,
		Parameters: params,
	})
	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"execution_id": executionID,
		"status":       "pending",
		"message":      "Workflow execution started",
	}

	return h.jsonResult(result)
}

func (h *ToolHandler) listCampaigns(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
	tenantID, _ := args["tenant_id"].(string)
	if tenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}

	status, _ := args["status"].(string)
	limit := getIntArg(args, "limit", 50)
	offset := getIntArg(args, "offset", 0)

	campaigns, total, err := h.campaignManager.List(ctx, tenantID, models.CampaignStatus(status), limit, offset)
	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"total":     total,
		"limit":     limit,
		"offset":    offset,
		"campaigns": campaigns,
	}

	return h.jsonResult(result)
}

func (h *ToolHandler) getCampaign(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
	tenantID, _ := args["tenant_id"].(string)
	campaignID, _ := args["campaign_id"].(string)

	if tenantID == "" || campaignID == "" {
		return nil, fmt.Errorf("tenant_id and campaign_id are required")
	}

	camp, err := h.campaignManager.Get(ctx, tenantID, campaignID)
	if err != nil {
		return nil, err
	}

	return h.jsonResult(camp)
}

func (h *ToolHandler) createCampaign(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
	tenantID, _ := args["tenant_id"].(string)
	workflowID, _ := args["workflow_id"].(string)
	name, _ := args["name"].(string)
	description, _ := args["description"].(string)

	if tenantID == "" || workflowID == "" || name == "" {
		return nil, fmt.Errorf("tenant_id, workflow_id, and name are required")
	}

	targetSelector := make(map[string]interface{})
	if ts, ok := args["target_selector"].(map[string]interface{}); ok {
		targetSelector = ts
	}

	var phases []campaign.PhaseConfig
	if phasesRaw, ok := args["phases"].([]interface{}); ok {
		for _, p := range phasesRaw {
			if pm, ok := p.(map[string]interface{}); ok {
				phase := campaign.PhaseConfig{
					Name:             getStringArg(pm, "name", ""),
					Percentage:       getFloatArg(pm, "percentage", 0),
					SuccessThreshold: getFloatArg(pm, "success_threshold", 95),
					WaitMinutes:      getIntArg(pm, "wait_minutes", 15),
				}
				phases = append(phases, phase)
			}
		}
	}

	if len(phases) == 0 {
		return nil, fmt.Errorf("at least one phase is required")
	}

	camp, err := h.campaignManager.Create(ctx, &campaign.CreateCampaignRequest{
		TenantID:       tenantID,
		WorkflowID:     workflowID,
		Name:           name,
		Description:    description,
		TargetSelector: targetSelector,
		PhaseConfig:    phases,
	})
	if err != nil {
		return nil, err
	}

	return h.jsonResult(camp)
}

func (h *ToolHandler) startCampaign(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
	tenantID, _ := args["tenant_id"].(string)
	campaignID, _ := args["campaign_id"].(string)

	if tenantID == "" || campaignID == "" {
		return nil, fmt.Errorf("tenant_id and campaign_id are required")
	}

	if err := h.campaignManager.Start(ctx, tenantID, campaignID); err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"campaign_id": campaignID,
		"status":      "running",
		"message":     "Campaign started successfully",
	}

	return h.jsonResult(result)
}

func (h *ToolHandler) getCampaignProgress(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
	tenantID, _ := args["tenant_id"].(string)
	campaignID, _ := args["campaign_id"].(string)

	if tenantID == "" || campaignID == "" {
		return nil, fmt.Errorf("tenant_id and campaign_id are required")
	}

	progress, err := h.campaignManager.GetProgress(ctx, tenantID, campaignID)
	if err != nil {
		return nil, err
	}

	return h.jsonResult(progress)
}

func (h *ToolHandler) searchAuditLogs(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
	tenantID, _ := args["tenant_id"].(string)
	if tenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}

	query := &audit.SearchQuery{
		TenantID:    tenantID,
		Query:       getStringArg(args, "query", ""),
		ActorID:     getStringArg(args, "actor_id", ""),
		ResourceID:  getStringArg(args, "resource_id", ""),
		MaxHits:     getIntArg(args, "limit", 100),
		StartOffset: 0,
		SortBy: []audit.SortField{
			{Field: "timestamp", Order: "desc"},
		},
	}

	// Parse event types
	if eventTypes, ok := args["event_types"].([]interface{}); ok {
		for _, et := range eventTypes {
			if s, ok := et.(string); ok {
				query.EventTypes = append(query.EventTypes, audit.EventType(s))
			}
		}
	}

	// Parse actions
	if actions, ok := args["actions"].([]interface{}); ok {
		for _, a := range actions {
			if s, ok := a.(string); ok {
				query.Actions = append(query.Actions, audit.EventAction(s))
			}
		}
	}

	// Parse time range
	if startTime, ok := args["start_time"].(string); ok && startTime != "" {
		if t, err := time.Parse(time.RFC3339, startTime); err == nil {
			query.StartTime = &t
		}
	}
	if endTime, ok := args["end_time"].(string); ok && endTime != "" {
		if t, err := time.Parse(time.RFC3339, endTime); err == nil {
			query.EndTime = &t
		}
	}

	if h.auditLogger == nil {
		return nil, fmt.Errorf("audit logging not configured")
	}

	result, err := h.auditLogger.Search(ctx, query)
	if err != nil {
		return nil, err
	}

	return h.jsonResult(result)
}

func (h *ToolHandler) generateWorkflow(ctx context.Context, args map[string]interface{}) (*CallToolResult, error) {
	description, _ := args["description"].(string)
	if description == "" {
		return nil, fmt.Errorf("description is required")
	}

	targetOS := getStringArg(args, "target_os", "linux")
	includeRollback := getBoolArg(args, "include_rollback", true)

	// Generate a template workflow based on common patterns
	workflow := h.generateWorkflowTemplate(description, targetOS, includeRollback)

	result := map[string]interface{}{
		"generated_workflow": workflow,
		"notes":              "This is a template workflow. Please review and customize as needed.",
	}

	return h.jsonResult(result)
}

func (h *ToolHandler) generateWorkflowTemplate(description, targetOS string, includeRollback bool) string {
	// Generate a basic workflow template
	template := fmt.Sprintf(`# Generated workflow for: %s
# Target OS: %s

name: generated_workflow
description: "%s"

variables:
  backup_dir: /var/backup
  log_file: /var/log/workflow.log

steps:
  - name: pre_check
    description: Pre-execution checks
    command: |
      echo "Starting workflow execution at $(date)"
      echo "Checking system prerequisites..."
`, description, targetOS, description)

	if targetOS == "linux" || targetOS == "both" {
		template += `
  - name: execute_main_linux
    description: Main execution for Linux
    condition: "{{ .OS == 'linux' }}"
    command: |
      echo "Executing main task on Linux..."
      # Add your Linux-specific commands here
    retry:
      max_attempts: 3
      delay_seconds: 10
`
	}

	if targetOS == "windows" || targetOS == "both" {
		template += `
  - name: execute_main_windows
    description: Main execution for Windows
    condition: "{{ .OS == 'windows' }}"
    command: |
      Write-Host "Executing main task on Windows..."
      # Add your Windows-specific commands here
    shell: powershell
    retry:
      max_attempts: 3
      delay_seconds: 10
`
	}

	template += `
  - name: verify
    description: Verify execution
    command: |
      echo "Verifying execution results..."
      # Add verification commands here
`

	if includeRollback {
		template += `
rollback:
  - name: rollback_changes
    description: Rollback on failure
    command: |
      echo "Rolling back changes..."
      # Add rollback commands here
`
	}

	return template
}

func (h *ToolHandler) jsonResult(data interface{}) (*CallToolResult, error) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}

	return &CallToolResult{
		Content: []Content{TextContent(string(jsonData))},
	}, nil
}

// Helper functions

func getStringArg(args map[string]interface{}, key, defaultValue string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return defaultValue
}

func getIntArg(args map[string]interface{}, key string, defaultValue int) int {
	if v, ok := args[key].(float64); ok {
		return int(v)
	}
	if v, ok := args[key].(int); ok {
		return v
	}
	return defaultValue
}

func getFloatArg(args map[string]interface{}, key string, defaultValue float64) float64 {
	if v, ok := args[key].(float64); ok {
		return v
	}
	return defaultValue
}

func getBoolArg(args map[string]interface{}, key string, defaultValue bool) bool {
	if v, ok := args[key].(bool); ok {
		return v
	}
	return defaultValue
}
